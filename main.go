package main

import (
	"denote/internal/denote"
	"denote/internal/fs"
	"denote/internal/ui"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unicode"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

const (
	wname = "/Denote/"
)

func main() {
	var err error
	var w *ui.Window
	args := os.Args[1:]
	if len(args) == 1 {
		if identifier, ok := strings.CutPrefix(args[0], "denote:"); ok {
			path, err := denote.IdentifierToPath(identifier)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error finding note: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(path)
			denote.Open(path)
			return
		} else {
			fmt.Println("Usage:")
			fmt.Println("Denote is designed to be used in Acme, however it does accept one argument:")
			fmt.Println()
			fmt.Println("\tDenote denote:<identifier> - opens denote file by identifier")
			fmt.Println()
		}
	}

	// 9p server startup
	if err := fs.StartServer(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to start fileserver: %v\n", err)
	}

	// start event consumer
	go consumeEvents()

	// open window
	if w, err = ui.WindowOpen(wname); err != nil {
		log.Fatal(err)
	}
	defer w.CloseFiles()

	if err = ui.TagSet(w, "Reset"); err != nil {
		log.Fatal(err)
	}

	// get initial results
	rs, err := denote.Search(fs.Filters{})
	if err != nil {
		panic(err)
	}
	fs.Sort(rs, fs.SortById, fs.SortOrderDesc)
	refreshWindow(w, rs)

	// event loop
	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			switch string(e.Text) {
			case "Reset":
				rs, err := denote.Search(fs.Filters{})
				if err != nil {
					panic(err)
				}
				fs.Sort(rs, fs.SortById, fs.SortOrderDesc)
				refreshWindow(w, rs)
			case "Look":
				// Use e.Arg for the search arguments
				performSearch(w, string(e.Arg))
			default:
				w.WriteEvent(e)
			}
		case 'l', 'L':
			w.WriteEvent(e)
		default:
			w.WriteEvent(e)
		}
	}
}

func performSearch(w *ui.Window, searchText string) {
	args := parseChordedArgs(searchText)

	var filterArgs []string
	sortBy := fs.SortById
	sortOrder := fs.SortOrderDesc

	for _, arg := range args {
		if sortSpec, ok := strings.CutPrefix(arg, "sort:"); ok {
			parts := strings.Split(sortSpec, ",")

			switch parts[0] {
			case "id", "date":
				sortBy = fs.SortById
			case "title":
				sortBy = fs.SortByTitle
			}

			if len(parts) > 1 {
				switch parts[1] {
				case "asc":
					sortOrder = fs.SortOrderAsc
				case "desc":
					sortOrder = fs.SortOrderDesc
				}
			} else {
				sortOrder = fs.SortOrderAsc
			}
		} else {
			filterArgs = append(filterArgs, arg)
		}
	}

	filters, err := fs.Filters{}.Parse(filterArgs)
	if err != nil {
		log.Printf("filter parse error: %v", err)
		return
	}

	rs, err := denote.Search(filters)
	if err != nil {
		log.Printf("search error: %v", err)
		return
	}
	fs.Sort(rs, sortBy, sortOrder)

	refreshWindow(w, rs)
}

func refreshWindow(w *ui.Window, rs fs.Results) {
	// write initial results to window
	content := ""
	for _, n := range rs {
		content += fmt.Sprintf("denote:%s | %s |", n.Identifier, n.Title)
		for _, t := range n.Tags {
			content += fmt.Sprintf("%s,", t)
		}
		content = strings.TrimSuffix(content, ",")
		content += "\n"
	}
	ui.BodyWrite(w, ",", []byte(content))
}

// parseChordedArgs parses space-separated arguments, handling quoted strings
func parseChordedArgs(s string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	escaped := false

	for i, r := range s {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		switch r {
		case '\\':
			if inQuote {
				// Look ahead to see if next char is quote or backslash
				if i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
					escaped = true
				} else {
					current.WriteRune(r)
				}
			} else {
				current.WriteRune(r)
			}
		case '"':
			inQuote = !inQuote
		case ' ', '\t', '\n':
			if inQuote {
				current.WriteRune(r)
			} else if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	// Add final argument if any
	if current.Len() > 0 {
		args = append(args, current.String())
	}

	// Trim whitespace from args
	for i := range args {
		args[i] = strings.TrimFunc(args[i], unicode.IsSpace)
	}

	return args
}

func consumeEvents() {
	for {
		err := fs.With9P(func(f *client.Fsys) error {
			// Open event file (blocking reads)
			fid, err := f.Open("event", plan9.OREAD)
			if err != nil {
				return fmt.Errorf("failed to open event file: %w", err)
			}
			defer fid.Close()

			// Read events in loop
			buf := make([]byte, 8192)
			for {
				n, err := fid.Read(buf)
				if err != nil {
					return fmt.Errorf("failed to read event: %w", err)
				}
				if n == 0 {
					continue
				}

				event := strings.TrimSpace(string(buf[:n]))

				// Parse event: "identifier action"
				parts := strings.Fields(event)
				if len(parts) != 2 {
					log.Printf("invalid event format: %s", event)
					continue
				}

				identifier := parts[0]
				action := parts[1]

				// Handle update events
				if action == "u" {
					if err := handleUpdateEvent(f, identifier); err != nil {
						log.Printf("error handling update for %s: %v", identifier, err)
					}
				}
			}
		})

		if err != nil {
			log.Printf("event consumer error: %v", err)
			time.Sleep(time.Second)
		}
	}
}

func handleUpdateEvent(f *client.Fsys, identifier string) error {
	// Read current metadata via 9P
	title, err := denote.ReadFile(f, identifier+"/title")
	if err != nil {
		return fmt.Errorf("failed to read title: %w", err)
	}

	keywords, err := denote.ReadFile(f, identifier+"/keywords")
	if err != nil {
		return fmt.Errorf("failed to read keywords: %w", err)
	}

	path, err := denote.ReadFile(f, identifier+"/path")
	if err != nil {
		return fmt.Errorf("failed to read path: %w", err)
	}

	ext, err := denote.ReadFile(f, identifier+"/extension")
	if err != nil {
		return fmt.Errorf("failed to read extension: %w", err)
	}

	// Parse tags from keywords
	var tags []string
	if keywords != "" {
		for _, tag := range strings.Split(keywords, ",") {
			tags = append(tags, strings.TrimSpace(tag))
		}
	}

	// Determine file type from extension
	var fileType string
	switch ext {
	case ".org":
		fileType = "org"
	case ".md":
		// Need to detect YAML vs TOML - default to YAML
		fileType = "md-yaml"
	case ".txt":
		fileType = "txt"
	default:
		fileType = "txt"
	}

	// Update front matter
	fm := &denote.FrontMatter{
		Title:      title,
		Tags:       tags,
		Identifier: identifier,
		FileType:   fileType,
	}

	if err := denote.UpdateFrontMatter(path, fm); err != nil {
		log.Printf("failed to update front matter for %s: %v", identifier, err)
	}

	// Rename file
	if _, err := denote.Rename(path, title, tags, identifier); err != nil {
		log.Printf("failed to rename file for %s: %v", identifier, err)
	}

	return nil
}
