package main

import (
	"denote/internal/fs"
	"denote/internal/sync"
	"denote/internal/tmpl"
	"denote/internal/ui"
	"fmt"
	"io"
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

// Helper functions for denote filename generation

func generateIdentifier() string {
	return time.Now().Format("20060102T150405")
}

func slugifyTitle(title string) string {
	slug := strings.ReplaceAll(strings.ToLower(title), " ", "-")
	return regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(slug, "")
}

func formatKeywords(keywords []string) string {
	if len(keywords) == 0 {
		return ""
	}
	return "__" + strings.Join(keywords, "_")
}

func buildDenoteFilename(identifier, title string, keywords []string, ext string) string {
	titleSlug := slugifyTitle(title)
	keywordsPart := formatKeywords(keywords)
	return fmt.Sprintf("%s--%s%s%s", identifier, titleSlug, keywordsPart, ext)
}

func generateNotePath(dir, title string, keywords []string, fileType string) (string, string) {
	identifier := generateIdentifier()
	ext := tmpl.FileExtensions[fileType]
	filename := buildDenoteFilename(identifier, title, keywords, ext)
	path := filepath.Join(dir, filename)
	content := tmpl.Generate(title, keywords, fileType, identifier)
	return path, content
}

func renameNote(oldPath, title string, keywords []string, identifier string) (string, error) {
	if identifier == "" {
		identifier = generateIdentifier()
	}

	ext := filepath.Ext(oldPath)
	dir := filepath.Dir(oldPath)
	filename := buildDenoteFilename(identifier, title, keywords, ext)
	newPath := filepath.Join(dir, filename)

	if err := os.Rename(oldPath, newPath); err != nil {
		return "", err
	}

	return newPath, nil
}

// 9P client helpers

func read9PFile(f *client.Fsys, path string) (string, error) {
	fid, err := f.Open(path, plan9.OREAD)
	if err != nil {
		return "", err
	}
	defer fid.Close()

	var content []byte
	buf := make([]byte, 8192)
	for {
		n, err := fid.Read(buf)
		if err != nil && err != io.EOF {
			return "", err
		}
		if n == 0 {
			break
		}
		content = append(content, buf[:n]...)
	}
	return strings.TrimSpace(string(content)), nil
}

func read9PFields(f *client.Fsys, identifier string, fields ...string) (map[string]string, error) {
	result := make(map[string]string)
	for _, field := range fields {
		val, err := read9PFile(f, "n/"+identifier+"/"+field)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", field, err)
		}
		result[field] = val
	}
	return result, nil
}

