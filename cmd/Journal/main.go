package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denote/internal/denote"
)

func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "acme-denote")
}

func readJournalConfig() (bool, error) {
	configPath := filepath.Join(configDir(), "journal.cfg")

	file, err := os.Open(configPath)
	if os.IsNotExist(err) {
		// File doesn't exist, default to false
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		if key == "always_encrypt" {
			return value == "true", nil
		}
	}

	return false, scanner.Err()
}

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

func parseRelativeDate(relStr string, now time.Time) (time.Time, error) {
	// Parse relative date like +1d, -3d, +2h, -5m, +30s
	if len(relStr) < 3 {
		return time.Time{}, fmt.Errorf("invalid relative date format")
	}

	sign := relStr[0]
	if sign != '+' && sign != '-' {
		return time.Time{}, fmt.Errorf("relative date must start with + or -")
	}

	unit := relStr[len(relStr)-1]
	numStr := relStr[1 : len(relStr)-1]

	num := 0
	_, err := fmt.Sscanf(numStr, "%d", &num)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid number in relative date: %v", err)
	}

	if sign == '-' {
		num = -num
	}

	var result time.Time
	switch unit {
	case 'd':
		result = now.AddDate(0, 0, num)
	case 'h':
		result = now.Add(time.Duration(num) * time.Hour)
	case 'm':
		result = now.Add(time.Duration(num) * time.Minute)
	case 's':
		result = now.Add(time.Duration(num) * time.Second)
	default:
		return time.Time{}, fmt.Errorf("invalid unit: %c (use d, h, m, or s)", unit)
	}

	return result, nil
}



func main() {
	// Check for relative date arguments before flag parsing (to avoid -1d being treated as flag)
	var relativeArg string
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if (strings.HasPrefix(arg, "+") || strings.HasPrefix(arg, "-")) &&
		   len(arg) >= 3 &&
		   (arg[len(arg)-1] == 'd' || arg[len(arg)-1] == 'h' || arg[len(arg)-1] == 'm' || arg[len(arg)-1] == 's') {
			// This is a relative date, remove it from os.Args before flag parsing
			relativeArg = arg
			os.Args = append(os.Args[:1], os.Args[2:]...)
		}
	}

	fileType := flag.String("f", "md-yaml", "file type (org, md-yaml, md-toml, txt)")
	encrypt := flag.Bool("e", false, "encrypt the file")
	flag.Parse()

	args := flag.Args()

	// If we had a relative arg, prepend it back to args
	if relativeArg != "" {
		args = append([]string{relativeArg}, args...)
	}

	// Read configuration
	alwaysEncrypt, err := readJournalConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading journal config: %v\n", err)
		os.Exit(1)
	}

	// Determine if we should encrypt (either flag or config)
	shouldEncrypt := *encrypt || alwaysEncrypt

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
		var path string
		var err error

		if shouldEncrypt {
			path, err = denote.CreateEncryptedNote(dir, title, []string{"journal"}, *fileType, identifier)
		} else {
			path, err = denote.CreateNote(dir, title, []string{"journal"}, *fileType, identifier)
		}

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
		// Check if it's a relative date (+1d, -3h, etc.) or absolute date (YYYYMMDD)
		var err error
		if strings.HasPrefix(args[0], "+") || strings.HasPrefix(args[0], "-") {
			// Relative date
			targetDate, err = parseRelativeDate(args[0], now)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: invalid relative date format: %v\n", err)
				os.Exit(1)
			}
		} else {
			// Absolute date (YYYYMMDD)
			targetDate, err = parseDate(args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: invalid date format (use YYYYMMDD or +/-Nd/h/m/s): %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Find journal entries for target date by searching title
	// Title format in file: "Wednesday 5 November 2025 11:01"
	// Title in Note struct: "wednesday 5 november 2025 1101" (hyphens converted to spaces)
	// Search pattern: "wednesday 5 november 2025" (date only, no time, with spaces)
	titlePattern := strings.ToLower(targetDate.Format("Monday 2 January 2006"))
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
	// Use target date but with current time for the title
	targetDate = time.Date(targetDate.Year(), targetDate.Month(), targetDate.Day(),
		now.Hour(), now.Minute(), now.Second(), 0, now.Location())
	title := formatJournalTitle(targetDate)
	// Identifier is always the actual creation time
	identifier := now.Format("20060102T150405")

	var path string
	if shouldEncrypt {
		path, err = denote.CreateEncryptedNote(dir, title, []string{"journal"}, *fileType, identifier)
	} else {
		path, err = denote.CreateNote(dir, title, []string{"journal"}, *fileType, identifier)
	}

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

