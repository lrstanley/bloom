// Package store keeps notes in a file, one JSON object per line.
package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"time"
)

// Note is a note and its tags.
type Note struct {
	ID      string    `json:"id"`
	Text    string    `json:"text"`
	Tags    []string  `json:"tags,omitempty"`
	Created time.Time `json:"created"`
}

// Store is the notes of a file, read when it is opened.
type Store struct {
	path  string
	notes []Note
	next  int
	dirty bool
}

// Open reads the notes in the file at path, which need not exist yet.
func Open(path string) (*Store, error) {
	s := &Store{path: path, next: 1}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := s.load(bufio.NewScanner(f)); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// load reads a note from each line of sc.
func (s *Store) load(sc *bufio.Scanner) error {
	for sc.Scan() {
		var n Note
		if err := json.Unmarshal(sc.Bytes(), &n); err != nil {
			return err
		}
		s.notes = append(s.notes, n)
		s.next = max(s.next, idNumber(n.ID)+1)
	}
	return sc.Err()
}

// Add adds a note with text and tags, and returns it.
func (s *Store) Add(text string, tags ...string) Note {
	n := Note{ID: s.newID(), Text: text, Tags: normalize(tags), Created: time.Now()}
	s.notes = append(s.notes, n)
	s.dirty = true
	return n
}

// All returns every note, oldest first.
func (s *Store) All() []Note { return slices.Clone(s.notes) }

// Filter returns the notes keep returns true for.
func (s *Store) Filter(keep func(Note) bool) []Note {
	var out []Note
	for _, n := range s.notes {
		if keep(n) {
			out = append(out, n)
		}
	}
	return out
}

// Tag adds tags to the note with ID id.
func (s *Store) Tag(id string, tags ...string) error {
	i := s.index(id)
	if i < 0 {
		return fmt.Errorf("no note %s", id)
	}
	s.notes[i].Tags = normalize(append(s.notes[i].Tags, tags...))
	s.dirty = true
	return nil
}

// Save writes the notes back to the file, if they changed.
func (s *Store) Save() error {
	if !s.dirty {
		return nil
	}
	f, err := os.Create(s.path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, n := range s.notes {
		if err := enc.Encode(n); err != nil {
			f.Close()
			return err
		}
	}
	s.dirty = false
	return f.Close()
}

// Close saves any changes.
func (s *Store) Close() error { return s.Save() }

func (s *Store) index(id string) int {
	return slices.IndexFunc(s.notes, func(n Note) bool { return n.ID == id })
}

func (s *Store) newID() string {
	id := "n" + strconv.Itoa(s.next)
	s.next++
	return id
}

// idNumber returns the number in a note ID such as n12, or 0.
func idNumber(id string) int {
	n, _ := strconv.Atoi(id[min(1, len(id)):])
	return n
}

// normalize sorts tags and drops duplicates.
func normalize(tags []string) []string {
	slices.Sort(tags)
	return slices.Compact(tags)
}
