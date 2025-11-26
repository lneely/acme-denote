package metadata

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Metadata is the metadata encoded into Denote-style
// file names.
type Metadata struct {
	Path       string
	Identifier string
	Signature  string
	Title      string
	Tags       []string
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
	// Allow lowercase Latin letters, other letters (CJK, etc.), and digits, no spaces
	tagPattern := regexp.MustCompile(`^([\p{Ll}\p{Lo}\p{Nd}]+,)*[\p{Ll}\p{Lo}\p{Nd}]+$`)

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

// ParseFilename extracts Denote metadata from a filename only (no file I/O).
// Returns metadata with Path, Identifier, Signature, Title (from filename), and Tags.
// Caller should use ExtractTitleFromContent to get title from file content.
func ParseFilename(path string) *Metadata {
	fname := filepath.Base(path)
	note := &Metadata{Path: path}

	if m := regexp.MustCompile(`^(\d{8}T\d{6})`).FindStringSubmatch(fname); m != nil {
		note.Identifier = m[1]
	}

	// Extract signature (optional component between identifier and title)
	if m := regexp.MustCompile(`==([^-]+?)--`).FindStringSubmatch(fname); m != nil {
		note.Signature = m[1]
	}

	// Extract title from filename
	if m := regexp.MustCompile(`--([^_\.]+)`).FindStringSubmatch(fname); m != nil {
		note.Title = strings.ReplaceAll(m[1], "-", " ")
	}

	if m := regexp.MustCompile(`__(.+?)(?:\.|$)`).FindStringSubmatch(fname); m != nil {
		note.Tags = strings.Split(m[1], "_")
	}

	return note
}

// ExtractTitleFromContent extracts the title from file content.
// ext should be the file extension (e.g., ".md", ".org", ".txt").
// Returns empty string if no title found or unsupported extension.
func ExtractTitleFromContent(content string, ext string) string {
	ext = strings.ToLower(ext)
	if ext != ".org" && ext != ".md" && ext != ".txt" {
		return ""
	}

	// Try org-mode #+title: first, then fall back to first heading
	if ext == ".org" {
		if m := regexp.MustCompile(`(?m)^#\+title:\s*(.+)$`).FindStringSubmatch(content); m != nil {
			return strings.TrimSpace(m[1])
		}
		// Fallback to first heading (lines starting with *)
		if m := regexp.MustCompile(`(?m)^\*+\s+(.+)$`).FindStringSubmatch(content); m != nil {
			return strings.TrimSpace(m[1])
		}
	}

	// Try markdown YAML front matter title: first, then fall back to # header
	if ext == ".md" {
		if m := regexp.MustCompile(`(?ms)^---\n.*?^title:\s*(.+?)$.*?^---`).FindStringSubmatch(content); m != nil {
			return strings.TrimSpace(strings.Trim(m[1], `"`))
		}
		if m := regexp.MustCompile(`(?m)^#\s+(.+)$`).FindStringSubmatch(content); m != nil {
			return strings.TrimSpace(m[1])
		}
	}

	return ""
}

// GenerateIdentifier creates a new identifier timestamp.
func GenerateIdentifier() string {
	return time.Now().Format("20060102T150405")
}

// slugifyTitle converts a title to a filesystem-safe slug.
func slugifyTitle(title string) string {
	slug := strings.ToLower(title)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "_", "-")
	return regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(slug, "")
}

// slugifySignature converts a signature to Denote-compliant format.
// Following Denote rules: lowercase, spaces/underscores -> double equals,
// remove special chars, normalize consecutive equals to double equals.
func slugifySignature(sig string) string {
	slug := strings.ToLower(sig)
	slug = strings.ReplaceAll(slug, " ", "==")
	slug = strings.ReplaceAll(slug, "_", "==")
	// Remove special characters per Denote spec
	slug = regexp.MustCompile(`[{}!@#$%^&*()+'"?,.\\|;:~\x60''""/-]`).ReplaceAllString(slug, "")
	// Normalize consecutive equals signs (3 or more) to double equals
	slug = regexp.MustCompile(`={3,}`).ReplaceAllString(slug, "==")
	// Trim trailing equals
	slug = strings.Trim(slug, "=")
	return slug
}

// formatKeywords formats keywords for a denote filename.
func formatKeywords(keywords []string) string {
	if len(keywords) == 0 {
		return ""
	}
	return "__" + strings.Join(keywords, "_")
}

// formatSignature formats a signature for a denote filename.
// Returns empty string if signature is empty, otherwise returns ==signature.
func formatSignature(sig string) string {
	if sig == "" {
		return ""
	}
	return "==" + slugifySignature(sig)
}

// BuildFilename constructs a denote filename from metadata components.
func BuildFilename(identifier, signature, title string, keywords []string, ext string) string {
	titleSlug := slugifyTitle(title)
	signaturePart := formatSignature(signature)
	keywordsPart := formatKeywords(keywords)
	return fmt.Sprintf("%s%s--%s%s%s", identifier, signaturePart, titleSlug, keywordsPart, ext)
}

// GenerateNote creates a new note with generated identifier and content.
// Returns (path, content).
func GenerateNote(dir, title, signature string, keywords []string, fileType string) (string, string) {
	identifier := GenerateIdentifier()
	ext := fileExtensions[fileType]
	filename := BuildFilename(identifier, signature, title, keywords, ext)
	path := filepath.Join(dir, filename)
	content := Generate(title, signature, keywords, fileType, identifier)
	return path, content
}
