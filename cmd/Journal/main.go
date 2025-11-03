package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
	
	if len(args) == 0 {
		// Journal - find or create today's entry
		dateFilter, _ := denote.ParseFilter(fmt.Sprintf("date:%s", now.Format("20060102")))
		tagFilter, _ := denote.ParseFilter("tag:journal")
		
		notes, err := denote.FindNotes(dir, []*denote.Filter{dateFilter, tagFilter})
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
		
		// Create new entry for today
		title := formatJournalTitle(now)
		path, err := denote.CreateNote(dir, title, []string{"journal"}, *fileType, "")
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
	}
	
	if args[0] == "add" {
		// Journal add [YYYYMMDD]
		var targetDate time.Time
		var identifier string
		
		if len(args) > 1 {
			var err error
			targetDate, err = parseDate(args[1])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: invalid date format (use YYYYMMDD): %v\n", err)
				os.Exit(1)
			}
			identifier = targetDate.Format("20060102") + "T" + now.Format("150405")
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
	}
	
	fmt.Fprintf(os.Stderr, "Usage:\n  Journal           Find or create today's entry\n  Journal add       Create additional entry for today\n  Journal add DATE  Create entry for specific date (YYYYMMDD)\n")
	os.Exit(1)
}

