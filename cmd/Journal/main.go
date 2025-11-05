package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denote/internal/denote"
)

func journalDir() string {
	base := denote.DefaultDir()
	subdir := os.Getenv("JOURNAL_DIR")
	if subdir == "" {
		subdir = "journal"
	}
	return filepath.Join(base, subdir)
}

func formatJournalTitle(t time.Time) string {
	return t.Format("Monday 2 January 2006 15:04")
}

func parseDate(dateStr string) (time.Time, error) {
	return time.Parse("20060102", dateStr)
}

func main() {
	fileType := flag.String("f", "md-yaml", "file type (org, md-yaml, md-toml, txt)")
	flag.Parse()

	args := flag.Args()
	dir := journalDir()

	// Ensure journal directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating journal directory: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()

	// Determine target date and action
	var targetDate time.Time
	
	if len(args) == 0 {
		// No args - today
		targetDate = now
	} else if args[0] == "add" {
		// Journal add [YYYYMMDD] - always create new entry
		var identifier string

		if len(args) > 1 {
			var err error
			targetDate, err = parseDate(args[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: invalid date format (use YYYYMMDD): %v\n", err)
				os.Exit(1)
			}
			identifier = targetDate.Format("20060102") + "T" + now.Format("150405")
			// Use target date but with current time for the title
			targetDate = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(),
				now.Hour(), now.Minute(), now.Second(), 0, now.Location())
		} else {
			targetDate = now
			identifier = now.Format("20060102T150405")
		}

		title := formatJournalTitle(targetDate)
		path, err := denote.CreateNote(dir, title, []string{"journal"}, *fileType, identifier)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error creating journal entry: %v\n", err)
			os.Exit(1)
		}
		note := denote.ParseNote(path)
		if err := denote.DisplayNotes([]*denote.Note{note}); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	} else {
		// Assume it's a date argument (YYYYMMDD)
		var err error
		targetDate, err = parseDate(args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid date format (use YYYYMMDD): %v\n", err)
			os.Exit(1)
		}
		// Use target date but with current time for the title
		targetDate = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(),
			now.Hour(), now.Minute(), now.Second(), 0, now.Location())
	}
	
	// Find journal entries for target date by searching title
	// Title format: "Wednesday 5 November 2025 11:01"
	// Filename format: wednesday-5-november-2025
	titlePattern := strings.ToLower(targetDate.Format("monday-2-january-2006"))
	titleFilter, _ := denote.ParseFilter(fmt.Sprintf("title:/%s/", titlePattern))
	tagFilter, _ := denote.ParseFilter("tag:journal")
	
	notes, err := denote.FindNotes(dir, []*denote.Filter{titleFilter, tagFilter})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error searching for journal: %v\n", err)
		os.Exit(1)
	}
	
	if len(notes) > 0 {
		if err := denote.DisplayNotes(notes); err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
		return
	}
	
	// No entries found - create new entry for target date
	title := formatJournalTitle(targetDate)
	// Identifier is always the actual creation time
	identifier := now.Format("20060102T150405")
	path, err := denote.CreateNote(dir, title, []string{"journal"}, *fileType, identifier)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating journal entry: %v\n", err)
		os.Exit(1)
	}
	note := denote.ParseNote(path)
	if err := denote.DisplayNotes([]*denote.Note{note}); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

