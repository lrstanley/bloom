// Copyright (c) Liam Stanley <liam@liam.sh>. All rights reserved. Use of
// this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Command gen builds package fonts from manifest.json: it downloads each font
// from the commit the manifest pins, checks it against the manifest's SHA-256,
// adds an SPDX-License-Identifier comment line to the fonts whose license is
// known, and writes the fonts gzipped, with fonts.gen.go and LICENSES.md. Run
// it with go generate, from package fonts.
package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"
)

type font struct {
	Name    string `json:"name"`
	Func    string `json:"func"`
	File    string `json:"file"`
	License string `json:"license"`
	Source  struct {
		Repo   string `json:"repo"`
		Commit string `json:"commit"`
		Path   string `json:"path"`
	} `json:"source"`
	SHA256 string `json:"sha256"`
	Usage  int    `json:"usage"`

	// Filled in from the font itself.
	Height int    `json:"-"`
	Credit string `json:"-"`
}

// URL is where the font is on GitHub, at the commit it was taken from.
func (f font) URL() string {
	return "https://github.com/" + f.Source.Repo + "/blob/" + f.Source.Commit + "/" + f.path()
}

func (f font) raw() string {
	return "https://raw.githubusercontent.com/" + f.Source.Repo + "/" + f.Source.Commit + "/" + f.path()
}

