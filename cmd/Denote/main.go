package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"9fans.net/go/acme"
)

type Note struct {
	Path       string
	Identifier string
	Signature  string
	Title      string
	Keywords   []string
	Extension  string
	FileTitle  string
}

var identifierRe = regexp.MustCompile(`^(\d{8}T\d{6})`)
var signatureRe = regexp.MustCompile(`==([^-_\.]+)`)
var titleRe = regexp.MustCompile(`--([^_\.]+)`)
var keywordsRe = regexp.MustCompile(`__(.+?)(?:\.|$)`)
var orgTitleRe = regexp.MustCompile(`(?m)^#\+title:\s*(.+)$`)
var mdTitleRe = regexp.MustCompile(`(?m)^#\s+(.+)$`)

func extractTitle(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// Check if binary
	if len(data) > 512 && !isText(data[:512]) {
		return ""
	}
	text := string(data)
	// Try org-mode title
	if m := orgTitleRe.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	// Try markdown title
	if m := mdTitleRe.FindStringSubmatch(text); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func isText(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

func parseNote(path string) *Note {
	name := filepath.Base(path)
	note := &Note{Path: path}

	if m := identifierRe.FindStringSubmatch(name); m != nil {
		note.Identifier = m[1]
	}
	if m := signatureRe.FindStringSubmatch(name); m != nil {
		note.Signature = m[1]
	}
	if m := titleRe.FindStringSubmatch(name); m != nil {
		note.Title = strings.ReplaceAll(m[1], "-", " ")
	}
	if m := keywordsRe.FindStringSubmatch(name); m != nil {
		note.Keywords = strings.Split(m[1], "_")
	}
	note.Extension = filepath.Ext(name)
	note.FileTitle = extractTitle(path)

	return note
}

func defaultDir() string {
	if d := os.Getenv("DENOTE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_DOCUMENTS_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), "doc")
}

func main() {
	dir := flag.String("dir", defaultDir(), "denote directory")
	flag.Parse()

	entries, err := os.ReadDir(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading directory: %v\n", err)
		os.Exit(1)
	}

	var notes []*Note
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(*dir, e.Name())
		if note := parseNote(path); note.Identifier != "" {
			notes = append(notes, note)
		}
	}

	sort.Slice(notes, func(i, j int) bool {
		return notes[i].Identifier > notes[j].Identifier
	})

	w, err := acme.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating acme window: %v\n", err)
		os.Exit(1)
	}
	defer w.CloseFiles()

	w.Name("+Denote")
	w.Ctl("clean")
	w.Clear()

	var buf strings.Builder
	for _, n := range notes {
		title := n.FileTitle
		if title == "" {
			title = n.Title
		}
		if title == "" {
			title = "(untitled)"
		}
		
		buf.WriteString(title)
		if len(n.Keywords) > 0 {
			fmt.Fprintf(&buf, " (%s)", strings.Join(n.Keywords, ", "))
		}
		fmt.Fprintf(&buf, "\n%s\n\n", n.Path)
	}

	w.Write("body", []byte(buf.String()))
	w.Ctl("clean")
	w.Addr("#0")
	w.Ctl("dot=addr")
	w.Ctl("show")
}
