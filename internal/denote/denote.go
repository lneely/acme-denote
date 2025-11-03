package denote

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

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

// ParseNote parses a denote filename into a Note struct
func ParseNote(path string) *Note {
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
	note.FileTitle = ExtractTitle(path)

	return note
}

// Filter represents a search filter
type Filter struct {
	field  string // "date", "title", "tag", or "" for any
	re     *regexp.Regexp
	negate bool
}

// ParseFilter parses a filter string into a Filter
func ParseFilter(arg string) (*Filter, error) {
	negate := strings.HasPrefix(arg, "!")
	if negate {
		arg = strings.TrimPrefix(arg, "!")
	}
	
	m := regexp.MustCompile(`^(?:(date|title|tag):)?(.+)$`).FindStringSubmatch(arg)
	if m == nil {
		return nil, fmt.Errorf("invalid filter syntax: %s", arg)
	}
	
	pattern := m[2]
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimPrefix(strings.TrimSuffix(pattern, "/"), "/")
	} else {
		pattern = regexp.QuoteMeta(pattern)
	}
	
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex: %v", err)
	}
	return &Filter{field: m[1], re: re, negate: negate}, nil
}

// Matches checks if a note matches this filter
func (f *Filter) Matches(n *Note) bool {
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

// FindNotes searches for notes in a directory matching filters
func FindNotes(dir string, filters []*Filter) ([]*Note, error) {
	var notes []*Note
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if note := ParseNote(path); note.Identifier != "" {
			match := true
			for _, filt := range filters {
				if !filt.Matches(note) {
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
		return nil, err
	}

	sort.Slice(notes, func(i, j int) bool {
		return notes[i].Identifier > notes[j].Identifier
	})

	return notes, nil
}

// DisplayNotes displays notes in an acme window
func DisplayNotes(notes []*Note) error {
	w, err := acme.New()
	if err != nil {
		return fmt.Errorf("error creating acme window: %v", err)
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
	
	return nil
}


var FrontMatterTemplates = map[string]string{
	"org": `#+title:      %s
#+date:       %s
#+filetags:   %s
#+identifier: %s
#+signature:  %s

`,
	"md-yaml": `---
title:      %s
date:       %s
tags:       %s
identifier: %s
signature:  %s
---

`,
	"md-toml": `+++
title      = %s
date       = %s
tags       = %s
identifier = %s
signature  = %s
+++

`,
	"txt": `title:      %s
date:       %s
tags:       %s
identifier: %s
signature:  %s
---------------------------

`,
}

var FileExtensions = map[string]string{
	"org":     ".org",
	"md-yaml": ".md",
	"md-toml": ".md",
	"txt":     ".txt",
}

func FormatKeywords(keywords []string, fileType string) string {
	if len(keywords) == 0 {
		return ""
	}
	switch fileType {
	case "org":
		return ":" + strings.Join(keywords, ":") + ":"
	case "md-yaml", "md-toml":
		return "[" + strings.Join(keywords, ", ") + "]"
	default:
		return strings.Join(keywords, " ")
	}
}

func CreateNote(dir, title string, keywords []string, fileType, identifier string) (string, error) {
	// Use provided identifier or generate new one
	if identifier == "" {
		identifier = time.Now().Format("20060102T150405")
	}
	
	// Format file name
	titleSlug := strings.ReplaceAll(strings.ToLower(title), " ", "-")
	titleSlug = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(titleSlug, "")
	
	var keywordsPart string
	if len(keywords) > 0 {
		keywordsPart = "__" + strings.Join(keywords, "_")
	}
	
	ext := FileExtensions[fileType]
	filename := fmt.Sprintf("%s--%s%s%s", identifier, titleSlug, keywordsPart, ext)
	path := filepath.Join(dir, filename)
	
	// Generate front matter
	template := FrontMatterTemplates[fileType]
	dateStr := time.Now().Format("2006-01-02 Mon 15:04")
	keywordsStr := FormatKeywords(keywords, fileType)
	
	content := fmt.Sprintf(template, title, dateStr, keywordsStr, identifier, "")
	
	// Write file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	
	return path, nil
}

func DefaultDir() string {
	if d := os.Getenv("DENOTE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_DOCUMENTS_DIR"); d != "" {
		return d
	}
	return filepath.Join(os.Getenv("HOME"), "doc")
}

var orgTitleRe = regexp.MustCompile(`(?m)^#\+title:\s*(.+)$`)
var mdYamlTitleRe = regexp.MustCompile(`(?ms)^---\n.*?^title:\s*(.+?)$.*?^---`)
var mdHeaderRe = regexp.MustCompile(`(?m)^#\s+(.+)$`)

func ExtractTitle(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".org" && ext != ".md" && ext != ".txt" {
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
