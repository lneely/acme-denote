package main

import (
	"denote/internal/metadata"
	p9client "denote/internal/p9/client"
	p9server "denote/internal/p9/server"
	"denote/internal/sync"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"9fans.net/go/acme"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

const (
	wname = "/Denote/"
	ftype = "md-yaml"
)

var denoteDir = os.Getenv("HOME") + "/doc"

// Helper functions for 9P operations

// readIndex reads and parses the index from 9P server.
func readIndex(f *client.Fsys) (metadata.Results, error) {
	indexContent, err := p9client.ReadFile(f, "index")
	if err != nil {
		return nil, fmt.Errorf("failed to read index: %w", err)
	}

	var results metadata.Results
	results, err = results.FromString(indexContent)
	if err != nil {
		return nil, err
	}

	// Populate Path for each note
	for _, note := range results {
		path, err := p9client.ReadFile(f, "n/"+note.Identifier+"/path")
		if err != nil {
			return nil, fmt.Errorf("failed to read path for %s: %w", note.Identifier, err)
		}
		note.Path = path
	}

	return results, nil
}

// setFilter sets or clears the filter on the 9P server.
func setFilter(f *client.Fsys, filterQuery string) error {
	cmd := "filter"
	if filterQuery != "" {
		cmd = "filter " + filterQuery
	}
	return p9client.WriteFile(f, "ctl", cmd)
}

// Event handling

func eventListener() {
	for {
		err := p9client.With9P(func(f *client.Fsys) error {
			fid, err := f.Open("event", plan9.OREAD)
			if err != nil {
				return fmt.Errorf("failed to open event file: %w", err)
			}
			defer fid.Close()

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
				if len(parts) != 2 {
					log.Printf("invalid event format: %s", event)
					continue
				}

				identifier := parts[0]
				action := parts[1]

				switch action {
				case "u":
					if err := sync.HandleUpdateEvent(f, identifier, denoteDir); err != nil {
						log.Printf("error handling update for %s: %v", identifier, err)
					}
				case "r":
					if err := sync.HandleRenameEvent(f, identifier, denoteDir); err != nil {
						log.Printf("error handling rename for %s: %v", identifier, err)
					}
				case "n":
					if err := sync.HandleNewEvent(f, identifier, denoteDir); err != nil {
						log.Printf("error handling new for %s: %v", identifier, err)
					}
				case "d":
					if err := sync.HandleDeleteEvent(identifier, denoteDir); err != nil {
						log.Printf("error handling delete for %s: %v", identifier, err)
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


func main() {
	var err error
	var w *acme.Win
	args := os.Args[1:]
	if len(args) == 1 {
		if identifier, ok := strings.CutPrefix(args[0], "denote:"); ok {
			// Find note by identifier using 9P
			var notePath string
			err := p9client.With9P(func(f *client.Fsys) error {
				// Set filter to find the identifier
				if err := p9client.WriteFile(f, "ctl", "filter date:"+identifier); err != nil {
					return err
				}
				defer p9client.WriteFile(f, "ctl", "filter") // Clear filter

				// Read index
				indexContent, err := p9client.ReadFile(f, "index")
				if err != nil {
					return err
				}

				// Parse index
				var results metadata.Results
				results, err = results.FromString(indexContent)
				if err != nil {
					return err
				}

				if len(results) == 0 {
					return fmt.Errorf("no note found with identifier %s", identifier)
				}

				// Read path for first result
				notePath, err = p9client.ReadFile(f, "n/"+results[0].Identifier+"/path")
				return err
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "error finding note: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(notePath)
			if err := exec.Command("plumb", notePath).Run(); err != nil {
				log.Printf("failed to plumb identifier: %v", err)
			}
			return
		} else {
			fmt.Println("Usage:")
			fmt.Println("Denote is designed to be used in Acme, however it does accept one argument:")
			fmt.Println()
			fmt.Println("\tDenote <identifier> - opens denote file by identifier")
			fmt.Println()
		}
	}

	// Load metadata from filesystem
	notes, err := sync.LoadAll(denoteDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load notes: %v\n", err)
	}

	// 9p server startup with pre-loaded data
	if err := p9server.StartServer(notes); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to start fileserver: %v\n", err)
	}

	// start event consumer
	go eventListener()

	// start acme log watcher
	go sync.WatchAcmeLog()

	// open window
	if w = acme.Show(wname); w == nil {
		if w, err = acme.New(); err != nil {
			log.Fatal(err)
		}
		if err = w.Name(wname); err != nil {
			w.Del(true)
			log.Fatal(fmt.Errorf("failed to set window name: %w", err))
		}
	}
	defer w.CloseFiles()

	if _, err = w.Write("tag", []byte("New Put Remove Reset Sync")); err != nil {
		w.Del(true)
		log.Fatal(fmt.Errorf("failed to set tag: %w", err))
	}

	// get initial results (clear any filter, read index)
	var rs metadata.Results
	err = p9client.With9P(func(f *client.Fsys) error {
		if err := setFilter(f, ""); err != nil {
			return err
		}
		var err error
		rs, err = readIndex(f)
		return err
	})
	if err != nil {
		panic(err)
	}
	metadata.Sort(rs, metadata.SortById, metadata.SortOrderDesc)
	refreshWindow(w, rs)

	// listen for 'n' events to refresh window
	go func() {
		for {
			err := p9client.With9P(func(f *client.Fsys) error {
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
					if len(parts) >= 2 {
						if parts[1] == "n" || parts[1] == "d" {
							refreshWindowWithDefaults(w)
						}
					}
				}
			})

			if err != nil {
				log.Printf("window event listener error: %v", err)
			}
		}
	}()

	w.Ctl("clean")
	w.Addr("#0")
	w.Ctl("dot=addr")
	w.Ctl("show")

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

				// Parse title and tags from input: 'title' tag1,tag2
				if !strings.HasPrefix(input, "'") {
					log.Printf("New: title must be single-quoted")
					break
				}

				closeQuote := strings.Index(input[1:], "'")
				if closeQuote == -1 {
					log.Printf("New: title must be single-quoted (missing closing quote)")
					break
				}

				title := input[1 : closeQuote+1]
				if title == "" {
					log.Printf("New: title cannot be empty")
					break
				}

				// Extract tags (everything after closing quote)
				remainder := strings.TrimSpace(input[closeQuote+2:])
				var tags []string
				if remainder != "" {
					// Validate tags
					tagPattern := regexp.MustCompile(`^([\p{Ll}\p{Nd}]+,)*[\p{Ll}\p{Nd}]+$`)
					if !tagPattern.MatchString(remainder) {
						log.Printf("New: tags must be comma-delimited lowercase unicode words (no spaces)")
						break
					}
					tags = strings.Split(remainder, ",")
				}

				// Generate filename and content
				fullPath, content := metadata.GenerateNote(denoteDir, title, tags, "md-yaml")

				// Create new Acme window
				var newWin *acme.Win
				if newWin = acme.Show(fullPath); newWin == nil {
					if newWin, err = acme.New(); err != nil {
						log.Printf("New: failed to create window: %v", err)
						break
					}
					if err = newWin.Name(fullPath); err != nil {
						newWin.Del(true)
						log.Printf("New: failed to set window name: %v", err)
						break
					}
				}

				// Write content to window
				if err := newWin.Addr("0"); err != nil {
					log.Printf("New: failed to seek: %v", err)
					newWin.Del(true)
					break
				}
				if _, err := newWin.Write("data", []byte(content)); err != nil {
					log.Printf("New: failed to write content: %v", err)
					newWin.Del(true)
					break
				}

				newWin.Ctl("dirty")
				newWin.Addr("$")
				newWin.Ctl("dot=addr")
				newWin.Ctl("show")

				// Start event listener for this note window
				go watchNoteWindow(newWin, fullPath)
			case "Remove":
				// Read chorded argument
				input := strings.TrimSpace(string(e.Arg))
				if input == "" {
					log.Printf("Remove: no input provided")
					break
				}

				// Write to denote/n/<identifier>/ctl via 9P
				if err := p9client.With9P(func(f *client.Fsys) error {
					return p9client.WriteFile(f, filepath.Join("n", input, "ctl"), "d")
				}); err != nil {
					log.Printf("failed to delete file: %v", err)
				}
			case "Reset":
				refreshWindowWithDefaults(w)
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
				var emptyResults metadata.Results
				updated, err := emptyResults.FromString(string(body))
				if err != nil {
					log.Printf("failed to parse window: %v", err)
					break
				}

				// Get current results and apply changes
				// Read unfiltered index
				var current metadata.Results
				err = p9client.With9P(func(f *client.Fsys) error {
					if err := setFilter(f, ""); err != nil {
						return err
					}
					var err error
					current, err = readIndex(f)
					return err
				})
				if err != nil {
					log.Printf("failed to get current results: %v", err)
					break
				}

				// Validate and apply changes via individual file writes
				if err := p9client.With9P(func(f *client.Fsys) error {
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
				if err := exec.Command("plumb", fmt.Sprintf("denote:%s", text)).Run(); err != nil {
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

func performSearch(w *acme.Win, searchText string) {
	args := parseArgs(searchText)

	var filterArgs []string
	sortBy := metadata.SortById
	sortOrder := metadata.SortOrderDesc

	for _, arg := range args {
		if sortSpec, ok := strings.CutPrefix(arg, "sort:"); ok {
			parts := strings.Split(sortSpec, ",")

			switch parts[0] {
			case "id", "date":
				sortBy = metadata.SortById
			case "title":
				sortBy = metadata.SortByTitle
			}

			if len(parts) > 1 {
				switch parts[1] {
				case "asc":
					sortOrder = metadata.SortOrderAsc
				case "desc":
					sortOrder = metadata.SortOrderDesc
				}
			} else {
				sortOrder = metadata.SortOrderAsc
			}
		} else {
			filterArgs = append(filterArgs, arg)
		}
	}

	// Build filter query string for ctl
	filterQuery := strings.Join(filterArgs, " ")

	// Set filter and read index
	var rs metadata.Results
	err := p9client.With9P(func(f *client.Fsys) error {
		if err := setFilter(f, filterQuery); err != nil {
			return err
		}
		var err error
		rs, err = readIndex(f)
		return err
	})
	if err != nil {
		log.Printf("search error: %v", err)
		return
	}
	metadata.Sort(rs, sortBy, sortOrder)

	refreshWindow(w, rs)
}

func refreshWindow(w *acme.Win, rs metadata.Results) {
	w.Addr(",")
	w.Write("data", rs.Bytes())
	w.Addr("#0")
	w.Ctl("dot=addr")
	w.Ctl("show")
}

func refreshWindowWithDefaults(w *acme.Win) {
	var rs metadata.Results
	err := p9client.With9P(func(f *client.Fsys) error {
		if err := setFilter(f, ""); err != nil {
			return err
		}
		var err error
		rs, err = readIndex(f)
		return err
	})
	if err != nil {
		log.Printf("error refreshing: %v", err)
		return
	}
	metadata.Sort(rs, metadata.SortById, metadata.SortOrderDesc)
	refreshWindow(w, rs)
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

func applyIndexChanges(f *client.Fsys, current, updated metadata.Results) error {
	// Validate entry count matches
	if len(updated) != len(current) {
		return fmt.Errorf("entry count mismatch: got %d, expected %d", len(updated), len(current))
	}

	// Build map of current state
	currentMap := make(map[string]*metadata.Metadata)
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
		tagsChanged := !metadata.SlicesEqual(orig.Tags, upd.Tags)

		if titleChanged || tagsChanged {
			// Write both title and keywords
			titlePath := "n/" + upd.Identifier + "/title"
			if err := p9client.WriteFile(f, titlePath, upd.Title); err != nil {
				return fmt.Errorf("failed to write title for %s: %w", upd.Identifier, err)
			}

			keywordsPath := "n/" + upd.Identifier + "/keywords"
			keywords := strings.Join(upd.Tags, ",")
			if err := p9client.WriteFile(f, keywordsPath, keywords); err != nil {
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

func watchNoteWindow(win *acme.Win, path string) {
	defer win.CloseFiles()

	for e := range win.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			// get metadata and emit 'n' event on CryptPut
			if string(e.Text) == "CryptPut" {
				win.WriteEvent(e)

				encryptedPath := path + ".gpg"
				meta := metadata.ParseFilename(encryptedPath)

				var newInput string
				if len(meta.Tags) > 0 {
					newInput = fmt.Sprintf("'%s' %s", meta.Title, strings.Join(meta.Tags, ","))
				} else {
					newInput = fmt.Sprintf("'%s'", meta.Title)
				}

				p9client.With9P(func(f *client.Fsys) error {
					return p9client.WriteFile(f, "new", newInput)
				})
			} else {
				win.WriteEvent(e)
			}
		}
	}
}
