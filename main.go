package main

import (
	"denote/internal/disk"
	p9client "denote/internal/p9/client"
	p9server "denote/internal/p9/server"
	"denote/pkg/encoding/frontmatter"
	"denote/pkg/metadata"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"9fans.net/go/acme"
	"9fans.net/go/plan9/client"
)

const (
	wname = "/Denote/"
	ftype = metadata.FileTypeMdYaml
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

// handleNewEvent is called when a new note is created.
// When a new note is created in 9P, open an acme window (file created on Put).
func handleNewEvent(f *client.Fsys, identifier, denoteDir string) error {
	fields, err := p9client.ReadFields(f, identifier, "title", "keywords", "signature")
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}
	title := fields["title"]
	signature := fields["signature"]
	var tags []string
	if keywords, ok := fields["keywords"]; ok && keywords != "" {
		tags = strings.Split(keywords, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
	}

	// Parse subdirectory from title if using '/:' separator
	targetDir := denoteDir
	if idx := strings.Index(title, "/:"); idx != -1 {
		subdir := title[:idx+1] // Include trailing slash
		title = title[idx+2:]   // Skip past '/:'
		targetDir = filepath.Join(denoteDir, subdir)

		targetDir = filepath.Clean(targetDir)
		absTarget, err := filepath.Abs(targetDir)
		if err != nil {
			return fmt.Errorf("failed to resolve target directory: %w", err)
		}
		absBase, err := filepath.Abs(denoteDir)
		if err != nil {
			return fmt.Errorf("failed to resolve base directory: %w", err)
		}
		if !strings.HasPrefix(absTarget, absBase) {
			return fmt.Errorf("path traversal attempt: directory outside base")
		}

		// Create subdirectory if it doesn't exist
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", targetDir, err)
		}

		// Update metadata with cleaned title
		if err := p9client.WriteFile(f, "n/"+identifier+"/title", title); err != nil {
			return fmt.Errorf("failed to update title in metadata: %w", err)
		}
	}

	newIdentifier := metadata.GenerateIdentifier()
	fm := metadata.NewFrontMatter(title, signature, tags, newIdentifier)
	ext := metadata.GetExtension(ftype)
	filename := metadata.BuildFilename(fm, ext)
	path := filepath.Join(targetDir, filename)
	content := string(frontmatter.Marshal(fm, ftype))

	if err := p9client.WriteFile(f, "n/"+identifier+"/path", path); err != nil {
		return fmt.Errorf("failed to update path in metadata: %w", err)
	}

	// Check if file already exists (manual creation case)
	if _, err := os.Stat(path); err == nil {
		return nil
	}

	// Open acme window with initial content (file will be created on Put)
	var win *acme.Win
	if win = acme.Show(path); win == nil {
		if win, err = acme.New(); err != nil {
			return fmt.Errorf("failed to create window: %w", err)
		}
		if err = win.Name(path); err != nil {
			win.Del(true)
			return fmt.Errorf("failed to set window name: %w", err)
		}
	}

	if err := win.Addr("0"); err != nil {
		win.Del(true)
		return fmt.Errorf("failed to seek: %w", err)
	}
	if _, err := win.Write("data", []byte(content)); err != nil {
		win.Del(true)
		return fmt.Errorf("failed to write content: %w", err)
	}

	win.Ctl("dirty")
	win.Addr("$")
	win.Ctl("dot=addr")
	win.Ctl("show")

	return nil
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
	notes, err := disk.LoadAll(denoteDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load notes: %v\n", err)
	}

	// Setup callbacks for note operations
	callbacks := p9server.Callbacks{
		OnNew: func(identifier string) error {
			return p9client.With9P(func(f *client.Fsys) error {
				return handleNewEvent(f, identifier, denoteDir)
			})
		},
		OnUpdate: func(identifier string) error {
			return p9client.With9P(func(f *client.Fsys) error {
				return disk.HandleUpdateEvent(f, identifier, denoteDir)
			})
		},
		OnRename: func(identifier string) error {
			return p9client.With9P(func(f *client.Fsys) error {
				return disk.HandleRenameEvent(f, identifier, denoteDir)
			})
		},
		OnDelete: func(identifier string) error {
			return p9client.With9P(func(f *client.Fsys) error {
				return disk.HandleDeleteEvent(f, identifier, denoteDir)
			})
		},
	}

	// 9p server startup with pre-loaded data and callbacks
	if err := p9server.StartServer(notes, denoteDir, callbacks); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to start fileserver: %v\n", err)
	}

	// start acme log watcher
	go disk.WatchAcmeLog()

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

	if _, err = w.Write("tag", []byte("New Put Remove Get")); err != nil {
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
					break
				}

				// Write to 9p (triggers callback which opens window)
				if err := p9client.With9P(func(f *client.Fsys) error {
					return p9client.WriteFile(f, "new", input)
				}); err != nil {
					log.Printf("failed to create note: %v", err)
				}

				// Refresh window and scroll to top to show new note
				refreshWindowWithDefaults(w)
				w.Addr("#0")
				w.Ctl("dot=addr")
				w.Ctl("show")
			case "Remove":
				// Read chorded argument
				input := strings.TrimSpace(string(e.Arg))
				if input == "" {
					break
				}

				// Preserve current position for restoring after refresh
				w.Ctl("addr=dot")
				q0, q1, _ := w.ReadAddr()

				// Write to denote/n/<identifier>/ctl via 9P
				if err := p9client.With9P(func(f *client.Fsys) error {
					return p9client.WriteFile(f, filepath.Join("n", input, "ctl"), "d")
				}); err != nil {
					log.Printf("failed to delete file: %v", err)
				}

				// Refresh window and restore position
				refreshWindowWithDefaults(w)
				w.Addr("#%d,#%d", q0, q1)
				w.Ctl("dot=addr")
				w.Ctl("show")
			case "Look":
				// Use e.Arg for the search arguments
				performSearch(w, string(e.Arg))
				w.Addr("#0")
				w.Ctl("dot=addr")
				w.Ctl("show")
			case "Get":
				if err := disk.GetAll(); err != nil {
					log.Printf("get error: %v", err)
				}
				refreshWindowWithDefaults(w)
				w.Addr("#0")
				w.Ctl("dot=addr")
				w.Ctl("show")
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
		tagsChanged := !slices.Equal(orig.Tags, upd.Tags)

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
