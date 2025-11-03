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
var mdYamlTitleRe = regexp.MustCompile(`(?ms)^---\n.*?^title:\s*(.+?)$.*?^---`)
var mdHeaderRe = regexp.MustCompile(`(?m)^#\s+(.+)$`)

// File extensions that support denote front matter (per denote.el)
var denoteFrontMatterFormats = []string{".org", ".md", ".txt"}

func extractTitle(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	supported := false
	for _, fmt := range denoteFrontMatterFormats {
		if ext == fmt {
			supported = true
			break
		}
	}
	if !supported {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := string(data)
	
	// Try org-mode #+title: first, then fall back to first heading
	if ext == ".org" {
		if m := orgTitleRe.FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
		// Fallback to first heading (lines starting with *)
		if m := regexp.MustCompile(`(?m)^\*+\s+(.+)$`).FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	
	// Try markdown YAML front matter title: first, then fall back to # header
	if ext == ".md" {
		if m := mdYamlTitleRe.FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(strings.Trim(m[1], `"`))
		}
		if m := mdHeaderRe.FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
	}
	
	return ""
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

type filter struct {
	field  string // "date", "title", "tag", or "" for any
	re     *regexp.Regexp
	negate bool
}

func parseFilter(arg string) (*filter, error) {
	// Check for negation prefix
	negate := strings.HasPrefix(arg, "!")
	if negate {
		arg = strings.TrimPrefix(arg, "!")
	}
	
	// Match field:/regex/ or field:text or just text/regex
	m := regexp.MustCompile(`^(?:(date|title|tag):)?(.+)$`).FindStringSubmatch(arg)
	if m == nil {
		return nil, fmt.Errorf("invalid filter syntax: %s\nSupported: [!]date:/regex/, [!]title:/regex/, [!]tag:/regex/, or plain text", arg)
	}
	
	pattern := m[2]
	// If pattern is wrapped in /.../, treat as regex
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimPrefix(strings.TrimSuffix(pattern, "/"), "/")
	} else {
		// Plain text - escape for exact match
		pattern = regexp.QuoteMeta(pattern)
	}
	
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %v", err)
	}
	return &filter{field: m[1], re: re, negate: negate}, nil
}

func (f *filter) matches(n *Note) bool {
	result := false
	switch f.field {
	case "date":
		result = f.re.MatchString(n.Identifier)
	case "title":
		title := n.Title
		if title == "" {
			title = n.FileTitle
		}
		result = f.re.MatchString(title)
	case "tag":
		// For tags, check if ANY tag matches
		for _, kw := range n.Keywords {
			if f.re.MatchString(kw) {
				result = true
				break
			}
		}
	default: // any field
		if f.re.MatchString(n.Identifier) {
			result = true
		} else {
			title := n.Title
			if title == "" {
				title = n.FileTitle
			}
			if f.re.MatchString(title) {
				result = true
			} else {
				for _, kw := range n.Keywords {
					if f.re.MatchString(kw) {
						result = true
						break
					}
				}
			}
		}
	}
	if f.negate {
		return !result
	}
	return result
}

func main() {
	dir := flag.String("dir", defaultDir(), "denote directory")
	flag.Parse()

	// Split args on whitespace to handle acme mouse chording
	var filterArgs []string
	for _, arg := range flag.Args() {
		filterArgs = append(filterArgs, strings.Fields(arg)...)
	}

	var filters []*filter
	for _, arg := range filterArgs {
		filt, err := parseFilter(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		filters = append(filters, filt)
	}

	var notes []*Note
	err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if note := parseNote(path); note.Identifier != "" {
			// All filters must match (AND logic)
			match := true
			for _, filt := range filters {
				if !filt.matches(note) {
					match = false
					break
				}
			}
			if match {
				notes = append(notes, note)
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading directory: %v\n", err)
		os.Exit(1)
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
