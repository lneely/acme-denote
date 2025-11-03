package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"denote/internal/denote"
)

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
		if _, ok := denote.FrontMatterTemplates[fileType]; !ok {
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
	path, err := denote.CreateNote(dir, title, keywords, fileType, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating note: %v\n", err)
		os.Exit(1)
	}
	
	// Display the newly created note
	note := denote.ParseNote(path)
	if err := denote.DisplayNotes([]*denote.Note{note}); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func main() {
	dir := flag.String("dir", denote.DefaultDir(), "denote directory")
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

	var filters []*denote.Filter
	for _, arg := range filterArgs {
		filt, err := denote.ParseFilter(arg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		filters = append(filters, filt)
	}

	notes, err := denote.FindNotes(*dir, filters)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading directory: %v\n", err)
		os.Exit(1)
	}

	if err := denote.DisplayNotes(notes); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
