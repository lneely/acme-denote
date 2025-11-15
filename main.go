package main

import (
	"denote/internal/denote"
	"denote/internal/fs"
	"denote/internal/ui"
	"fmt"
	"log"
	"os"
	"strings"
	"unicode"
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
