package main

import (
	"flag"
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
var orgTitleRe = regexp.MustCompile(`(?m)^#\+title:\s*(.+)$`)
var mdYamlTitleRe = regexp.MustCompile(`(?ms)^---\n.*?^title:\s*(.+?)$.*?^---`)
var mdHeaderRe = regexp.MustCompile(`(?m)^#\s+(.+)$`)

// File extensions that support denote front matter (per denote.el)
var denoteFrontMatterFormats = []string{".org", ".md", ".txt"}

var frontMatterTemplates = map[string]string{
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

var fileExtensions = map[string]string{
	"org":     ".org",
	"md-yaml": ".md",
	"md-toml": ".md",
	"txt":     ".txt",
}

func formatKeywords(keywords []string, fileType string) string {
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

func createNote(dir, title string, keywords []string, fileType string) (string, error) {
	// Generate identifier
	identifier := time.Now().Format("20060102T150405")
	
	// Format file name
	titleSlug := strings.ReplaceAll(strings.ToLower(title), " ", "-")
	titleSlug = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(titleSlug, "")
	
	var keywordsPart string
	if len(keywords) > 0 {
		keywordsPart = "__" + strings.Join(keywords, "_")
	}
	
	ext := fileExtensions[fileType]
	filename := fmt.Sprintf("%s--%s%s%s", identifier, titleSlug, keywordsPart, ext)
	path := filepath.Join(dir, filename)
	
	// Generate front matter
	template := frontMatterTemplates[fileType]
	dateStr := time.Now().Format("2006-01-02 Mon 15:04")
	keywordsStr := formatKeywords(keywords, fileType)
	
	content := fmt.Sprintf(template, title, dateStr, keywordsStr, identifier, "")
	
	// Write file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	
	return path, nil
}

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

func handleNew(dir string, args []string) {
	// Rejoin args in case acme split them oddly
	fullArg := strings.Join(args, " ")
	
	// Show usage if no arguments
	if fullArg == "" {
		fmt.Fprintf(os.Stderr, `Usage: new [-f type] 'Title' [tag1 tag2 ...]

Create a new denote note with the given title and optional tags.

Options:
  -f type    File type (default: md-yaml)
             Supported: org, md-yaml, md-toml, txt

Examples:
  new 'My Document Title' tag1 tag2
  new -f org 'Meeting Notes' meeting project
  new -f txt 'Journal Entry' journal

Note: Title must be quoted with single quotes.
`)
		os.Exit(0)
	}
	
	fileType := "md-yaml"
	
	// Check for -f flag
	if strings.HasPrefix(fullArg, "-f ") {
		parts := strings.SplitN(fullArg, " ", 3)
		if len(parts) < 3 {
			fmt.Fprintf(os.Stderr, "error: missing arguments after -f\nUsage: new [-f type] 'Title' [tag1 tag2 ...]\n")
			os.Exit(1)
		}
		fileType = parts[1]
		fullArg = parts[2]
		if _, ok := frontMatterTemplates[fileType]; !ok {
			fmt.Fprintf(os.Stderr, "error: unsupported file type: %s\nSupported: org, md-yaml, md-toml, txt\n", fileType)
			os.Exit(1)
		}
	}
	
	// Extract title between single quotes
	titleRe := regexp.MustCompile(`'([^']+)'`)
	m := titleRe.FindStringSubmatch(fullArg)
	if m == nil {
		fmt.Fprintf(os.Stderr, "error: title must be quoted with single quotes\nUsage: new [-f type] 'Title' [tag1 tag2 ...]\n")
		os.Exit(1)
	}
	title := m[1]
	
	// Extract keywords (everything after the closing quote)
	afterTitle := strings.TrimSpace(fullArg[strings.Index(fullArg, m[0])+len(m[0]):])
	var keywords []string
	if afterTitle != "" {
		keywords = strings.Fields(afterTitle)
	}
	
	// Create the note
	path, err := createNote(dir, title, keywords, fileType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating note: %v\n", err)
		os.Exit(1)
	}
	
	// Display the newly created note
	note := parseNote(path)
	displayNotes([]*Note{note})
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

	// Show help if ? is passed
	if len(filterArgs) == 1 && filterArgs[0] == "?" {
		fmt.Fprintf(os.Stderr, `Denote - manage denote notes in acme

Usage:
  Denote [filters...]           List notes matching filters
  Denote new 'Title' [tags...]  Create a new note
  Denote ?                      Show this help

Filters:
  date:/regex/     Match date/identifier
  title:/regex/    Match title
  tag:/regex/      Match tags
  /regex/          Match any field
  !filter          Negate filter
  plain-text       Exact match (no regex)

Examples:
  Denote                              List all notes
  Denote date:/2025.*/                Notes from 2025
  Denote tag:meeting                  Notes tagged 'meeting'
  Denote date:/202510.*/ !tag:journal October 2025, not journal
  Denote new 'My Note' tag1 tag2      Create new note

Options:
  -dir path    Denote directory (default: $DENOTE_DIR or ~/doc)

For 'new' command help: Denote new
`)
		os.Exit(0)
	}

	// Check if this is a "new" command
	if len(filterArgs) > 0 && filterArgs[0] == "new" {
		handleNew(*dir, filterArgs[1:])
		return
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

	displayNotes(notes)
}

func displayNotes(notes []*Note) {
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