func readIndex() (fs.Results, error) {
	var results fs.Results

	err := fs.With9P(func(f *client.Fsys) error {
		indexContent, err := read9PFile(f, "index")
		if err != nil {
			return fmt.Errorf("failed to read index: %w", err)
		}

		results, err = results.FromString(indexContent)
		if err != nil {
			return err
		}

		// Populate Path and Extension for each note
		for _, note := range results {
			fields, err := read9PFields(f, note.Identifier, "path", "extension")
			if err != nil {
				return fmt.Errorf("failed to read fields for %s: %w", note.Identifier, err)
			}
			note.Path = fields["path"]
			note.Extension = fields["extension"]
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

func setFilter(filterQuery string) error {
	return fs.With9P(func(f *client.Fsys) error {
		var cmd string
		if filterQuery == "" {
			cmd = "filter"
		} else {
			cmd = "filter " + filterQuery
		}
		return fs.WriteFile(f, "ctl", cmd)
	})
}

func identifierToPath(identifier string) (string, error) {
	if err := setFilter(fmt.Sprintf("date:%s", identifier)); err != nil {
		return "", fmt.Errorf("failed to set filter: %w", err)
	}
	defer setFilter("")

	notes, err := readIndex()
	if err != nil {
		return "", fmt.Errorf("failed to read index: %w", err)
	}

	if len(notes) == 0 {
		return "", fmt.Errorf("no note found with identifier %s", identifier)
	}

	return notes[0].Path, nil
}

// Event handling

func eventListener() {
	for {
		err := fs.With9P(func(f *client.Fsys) error {
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
					if err := handleUpdateEvent(f, identifier); err != nil {
						log.Printf("error handling update for %s: %v", identifier, err)
					}
				case "r":
					if err := handleRenameEvent(f, identifier); err != nil {
						log.Printf("error handling rename for %s: %v", identifier, err)
					}
				case "n":
					if err := handleNewEvent(identifier); err != nil {
						log.Printf("error handling new for %s: %v", identifier, err)
					}
				case "d":
					if err := handleDeleteEvent(identifier); err != nil {
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

func handleUpdateEvent(f *client.Fsys, identifier string) error {
	fields, err := read9PFields(f, identifier, "title", "keywords", "path", "extension")
	if err != nil {
		return err
	}

	title := fields["title"]
	keywords := fields["keywords"]
	path := fields["path"]
	ext := fields["extension"]

	var tags []string
	if keywords != "" {
		for _, tag := range strings.Split(keywords, ",") {
			tags = append(tags, strings.TrimSpace(tag))
		}
	}

	isDenoteFile := ext == ".org" || ext == ".md" || ext == ".txt"

	if isDenoteFile {
		var fileType string
		switch ext {
		case ".org":
			fileType = "org"
		case ".md":
			fileType = "md-yaml"
		case ".txt":
			fileType = "txt"
		}

		fm := &tmpl.FrontMatter{
			Title:      title,
			Tags:       tags,
			Identifier: identifier,
			FileType:   fileType,
		}

		if err := tmpl.Update(path, fm); err != nil {
			log.Printf("failed to update front matter for %s: %v", identifier, err)
		}
	}

	return nil
}

func handleRenameEvent(f *client.Fsys, identifier string) error {
	fields, err := read9PFields(f, identifier, "title", "keywords", "path")
	if err != nil {
		return err
	}

	title := fields["title"]
	keywords := fields["keywords"]
	path := fields["path"]

	var tags []string
	if keywords != "" {
		for _, tag := range strings.Split(keywords, ",") {
			tags = append(tags, strings.TrimSpace(tag))
		}
	}

	newPath, err := renameNote(path, title, tags, identifier)
	if err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	if newPath != path {
		if err := fs.UpdateNotePath(identifier, newPath); err != nil {
			return fmt.Errorf("failed to update path: %w", err)
		}
	}

	return nil
}

func handleNewEvent(identifier string) error {
	metadata, err := fs.GetNote(identifier)
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	path, content := generateNotePath(denoteDir, metadata.Title, metadata.Tags, "md-yaml")

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create note file: %w", err)
	}

	if err := fs.UpdateNotePath(identifier, path); err != nil {
		return fmt.Errorf("failed to update path in metadata: %w", err)
	}

	log.Printf("created new note: %s at %s", identifier, path)
	return nil
}

func handleDeleteEvent(identifier string) error {
	pattern := filepath.Join(denoteDir, identifier+"--*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find file: %w", err)
	}

	if len(matches) == 0 {
		return fmt.Errorf("no file found matching identifier: %s", identifier)
	}

	if len(matches) > 1 {
		log.Printf("warning: multiple files match identifier %s, deleting first: %s", identifier, matches[0])
	}

	if err := os.Remove(matches[0]); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	log.Printf("deleted note: %s", matches[0])
	return nil
}

func main() {
	var err error
	var w *ui.Window
	args := os.Args[1:]
	if len(args) == 1 {
		if identifier, ok := strings.CutPrefix(args[0], "denote:"); ok {
			path, err := identifierToPath(identifier)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error finding note: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(path)
			if err := exec.Command("plumb", path).Run(); err != nil {
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

	// 9p server startup
	if err := fs.StartServer(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to start fileserver: %v\n", err)
	}

	// start event consumer
	go eventListener()

	// start acme log watcher
	go sync.WatchAcmeLog()

	// open window
	if w, err = ui.WindowOpen(wname); err != nil {
		log.Fatal(err)
	}
	defer w.CloseFiles()

	if err = ui.TagSet(w, "New Put Remove Reset Sync"); err != nil {
		log.Fatal(err)
	}

	// get initial results (clear any filter, read index)
	if err := setFilter(""); err != nil {
		panic(err)
	}
	rs, err := readIndex()
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
				fullPath, content := generateNotePath(denoteDir, title, tags, "md-yaml")

				// Create new Acme window
				newWin, err := ui.WindowOpen(fullPath)
				if err != nil {
					log.Printf("New: failed to create window: %v", err)
					break
				}

				// Write content to window
				if err := ui.BodyWrite(newWin, "0", []byte(content)); err != nil {
					log.Printf("New: failed to write content: %v", err)
					newWin.Del(true)
					break
				}

				ui.WindowDirty(newWin, true)
				ui.DotToAddr(newWin, "$")

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
				if err := fs.With9P(func(f *client.Fsys) error {
					return fs.WriteFile(f, filepath.Join("n", input, "ctl"), "d")
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
				var emptyResults fs.Results
				updated, err := emptyResults.FromString(string(body))
				if err != nil {
					log.Printf("failed to parse window: %v", err)
					break
				}

				// Get current results and apply changes
				// Read unfiltered index
				if err := setFilter(""); err != nil {
					log.Printf("failed to clear filter: %v", err)
					break
				}
				current, err := readIndex()
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

	// Build filter query string for ctl
	filterQuery := strings.Join(filterArgs, " ")

	// Set filter on server
	if err := setFilter(filterQuery); err != nil {
		log.Printf("failed to set filter: %v", err)
		return
	}

	// Read filtered index
	rs, err := readIndex()
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

func refreshWindowWithDefaults(w *ui.Window) {
	// Clear filter and read index
	if err := setFilter(""); err != nil {
		log.Printf("error clearing filter: %v", err)
		return
	}
	rs, err := readIndex()
	if err != nil {
		log.Printf("error refreshing: %v", err)
		return
	}
	fs.Sort(rs, fs.SortById, fs.SortOrderDesc)
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
			titlePath := "n/" + upd.Identifier + "/title"
			if err := fs.WriteFile(f, titlePath, upd.Title); err != nil {
				return fmt.Errorf("failed to write title for %s: %w", upd.Identifier, err)
			}

			keywordsPath := "n/" + upd.Identifier + "/keywords"
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

func watchNoteWindow(win *acme.Win, path string) {
	defer win.CloseFiles()

	for e := range win.EventChan() {
		switch e.C2 {
		case 'x', 'X':
			// get metadata and emit 'n' event on CryptPut
			if string(e.Text) == "CryptPut" {
				win.WriteEvent(e)

				encryptedPath := path + ".gpg"
				meta := fs.ExtractMetadata(encryptedPath)

				var newInput string
				if len(meta.Tags) > 0 {
					newInput = fmt.Sprintf("'%s' %s", meta.Title, strings.Join(meta.Tags, ","))
				} else {
					newInput = fmt.Sprintf("'%s'", meta.Title)
				}

				fs.With9P(func(f *client.Fsys) error {
					return fs.WriteFile(f, "new", newInput)
				})
			} else {
				win.WriteEvent(e)
			}
		}
	}
}
