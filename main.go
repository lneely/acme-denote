package main

import (
	p9client "denote/internal/p9/client"
	"denote/pkg/encoding/results"
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

const wname = "/Denote/"

// readIndex reads and parses the index from 9P server.
func readIndex(f *client.Fsys) (metadata.Results, error) {
	indexContent, err := p9client.ReadFile(f, "index")
	if err != nil {
		return nil, fmt.Errorf("failed to read index: %w", err)
	}
	return results.Unmarshal([]byte(indexContent))
}

// setFilter sets or clears the filter on the 9P server.
func setFilter(f *client.Fsys, filterQuery string) error {
	cmd := "filter"
	if filterQuery != "" {
		cmd = "filter " + filterQuery
	}
	return p9client.WriteFile(f, "ctl", cmd)
}

func main() {
	var err error
	var w *acme.Win
	args := os.Args[1:]
	if len(args) == 1 {
		if identifier, ok := strings.CutPrefix(args[0], "denote:"); ok {
			// Plumb the identifier directly (plumbing rules handle the mount)
			if err := exec.Command("plumb", "denote:"+identifier).Run(); err != nil {
				log.Fatalf("failed to plumb identifier: %v", err)
			}
			return
		}
		fmt.Println("Usage: Denote [denote:<identifier>]")
		return
	}

	// Connect to denotesrv, auto-starting if needed
	if err := p9client.With9P(func(f *client.Fsys) error {
		return nil
	}); err != nil {
		cmd := exec.Command("denotesrv", "start")
		if err := cmd.Run(); err != nil {
			log.Fatalf("failed to start denotesrv: %v", err)
		}
		for i := 0; i < 10; i++ {
			if err := p9client.With9P(func(f *client.Fsys) error {
				return nil
			}); err == nil {
				break
			}
			if i == 9 {
				log.Fatal("denotesrv failed to start")
			}
		}
	}

	// open window - look for existing /Denote/ window
	if wins, err := acme.Windows(); err == nil {
		for _, winInfo := range wins {
			if winInfo.Name == wname {
				if w, err = acme.Open(winInfo.ID, nil); err == nil {
					break
				}
			}
		}
	}
	if w == nil {
		if w, err = acme.New(); err != nil {
			log.Fatal(err)
		}
		if err = w.Name(wname); err != nil {
			w.Del(true)
			log.Fatal(err)
		}
	}
	defer w.CloseFiles()

	if _, err = w.Write("tag", []byte("New Put Remove Get")); err != nil {
		w.Del(true)
		log.Fatal(err)
	}

	// get initial results
	var rs metadata.Results
	err = p9client.With9P(func(f *client.Fsys) error {
		if err := setFilter(f, ""); err != nil {
			return err
		}
		rs, err = readIndex(f)
		return err
	})
	if err != nil {
		log.Fatal(err)
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
				input := strings.TrimSpace(string(e.Arg))
				if input == "" {
					break
				}
				if err := p9client.With9P(func(f *client.Fsys) error {
					return p9client.WriteFile(f, "new", input)
				}); err != nil {
					log.Printf("failed to create note: %v", err)
				}
				refreshWindowWithDefaults(w)
				w.Addr("#0")
				w.Ctl("dot=addr")
				w.Ctl("show")
			case "Remove":
				input := strings.TrimSpace(string(e.Arg))
				if input == "" {
					break
				}
				w.Ctl("addr=dot")
				q0, q1, _ := w.ReadAddr()
				if err := p9client.With9P(func(f *client.Fsys) error {
					return p9client.WriteFile(f, filepath.Join("n", input, "ctl"), "d")
				}); err != nil {
					log.Printf("failed to delete file: %v", err)
				}
				refreshWindowWithDefaults(w)
				w.Addr("#%d,#%d", q0, q1)
				w.Ctl("dot=addr")
				w.Ctl("show")
			case "Look":
				performSearch(w, string(e.Arg))
				w.Addr("#0")
				w.Ctl("dot=addr")
				w.Ctl("show")
			case "Get":
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
				updated, err := results.UnmarshalStrict(body)
				if err != nil {
					log.Printf("failed to parse window: %v", err)
					break
				}
				var current metadata.Results
				err = p9client.With9P(func(f *client.Fsys) error {
					if err := setFilter(f, ""); err != nil {
						return err
					}
					current, err = readIndex(f)
					return err
				})
				if err != nil {
					log.Printf("failed to get current results: %v", err)
					break
				}
				if err := p9client.With9P(func(f *client.Fsys) error {
					return applyIndexChanges(f, current, updated)
				}); err != nil {
					log.Printf("failed to apply changes: %v", err)
				}
			default:
				w.WriteEvent(e)
			}
		case 'l', 'L':
			text := string(e.Text)
			if isIdentifier(text) {
				if err := exec.Command("plumb", "denote:"+text).Run(); err != nil {
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
			if len(parts) > 1 && parts[1] == "asc" {
				sortOrder = metadata.SortOrderAsc
			}
		} else {
			filterArgs = append(filterArgs, arg)
		}
	}

	filterQuery := strings.Join(filterArgs, " ")
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
	w.Write("data", results.Marshal(rs))
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
			if inQuote && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
				escaped = true
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
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	for i := range args {
		args[i] = strings.TrimFunc(args[i], unicode.IsSpace)
	}
	return args
}

func applyIndexChanges(f *client.Fsys, current, updated metadata.Results) error {
	if len(updated) != len(current) {
		return fmt.Errorf("entry count mismatch: got %d, expected %d", len(updated), len(current))
	}
	currentMap := make(map[string]*metadata.Metadata)
	for _, m := range current {
		currentMap[m.Identifier] = m
	}
	for _, upd := range updated {
		orig, exists := currentMap[upd.Identifier]
		if !exists {
			return fmt.Errorf("identifier '%s' not found", upd.Identifier)
		}
		if orig.Title != upd.Title || !slices.Equal(orig.Tags, upd.Tags) {
			if err := p9client.WriteFile(f, "n/"+upd.Identifier+"/title", upd.Title); err != nil {
				return fmt.Errorf("failed to write title for %s: %w", upd.Identifier, err)
			}
			if err := p9client.WriteFile(f, "n/"+upd.Identifier+"/keywords", strings.Join(upd.Tags, ",")); err != nil {
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
