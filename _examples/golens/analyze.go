// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

package main

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

// kind is what sort of function a node of the graph is.
type kind uint8

const (
	kindFunc      kind = iota // a function declared in the project
	kindMethod                // a method declared in the project
	kindInterface             // an interface method, whose callees are its implementations
	kindExternal              // declared outside the project, such as in the standard library
)

// fn is a function in the call graph.
type fn struct {
	obj     *types.Func
	name    string // the name to show: Name, or Recv.Name for a method
	pkg     string // the package's name
	pkgPath string
	kind    kind

	sig  string // the declaration's signature, as Go source
	doc  string
	file string // relative to the project, for local functions
	line int    // the line of src[0] in file

	// src is the source of the declaration, doc comment included, with tabs
	// expanded; toks is its highlighting, by chroma.
	src  []string
	toks [][]tok

	// calls are the functions this one calls or refers to, in the order
	// they first appear, and sites where in src each is named. For an
	// interface method, calls are its implementations in the project.
	calls   []*fn
	sites   map[*fn][]site
	callers []*fn
}

// site is where a callee is named in a function's source: a line of src, and
// the cells of that line.
type site struct{ line, col, n int }

// local reports whether f has source in the project.
func (f *fn) local() bool { return f.kind == kindFunc || f.kind == kindMethod }

// qualified is f's name with its package.
func (f *fn) qualified() string { return f.pkg + "." + f.name }

// siteAt returns the callee named at the cell col of line, if any.
func (f *fn) siteAt(line, col int) (*fn, bool) {
	for callee, sites := range f.sites {
		for _, s := range sites {
			if s.line == line && col >= s.col && col < s.col+s.n {
				return callee, true
			}
		}
	}
	return nil, false
}

// mainPkg is the name of a command's package, and of its entry point.
const mainPkg = "main"

// program is the call graph of a project.
type program struct {
	module string // the module's path, or the directory's name
	roots  []*fn  // the entry points: functions that nothing in the project calls
	all    []*fn  // every function, local ones first, for search
}

// analyzer builds a [program] from loaded packages.
type analyzer struct {
	root  string
	fns   map[*types.Func]*fn
	named []*types.Named // the project's named non-interface types, for implementations
}

// analyze loads the packages matching patterns in dir, or ./... without any,
// and builds their call graph. Calls are found from the syntax and types alone, so a call through
// a function value is not seen, and a call through an interface goes to the
// interface method, which in turn "calls" each of its implementations.
func analyze(dir string, patterns ...string) (*program, error) {
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule,
		Dir: abs,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, err
	}
	var errs []error
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			errs = append(errs, e)
		}
	})
	if len(pkgs) == 0 || (len(errs) > 0 && !anyTyped(pkgs)) {
		return nil, cmp.Or(errors.Join(errs...), fmt.Errorf("no Go packages in %s", abs))
	}

	a := &analyzer{root: abs, fns: make(map[*types.Func]*fn)}
	prog := &program{module: filepath.Base(abs)}
	for _, p := range pkgs {
		if p.Module != nil && p.Module.Main {
			prog.module = p.Module.Path
		}
		if p.Types == nil || p.TypesInfo == nil {
			continue
		}
		a.collectTypes(p.Types)
		for _, file := range p.Syntax {
			a.file(p, file)
		}
	}
	a.implementations()
	a.link(prog)
	return prog, nil
}

func anyTyped(pkgs []*packages.Package) bool {
	return slices.ContainsFunc(pkgs, func(p *packages.Package) bool { return p.TypesInfo != nil })
}