func (f font) path() string {
	parts := strings.Split(f.Source.Path, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func main() {
	manifest := flag.String("manifest", "internal/gen/manifest.json", "the font manifest")
	out := flag.String("out", ".", "the directory of package fonts")
	flag.Parse()

	if err := run(context.Background(), *manifest, *out); err != nil {
		slog.Error("generating fonts", "error", err) //nolint:sloglint // a command's own logger.
		os.Exit(1)
	}
}

func run(ctx context.Context, manifest, out string) error {
	data, rerr := os.ReadFile(manifest)
	if rerr != nil {
		return rerr
	}
	var fonts []font
	if err := json.Unmarshal(data, &fonts); err != nil {
		return fmt.Errorf("manifest: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(out, "data"), 0o750); err != nil {
		return err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	for i := range fonts {
		f := &fonts[i]
		body, err := fetch(ctx, client, f.raw())
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		body, err = normalize(body)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		if sum := sha256.Sum256(body); hex.EncodeToString(sum[:]) != f.SHA256 {
			return fmt.Errorf("%s: checksum %x does not match the manifest", f.Name, sum)
		}
		body, err = annotate(f, body)
		if err != nil {
			return fmt.Errorf("%s: %w", f.Name, err)
		}
		err = os.WriteFile(filepath.Join(out, "data", f.File), compress(body), 0o644) //nolint:gosec // source files.
		if err != nil {
			return err
		}
	}
	slices.SortFunc(fonts, func(a, b font) int { return strings.Compare(a.Name, b.Name) })
	if err := write(filepath.Join(out, "fonts.gen.go"), codeTmpl, fonts, true); err != nil {
		return err
	}
	return write(filepath.Join(out, "LICENSES.md"), licensesTmpl, fonts, false)
}

func fetch(ctx context.Context, client *http.Client, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

// normalize unzips a zipped font and turns CRLF line endings into LF, which
// is what the manifest's checksums are of.
func normalize(b []byte) ([]byte, error) {
	if bytes.HasPrefix(b, []byte("PK\x03\x04")) {
		zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return nil, err
		}
		if len(zr.File) == 0 {
			return nil, errors.New("empty zip")
		}
		rc, err := zr.File[0].Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		if b, err = io.ReadAll(io.LimitReader(rc, 64<<20)); err != nil {
			return nil, err
		}
	}
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n")), nil
}

// annotate reads what the docs say about a font from its header, and adds
// an SPDX-License-Identifier comment line to the header of a font whose
// license is known.
func annotate(f *font, b []byte) ([]byte, error) {
	text := strings.TrimPrefix(string(b), "\ufeff")
	header, rest, _ := strings.Cut(text, "\n")
	fields := strings.Fields(header)
	if len(fields) < 6 {
		return nil, errors.New("short header")
	}
	height, err := strconv.Atoi(fields[1])
	if err != nil {
		return nil, fmt.Errorf("height: %w", err)
	}
	ncom, err := strconv.Atoi(fields[5])
	if err != nil {
		return nil, fmt.Errorf("comment lines: %w", err)
	}
	f.Height = height
	lines := strings.SplitN(rest, "\n", ncom+1)
	for _, l := range lines[:min(ncom, len(lines))] {
		if l = strings.TrimSpace(l); l != "" && strings.ContainsFunc(l, isLetter) {
			f.Credit = clean(l)
			break
		}
	}
	if f.License == "" {
		return b, nil
	}
	fields[5] = strconv.Itoa(ncom + 1)
	comments := lines[:min(ncom, len(lines))]
	body := ""
	if len(lines) > ncom {
		body = lines[ncom]
	}
	var out strings.Builder
	out.WriteString(strings.Join(fields, " ") + "\n")
	for _, c := range comments {
		out.WriteString(c + "\n")
	}
	out.WriteString("SPDX-License-Identifier: " + f.License + "\n")
	out.WriteString(body)
	return []byte(out.String()), nil
}

func isLetter(r rune) bool { return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' }

// clean makes a header line safe and short enough for a doc comment.
func clean(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f || r == 0xfffd {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > 90 {
		s = string([]rune(s)[:89]) + "…"
	}
	return s
}

func compress(b []byte) []byte {
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	_, _ = zw.Write(b)
	_ = zw.Close()
	return buf.Bytes()
}

func write(path string, tmpl *template.Template, fonts []font, gofmt bool) error {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, fonts); err != nil {
		return err
	}
	b := buf.Bytes()
	if gofmt {
		var err error
		if b, err = format.Source(b); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return os.WriteFile(path, b, 0o644) //nolint:gosec // source files.
}

var funcs = template.FuncMap{
	"lower": func(s string) string { return strings.ToLower(s[:1]) + s[1:] },
	"quote": strconv.Quote,
	"byLicense": func(fonts []font, license string) []font {
		var out []font
		for _, f := range fonts {
			if f.License == license {
				out = append(out, f)
			}
		}
		return out
	},
}

var codeTmpl = template.Must(template.New("code").Funcs(funcs).Parse(`// Code generated by internal/gen from internal/gen/manifest.json. DO NOT EDIT.

package fonts

import (
	_ "embed"

	"github.com/lrstanley/bloom/fig"
)

// Names are the names of the fonts, as [ByName] knows them.
var Names = []string{ {{- range .}}{{quote .Name}}, {{end -}} }

// ByName returns the font named name, such as "standard" or "doom". Using it
// puts every font in the program, as the name is not known until it runs.
func ByName(name string) (*fig.Font, bool) {
	switch name {
	{{- range .}}
	case {{quote .Name}}:
		return {{.Func}}(), true
	{{- end}}
	}
	return nil, false
}

// All returns every font, in the order of [Names]. Using it puts every font
// in the program.
func All() []*fig.Font {
	return []*fig.Font{ {{- range .}}{{.Func}}(), {{end -}} }
}
{{range .}}
{{- if ne .Name "standard"}}
//go:embed data/{{.File}}
var {{lower .Func}}Data string

var {{lower .Func}}Font lazy
{{end}}
// {{.Func}} is the font "{{.Name}}", {{.Height}} rows tall{{if .Credit}}: {{.Credit}}{{end}}.
// From {{.URL}}.
{{- if .License}}
// License: {{.License}}, see LICENSES.md.
{{- else}}
// No license is stated.
{{- end}}
{{- if eq .Name "standard"}}
//
// It is [fig.Standard], which package fig holds as its default font.
func {{.Func}}() *fig.Font { return fig.Standard() }
{{- else}}
func {{.Func}}() *fig.Font { return {{lower .Func}}Font.get({{quote .Name}}, {{lower .Func}}Data) }
{{- end}}
{{end}}`))

//nolint:misspell // Christiaan Keet's name.
var licensesTmpl = template.Must(template.New("licenses").Funcs(funcs).Parse(`# Font licenses

<!-- Generated by internal/gen from internal/gen/manifest.json. Do not edit. -->

These fonts state a license, and carry an SPDX-License-Identifier line in
their headers. The others state none; where each came from is in its
function's documentation.

## BSD-3-Clause

From [figlet](https://github.com/cmatsuoka/figlet), whose LICENSE covers
the fonts it distributes:
{{range byLicense . "BSD-3-Clause"}}
- {{.Name}}
{{- end}}

` + "```" + `
Copyright (C) 1991, 1993, 1994 Glenn Chappell and Ian Chai
Copyright (C) 1996, 1997, 1998, 1999, 2000, 2001 John Cowan
Copyright (C) 2002 Christiaan Keet
Copyright (C) 2011 Claudio Matsuoka
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions
are met:

1. Redistributions of source code must retain the above copyright
   notice, this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright
   notice, this list of conditions and the following disclaimer in the
   documentation and/or other materials provided with the distribution.
3. Neither the name of the copyright holders nor the names of their
   contributors may be used to endorse or promote products derived
   from this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS
FOR A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE
COPYRIGHT HOLDERS OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT,
INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING,
BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS
OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR
TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE
USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
` + "```" + `

## MIT

Each font's header names its author and the MIT License:
{{range byLicense . "MIT"}}
- {{.Name}}: {{.Credit}}
{{- end}}

` + "```" + `
Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
` + "```" + `

## WTFPL

From [TOIlet](https://github.com/cacalabs/toilet). Each font's header states
the WTFPL:
{{range byLicense . "WTFPL"}}
- {{.Name}}: {{.Credit}}
{{- end}}

` + "```" + `
            DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE
                    Version 2, December 2004

 Copyright (C) 2004 Sam Hocevar <sam@hocevar.net>

 Everyone is permitted to copy and distribute verbatim or modified
 copies of this license document, and changing it is allowed as long
 as the name is changed.

            DO WHAT THE FUCK YOU WANT TO PUBLIC LICENSE
   TERMS AND CONDITIONS FOR COPYING, DISTRIBUTION AND MODIFICATION

  0. You just DO WHAT THE FUCK YOU WANT TO.
` + "```" + `
`))
