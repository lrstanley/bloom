// Command notes keeps short notes in a file, for golens to explore.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"example.com/notes/render"
	"example.com/notes/store"
)

// usage is printed for -h and for unknown commands.
const usage = "usage: notes [add|list|find|tag] [args...]"

func main() {
	os.Exit(run(os.Args[1:]))
}

// run runs the command in args and returns the exit code.
func run(args []string) int {
	cmd, rest, err := parseArgs(args)
	if err != nil {
		return report(err)
	}
	s, err := store.Open(notesPath())
	if err != nil {
		return report(err)
	}
	defer s.Close()

	switch cmd {
	case "add":
		err = add(s, rest)
	case "list":
		err = list(s, pick(rest))
	case "find":
		err = find(s, rest)
	case "tag":
		err = tag(s, rest)
	}
	if err != nil {
		return report(err)
	}
	return 0
}

// parseArgs splits args into a command and its arguments.
func parseArgs(args []string) (cmd string, rest []string, err error) {
	if len(args) == 0 || args[0] == "-h" {
		return "", nil, errors.New(usage)
	}
	switch args[0] {
	case "add", "list", "find", "tag":
		return args[0], args[1:], nil
	}
	return "", nil, fmt.Errorf("unknown command %q\n%s", args[0], usage)
}

// report prints err to standard error and returns the exit code for it.
func report(err error) int {
	fmt.Fprintln(os.Stderr, "notes:", err)
	return 1
}

// notesPath is where the notes are kept: $NOTES, or ~/.notes.
func notesPath() string {
	if p := os.Getenv("NOTES"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return home + "/.notes"
}

// pick returns the renderer named by the -json flag in args.
func pick(args []string) render.Renderer {
	for _, a := range args {
		if a == "-json" {
			return render.JSON{Indent: "  "}
		}
	}
	return render.Text{Width: 72}
}

// add saves a note made of args, with any #tags in it.
func add(s *store.Store, args []string) error {
	text := strings.Join(args, " ")
	if text == "" {
		return errors.New("nothing to add")
	}
	n := s.Add(text, tagsIn(text)...)
	fmt.Println("added", n.ID)
	return s.Save()
}

// list prints every note with r.
func list(s *store.Store, r render.Renderer) error {
	return r.Render(os.Stdout, s.All())
}

// find prints the notes that mention every word in args.
func find(s *store.Store, args []string) error {
	notes := s.Filter(func(n store.Note) bool {
		for _, w := range args {
			if !strings.Contains(strings.ToLower(n.Text), strings.ToLower(w)) {
				return false
			}
		}
		return true
	})
	return render.Text{Width: 72}.Render(os.Stdout, notes)
}

// tag adds the tags in args[1:] to the note with ID args[0].
func tag(s *store.Store, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: notes tag ID TAG...")
	}
	if err := s.Tag(args[0], args[1:]...); err != nil {
		return err
	}
	return s.Save()
}

// tagsIn returns the words of text that start with #, without it.
func tagsIn(text string) []string {
	var tags []string
	for _, w := range strings.Fields(text) {
		if t, ok := strings.CutPrefix(w, "#"); ok && t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}