// get returns the node for obj, making it the first time.
func (a *analyzer) get(obj *types.Func) *fn {
	obj = obj.Origin()
	if f, ok := a.fns[obj]; ok {
		return f
	}
	f := &fn{obj: obj, name: obj.Name(), kind: kindExternal, sites: make(map[*fn][]site)}
	if p := obj.Pkg(); p != nil {
		f.pkg, f.pkgPath = p.Name(), p.Path()
	}
	sig, _ := obj.Type().(*types.Signature)
	if recv := sig.Recv(); recv != nil {
		t := recv.Type()
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		if n, ok := t.(*types.Named); ok {
			f.name = n.Obj().Name() + "." + f.name
		}
		if types.IsInterface(t) {
			f.kind = kindInterface
		}
	}
	// Replaced with the declaration's own text for local functions.
	f.sig = "func " + f.name + strings.TrimPrefix(types.TypeString(sig, func(p *types.Package) string {
		return p.Name()
	}), "func")
	a.fns[obj] = f
	return f
}

// collectTypes notes the named types of pkg that can implement interfaces.
func (a *analyzer) collectTypes(pkg *types.Package) {
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok || tn.IsAlias() {
			continue
		}
		if n, named := tn.Type().(*types.Named); named && !types.IsInterface(n) && n.TypeParams().Len() == 0 {
			a.named = append(a.named, n)
		}
	}
}

// file adds the functions declared in file.
func (a *analyzer) file(p *packages.Package, file *ast.File) {
	tf := p.Fset.File(file.Pos())
	if tf == nil {
		return
	}
	content, err := os.ReadFile(tf.Name())
	if err != nil {
		return
	}
	rel, err := filepath.Rel(a.root, tf.Name())
	if err != nil {
		rel = tf.Name()
	}
	rel = display(rel)
	for _, decl := range file.Decls {
		d, ok := decl.(*ast.FuncDecl)
		if !ok || d.Body == nil {
			continue
		}
		obj, ok := p.TypesInfo.Defs[d.Name].(*types.Func)
		if !ok {
			continue
		}
		f := a.get(obj)
		f.kind = kindFunc
		if d.Recv != nil {
			f.kind = kindMethod
		}
		f.file, f.doc = rel, display(strings.TrimSpace(d.Doc.Text()))
		f.sig = signature(p.Fset, d)

		start := d.Pos()
		if d.Doc != nil {
			start = d.Doc.Pos()
		}
		// Unadjusted for //line directives: this is the line in content.
		f.line = tf.PositionFor(start, false).Line
		from := tf.Offset(tf.LineStart(f.line))
		raw := strings.Split(string(content[from:tf.Offset(d.End())]), "\n")
		f.src = make([]string, len(raw))
		for i, l := range raw {
			f.src[i] = display(l)
		}
		f.toks = highlight(f.src)

		ast.Inspect(d.Body, func(n ast.Node) bool {
			id, isIdent := n.(*ast.Ident)
			if !isIdent {
				return true
			}
			callee, isFunc := p.TypesInfo.Uses[id].(*types.Func)
			if !isFunc {
				return true
			}
			to := a.get(callee)
			pos := tf.PositionFor(id.Pos(), false)
			line := pos.Line - f.line
			if line < 0 || line >= len(raw) {
				return true
			}
			if _, seen := f.sites[to]; !seen {
				f.calls = append(f.calls, to)
			}
			col := len(display(raw[line][:pos.Column-1]))
			f.sites[to] = append(f.sites[to], site{line: line, col: col, n: len(id.Name)})
			return true
		})
	}
}

// implementations makes each interface method that is called "call" the
// project's methods that implement it.
func (a *analyzer) implementations() {
	for _, f := range a.fns {
		if f.kind != kindInterface {
			continue
		}
		sig, _ := f.obj.Type().(*types.Signature)
		recv := sig.Recv().Type()
		iface, ok := recv.Underlying().(*types.Interface)
		if n, generic := recv.(*types.Named); !ok || (generic && n.TypeParams().Len() > 0) {
			continue // what implements a generic interface depends on its type arguments
		}
		for _, n := range a.named {
			var t types.Type = n
			if !types.Implements(t, iface) {
				if t = types.NewPointer(n); !types.Implements(t, iface) {
					continue
				}
			}
			obj, _, _ := types.LookupFieldOrMethod(t, true, f.obj.Pkg(), f.obj.Name())
			if m, isFunc := obj.(*types.Func); isFunc {
				if impl := a.fns[m.Origin()]; impl != nil && impl.local() && !slices.Contains(f.calls, impl) {
					f.calls = append(f.calls, impl)
				}
			}
		}
		slices.SortFunc(f.calls, byName)
	}
}

