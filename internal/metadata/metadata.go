package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Metadata is the metadata encoded into Denote-style
// file names.
type Metadata struct {
	Path       string
	Identifier string
	Title      string
	Tags       []string
	Extension  string
}

type Results []*Metadata

func (es Results) Bytes() []byte {
	var buf strings.Builder
	for _, e := range es {
		title := e.Title
		if title == "" {
			title = "(untitled)"
		}

		tags := strings.Join(e.Tags, ",")
		fmt.Fprintf(&buf, "%s | %s | %s\n", e.Identifier, title, tags)
	}
	return []byte(buf.String())
}

func (es Results) FromString(data string) (Results, error) {
	var results Results
	lines := strings.Split(strings.TrimSpace(data), "\n")
	// Allow lowercase unicode letters and digits, no spaces
	tagPattern := regexp.MustCompile(`^([\p{Ll}\p{Nd}]+,)*[\p{Ll}\p{Nd}]+$`)

	for lineNum, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			return nil, fmt.Errorf("line %d: expected 3 columns, got %d (line: %q)", lineNum+1, len(parts), line)
		}

		identifier := strings.TrimSpace(parts[0])
		title := strings.TrimSpace(parts[1])
		tagsStr := strings.TrimSpace(parts[2])

		if identifier == "" {
			return nil, fmt.Errorf("line %d: identifier cannot be empty", lineNum+1)
		}

		if strings.Contains(title, "|") {
			return nil, fmt.Errorf("line %d: title cannot contain '|'", lineNum+1)
		}

		var tags []string
		if tagsStr != "" {
			if !tagPattern.MatchString(tagsStr) {
				return nil, fmt.Errorf("line %d: tags must be comma-delimited lowercase unicode words (no spaces): got '%s'", lineNum+1, tagsStr)
			}
			tags = strings.Split(tagsStr, ",")
		} else {
			tags = []string{}
		}

		results = append(results, &Metadata{
			Identifier: identifier,
			Title:      title,
			Tags:       tags,
		})
	}

	return results, nil
}

func SlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type SortBy string

const (
	SortById    SortBy = "id"
	SortByDate  SortBy = "date"
	SortByTitle SortBy = "title"
)

type SortOrder int

const (
	SortOrderAsc SortOrder = iota
	SortOrderDesc
)

// Sort organizes a list of notes by sortType and order using metadata.
func Sort(md Results, sortType SortBy, order SortOrder) {
	switch sortType {
	case SortById, SortByDate:
		sort.Slice(md, func(i, j int) bool {
			if order == SortOrderAsc {
				return md[i].Identifier < md[j].Identifier // Reverse chronological by default
			} else {
				return md[i].Identifier > md[j].Identifier // Reverse chronological by default
			}
		})
	case SortByTitle:
		sort.Slice(md, func(i, j int) bool {
			if order == SortOrderAsc {
				return strings.ToLower(md[i].Title) < strings.ToLower(md[j].Title)
			} else {
				return strings.ToLower(md[i].Title) > strings.ToLower(md[j].Title)
			}
		})
	default:
		sort.Slice(md, func(i, j int) bool {
			return md[i].Identifier > md[j].Identifier // Reverse chronological by default
		})
	}
}

// ExtractMetadata extracts Denote metadata from a filename
func ExtractMetadata(path string) *Metadata {
	fname := filepath.Base(path)
	note := &Metadata{Path: path}

	if m := regexp.MustCompile(`^(\d{8}T\d{6})`).FindStringSubmatch(fname); m != nil {
		note.Identifier = m[1]
	}

	// Extract title from filename
	filenameTitle := ""
	if m := regexp.MustCompile(`--([^_\.]+)`).FindStringSubmatch(fname); m != nil {
		filenameTitle = strings.ReplaceAll(m[1], "-", " ")
	}

	// Try to get title from file content, fall back to filename
	fileContentTitle := extractTitle(path)
	if fileContentTitle != "" {
		note.Title = fileContentTitle
	} else {
		note.Title = filenameTitle
	}

	if m := regexp.MustCompile(`__(.+?)(?:\.|$)`).FindStringSubmatch(fname); m != nil {
		note.Tags = strings.Split(m[1], "_")
	}
	note.Extension = filepath.Ext(fname)

	return note
}

func extractTitle(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".org" && ext != ".md" && ext != ".txt" {
		return ""
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := string(data)

	// Try org-mode #+title: first, then fall back to first heading
	if ext == ".org" {
		if m := regexp.MustCompile(`(?m)^#\+title:\s*(.+)$`).FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
		// Fallback to first heading (lines starting with *)
		if m := regexp.MustCompile(`(?m)^\*+\s+(.+)$`).FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
	}

	// Try markdown YAML front matter title: first, then fall back to # header
	if ext == ".md" {
		if m := regexp.MustCompile(`(?ms)^---\n.*?^title:\s*(.+?)$.*?^---`).FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(strings.Trim(m[1], `"`))
		}
		if m := regexp.MustCompile(`(?m)^#\s+(.+)$`).FindStringSubmatch(text); m != nil {
			return strings.TrimSpace(m[1])
		}
	}

	return ""
}
