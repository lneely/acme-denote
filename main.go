package main

import (
	"denote/internal/denote"
	"denote/internal/fs"
	"denote/internal/sync"
	"denote/internal/ui"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
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
	go denote.EventListener()

	// start acme log watcher
	go sync.WatchAcmeLog()

	// open window
	if w, err = ui.WindowOpen(wname); err != nil {
		log.Fatal(err)
	}
	defer w.CloseFiles()

	if err = ui.TagSet(w, "New Reset Sync Put"); err != nil {
		log.Fatal(err)
	}

	// get initial results
	rs, err := denote.Search(fs.Filters{})
	if err != nil {
		panic(err)
	}
	fs.Sort(rs, fs.SortById, fs.SortOrderDesc)
	refreshWindow(w, rs)

	// listen for 'n' events to refresh window
	go func() {
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
					parts := strings.Fields(event)
					if len(parts) >= 2 && parts[1] == "n" {
						// Refresh window with new results
						rs, err := denote.Search(fs.Filters{})
						if err != nil {
							log.Printf("error refreshing after new file: %v", err)
							continue
						}
						fs.Sort(rs, fs.SortById, fs.SortOrderDesc)
						refreshWindow(w, rs)
					}
				}
			})

			if err != nil {
				log.Printf("window event listener error: %v", err)
			}
		}
	}()

	ui.WindowDirty(w, false)
	ui.DotToAddr(w, "#0")

	// event loop
	for e := range w.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			switch string(e.Text) {
			case "New":
				// Read chorded argument
				input := strings.TrimSpace(string(e.Arg))
				if input == "" {
					log.Printf("New: no input provided")
					break
				}

				// Write to denote/new via 9P
				if err := fs.With9P(func(f *client.Fsys) error {
					return fs.WriteFile(f, "new", input)
				}); err != nil {
					log.Printf("failed to create new file: %v", err)
				}
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
			case "Sync":
				if err := sync.SyncAll(); err != nil {
					log.Printf("sync error: %v", err)
				}
			case "Put":
				body, err := w.ReadAll("body")
				if err != nil {
					log.Printf("failed to read window body: %v", err)
					break
				}

				// Parse the window content
				var emptyResults fs.Results
				updated, err := emptyResults.FromString(string(body))
				if err != nil {
					log.Printf("failed to parse window: %v", err)
					break
				}

				// Get current results and apply changes
				current, err := denote.Search(fs.Filters{})
				if err != nil {
					log.Printf("failed to get current results: %v", err)
					break
				}

				// Validate and apply changes via individual file writes
				if err := fs.With9P(func(f *client.Fsys) error {
					return applyIndexChanges(f, current, updated)
				}); err != nil {
					log.Printf("failed to apply changes: %v", err)
				}
			default:
				w.WriteEvent(e)
			}
		case 'l', 'L':
			// Check if text matches identifier pattern
			text := string(e.Text)
			if isIdentifier(text) {
				// Plumb with denote: prefix
				cmd := fmt.Sprintf("plumb 'denote:%s'", text)
				if err := exec.Command("rc", "-c", cmd).Run(); err != nil {
					log.Printf("failed to plumb identifier: %v", err)
				}
			} else {
				w.WriteEvent(e)
			}
		default:
			w.WriteEvent(e)
		}
	}
}

func performSearch(w *ui.Window, searchText string) {
	args := parseArgs(searchText)

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
	ui.BodyWrite(w, ",", rs.Bytes())
	ui.DotToAddr(w, "#0")
}

// parseArgs parses space-separated arguments, handling quoted strings
func parseArgs(s string) []string {
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

func applyIndexChanges(f *client.Fsys, current, updated fs.Results) error {
	// Validate entry count matches
	if len(updated) != len(current) {
		return fmt.Errorf("entry count mismatch: got %d, expected %d", len(updated), len(current))
	}

	// Build map of current state
	currentMap := make(map[string]*fs.Metadata)
	for _, m := range current {
		currentMap[m.Identifier] = m
	}

	// Write title and keywords for changed entries
	for _, upd := range updated {
		orig, exists := currentMap[upd.Identifier]
		if !exists {
			return fmt.Errorf("identifier '%s' not found", upd.Identifier)
		}

		// Check if anything changed
		titleChanged := orig.Title != upd.Title
		tagsChanged := !fs.SlicesEqual(orig.Tags, upd.Tags)

		if titleChanged || tagsChanged {
			// Write both title and keywords
			titlePath := upd.Identifier + "/title"
			if err := fs.WriteFile(f, titlePath, upd.Title); err != nil {
				return fmt.Errorf("failed to write title for %s: %w", upd.Identifier, err)
			}

			keywordsPath := upd.Identifier + "/keywords"
			keywords := strings.Join(upd.Tags, ",")
			if err := fs.WriteFile(f, keywordsPath, keywords); err != nil {
				return fmt.Errorf("failed to write keywords for %s: %w", upd.Identifier, err)
			}
		}
	}

	return nil
}

var identifierPattern = regexp.MustCompile(`^\d{8}T\d{6}$`)

func isIdentifier(s string) bool {
	return identifierPattern.MatchString(s)
}