// link fills in callers, the roots, and the list of every function.
func (a *analyzer) link(prog *program) {
	for _, f := range a.fns {
		prog.all = append(prog.all, f)
		for _, c := range f.calls {
			if c != f {
				c.callers = append(c.callers, f)
			}
		}
	}
	slices.SortFunc(prog.all, func(x, y *fn) int {
		return cmp.Or(cmp.Compare(x.kind, y.kind), byName(x, y))
	})
	for _, f := range prog.all {
		slices.SortFunc(f.callers, byName)
		if f.local() && len(f.callers) == 0 {
			prog.roots = append(prog.roots, f)
		}
	}
	// Entry points first: main, then init.
	rank := func(f *fn) int {
		switch {
		case f.pkg == mainPkg && f.name == mainPkg:
			return 0
		case f.name == "init":
			return 1
		}
		return 2
	}
	slices.SortStableFunc(prog.roots, func(x, y *fn) int { return cmp.Compare(rank(x), rank(y)) })

	// Functions only reachable from each other, such as mutual recursion
	// nothing else calls, become roots too, so that every function can be
	// found by walking down from a root.
	seen := make(map[*fn]bool)
	var walk func(f *fn)
	walk = func(f *fn) {
		if seen[f] {
			return
		}
		seen[f] = true
		for _, c := range f.calls {
			walk(c)
		}
	}
	for _, r := range prog.roots {
		walk(r)
	}
	for _, f := range prog.all {
		if f.local() && !seen[f] {
			prog.roots = append(prog.roots, f)
			walk(f)
		}
	}
}

func byName(x, y *fn) int {
	return cmp.Or(cmp.Compare(x.pkgPath, y.pkgPath), cmp.Compare(x.name, y.name))
}

// pathTo returns the shortest path to target from a root, as indexes: of the
// root, then of each callee in turn. It returns nil when target cannot be
// reached.
func (p *program) pathTo(target *fn) []int {
	type step struct {
		f    *fn
		prev int // index of the step before, or -1
		i    int // index of f in the roots or in its caller's calls
	}
	var queue []step
	seen := make(map[*fn]bool)
	for i, r := range p.roots {
		if !seen[r] {
			seen[r] = true
			queue = append(queue, step{f: r, prev: -1, i: i})
		}
	}
	for at := 0; at < len(queue); at++ {
		s := queue[at]
		if s.f == target {
			var path []int
			for j := at; j >= 0; j = queue[j].prev {
				path = append(path, queue[j].i)
			}
			slices.Reverse(path)
			return path
		}
		for i, c := range s.f.calls {
			if !seen[c] {
				seen[c] = true
				queue = append(queue, step{f: c, prev: at, i: i})
			}
		}
	}
	return nil
}

// signature returns d's declaration without its doc comment or body.
func signature(fset *token.FileSet, d *ast.FuncDecl) string {
	decl := *d
	decl.Doc, decl.Body = nil, nil
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, &decl); err != nil {
		return d.Name.Name
	}
	return display(b.String())
}

// display returns source text as it is drawn: tabs as spaces to four-cell
// stops, and control characters, which comments and strings may hold, as
// their symbols, so the source can't control the terminal.
func display(s string) string {
	var b strings.Builder
	col := 0
	for _, r := range s {
		switch {
		case r == '\t':
			n := 4 - col%4
			b.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		case r == '\n':
			b.WriteRune(r)
			col = 0
			continue
		case r == '\r':
			continue
		case r < 0x20:
			r += 0x2400
		case r == 0x7f:
			r = '\u2421'
		case unicode.IsControl(r):
			r = unicode.ReplacementChar
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}
