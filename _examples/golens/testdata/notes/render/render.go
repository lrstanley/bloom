// Package render prints notes.
package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"example.com/notes/store"
)

// Renderer prints notes to w.
type Renderer interface {
	Render(w io.Writer, notes []store.Note) error
}

// Text prints notes as plain text, wrapped to Width.
type Text struct {
	Width int
}

// Render implements [Renderer].
func (t Text) Render(w io.Writer, notes []store.Note) error {
	for _, n := range notes {
		head := n.ID + " " + n.Created.Format("Jan 2")
		if len(n.Tags) > 0 {
			head += " #" + strings.Join(n.Tags, " #")
		}
		if _, err := fmt.Fprintf(w, "%s\n%s\n\n", head, indent(wrap(n.Text, t.Width-2), "  ")); err != nil {
			return err
		}
	}
	return nil
}

// JSON prints notes as a JSON array.
type JSON struct {
	Indent string
}

// Render implements [Renderer].
func (j JSON) Render(w io.Writer, notes []store.Note) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", j.Indent)
	return enc.Encode(notes)
}

// wrap breaks s into lines of at most width cells, between words.
func wrap(s string, width int) string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	line := 0
	for i, w := range words {
		if i > 0 && line+1+len(w) > width {
			b.WriteByte('\n')
			line = 0
		} else if i > 0 {
			b.WriteByte(' ')
			line++
		}
		b.WriteString(w)
		line += len(w)
	}
	return b.String()
}

// indent puts prefix before every line of s.
func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}
