package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

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

func handleRename(args []string) {
	fullArg := strings.Join(args, " ")
	
	if fullArg == "" {
		fmt.Fprintf(os.Stderr, `Usage: rename /path/to/file ['Title'] [tag1 tag2 ...]

Rename a file using denote naming convention and optionally update front matter.

No arguments (just file path):
  - Extract title and tags from front matter (if present)
  - Apply denote naming convention to filename

Title provided:
  - Update front matter title (if file supports it)
  - Rename file with new title

Title and tags provided:
  - Update front matter title and tags (if file supports it)
  - Rename file with new title and tags

Examples:
  rename /path/to/file.md
  rename /path/to/file.md 'New Title'
  rename /path/to/file.md 'New Title' tag1 tag2

Note: Title must be quoted with single quotes.
`)
		os.Exit(0)
	}
	
	// Extract file path - try to find where it ends
	// Strategy: file path is everything before the first quoted title or until we find an existing file
	var filePath string
	var argsAfterPath string
	
	// Check if there's a quoted title
	titleRe := regexp.MustCompile(`'([^']+)'`)
	if m := titleRe.FindStringIndex(fullArg); m != nil {
		// Everything before the quote is the file path
		filePath = strings.TrimSpace(fullArg[:m[0]])
		argsAfterPath = strings.TrimSpace(fullArg[m[0]:])
	} else {
		// No quoted title - try to find where file path ends by checking what exists
		// Start with the whole string and work backwards
		parts := strings.Fields(fullArg)
		for i := len(parts); i > 0; i-- {
			testPath := strings.Join(parts[:i], " ")
			if _, err := os.Stat(testPath); err == nil {
				filePath = testPath
				if i < len(parts) {
					argsAfterPath = strings.Join(parts[i:], " ")
				}
				break
			}
		}
		if filePath == "" {
			fmt.Fprintf(os.Stderr, "error: no valid file path found\n")
			os.Exit(1)
		}
	}
	
	if !filepath.IsAbs(filePath) {
		var err error
		filePath, err = filepath.Abs(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid file path: %v\n", err)
			os.Exit(1)
		}
	}
	
	if _, err := os.Stat(filePath); err != nil {
		fmt.Fprintf(os.Stderr, "error: file not found: %s\n", filePath)
		os.Exit(1)
	}
	
	// Check if title is provided (in quotes)
	m := titleRe.FindStringSubmatch(argsAfterPath)
	
	var newTitle string
	var newTags []string
	
	if m != nil {
		// Title provided with quotes
		newTitle = m[1]
		
		// Extract tags (everything after the closing quote)
		afterTitle := strings.TrimSpace(argsAfterPath[strings.Index(argsAfterPath, m[0])+len(m[0]):])
		if afterTitle != "" {
			newTags = strings.Fields(afterTitle)
		}
	} else if argsAfterPath != "" {
		// No quotes - acme 2-1 chord passes everything as one string
		// Treat everything as title if no tags look like tags (lowercase single words)
		// For now, assume last words that are lowercase and short are tags
		argParts := strings.Fields(argsAfterPath)
		if len(argParts) > 0 {
			// Find where tags start (lowercase single words at the end)
			titleEnd := len(argParts)
			for i := len(argParts) - 1; i >= 0; i-- {
				word := argParts[i]
				// If word looks like a tag (lowercase, no spaces, short)
				if word == strings.ToLower(word) && len(word) < 20 && !strings.Contains(word, " ") {
					titleEnd = i
				} else {
					break
				}
			}
			
			// If we found potential tags
			if titleEnd < len(argParts) {
				newTitle = strings.Join(argParts[:titleEnd], " ")
				newTags = argParts[titleEnd:]
			} else {
				// Everything is the title
				newTitle = argsAfterPath
			}
		}
	}
	
	// Parse existing file
	note := denote.ParseNote(filePath)
	fm, err := denote.ParseFrontMatter(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
		os.Exit(1)
	}
	
	// Determine final title and tags
	finalTitle := newTitle
	finalTags := newTags
	finalIdentifier := note.Identifier
	
	if finalTitle == "" {
		// No title argument - use front matter or filename
		if fm.Title != "" {
			finalTitle = fm.Title
		} else if note.FileTitle != "" {
			finalTitle = note.FileTitle
		} else if note.Title != "" {
			finalTitle = note.Title
		} else {
			// Use filename without extension as fallback
			finalTitle = strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
		}
	}
	
	if len(finalTags) == 0 {
		// No tags argument - use front matter or filename tags
		if len(fm.Tags) > 0 {
			finalTags = fm.Tags
		} else if len(note.Keywords) > 0 {
			finalTags = note.Keywords
		}
	}
	
	if finalIdentifier == "" {
		// No identifier in filename - use from front matter or generate new
		if fm.Identifier != "" {
			finalIdentifier = fm.Identifier
		} else {
			finalIdentifier = time.Now().Format("20060102T150405")
		}
	}
	
	// Update front matter if file supports it and we have new values
	if fm.FileType != "" && (newTitle != "" || len(newTags) > 0) {
		fm.Title = finalTitle
		fm.Tags = finalTags
		fm.Identifier = finalIdentifier
		
		if err := denote.UpdateFrontMatter(filePath, fm); err != nil {
			fmt.Fprintf(os.Stderr, "error updating front matter: %v\n", err)
			os.Exit(1)
		}
	}
	
	// Rename file
	newPath, err := denote.RenameNote(filePath, finalTitle, finalTags, finalIdentifier)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error renaming file: %v\n", err)
		os.Exit(1)
	}
	
	// Display the renamed note
	renamedNote := denote.ParseNote(newPath)
	if err := denote.DisplayNotes([]*denote.Note{renamedNote}); err != nil {
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
  Denote [filters...] [sort:TYPE]      List notes matching filters
  Denote new 'Title' [tags...]         Create a new note
  Denote rename /path ['Title'] [tags...]  Rename a note
  Denote ?                             Show this help

Filters:
  date:/regex/     Match date/identifier
  title:/regex/    Match title
  tag:/regex/      Match tags
  /regex/          Match any field
  !filter          Negate filter
  plain-text       Exact match (no regex)
  sort:TYPE        Sort results (id, date, title) - default: id

Examples:
  Denote                                List all notes (sorted by id)
  Denote sort:title                     List all notes sorted by title
  Denote date:/2025.*/                  Notes from 2025
  Denote tag:meeting sort:title         Notes tagged 'meeting' sorted by title
  Denote tag:journal                    List journal entries
  Denote date:/202510.*/ !tag:journal   October 2025, not journal
  Denote new 'My Note' tag1 tag2        Create new note
  Denote rename /path/file.md 'New Title' tag1 tag2

Options:
  -dir path    Denote directory (default: $DENOTE_DIR or ~/doc)

For 'new' command help: Denote new
For 'rename' command help: Denote rename
`)
		os.Exit(0)
	}

	// Check if this is a "new" command
	if len(filterArgs) > 0 && filterArgs[0] == "new" {
		handleNew(*dir, filterArgs[1:])
		return
	}

	// Check if this is a "rename" command
	if len(filterArgs) > 0 && filterArgs[0] == "rename" {
		handleRename(filterArgs[1:])
		return
	}

	var filters []*denote.Filter
	sortType := denote.SortByID // Default sort

	for _, arg := range filterArgs {
		if strings.HasPrefix(arg, "sort:") {
			sortStr := strings.TrimPrefix(arg, "sort:")
			var err error
			sortType, err = denote.ParseSortType(sortStr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			continue
		}
		
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

	// Apply sorting
	denote.SortNotes(notes, sortType)

	if err := denote.DisplayNotes(notes); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
