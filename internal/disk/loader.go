package disk

import (
	"denote/pkg/encoding/frontmatter"
	"denote/pkg/metadata"
	"denote/pkg/util"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SupportsFrontMatter returns true if the file extension supports frontmatter.
func SupportsFrontMatter(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".org" || ext == ".md" || ext == ".txt"
}

// extractTitleFromContent extracts the title from file content.
// ext should be the file extension (e.g., ".md", ".org", ".txt").
// Returns empty string if no title found or unsupported extension.
func extractTitleFromContent(content string, ext string) string {
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

// ExtractMetadata extracts metadata from a file (combines filename and content parsing).
// This is the I/O wrapper around metadata's pure functions.
func ExtractMetadata(path string) (*metadata.Metadata, error) {
	// Parse filename (no I/O)
	note := metadata.ParseFilename(path)

	// Check if we should try to read file content for title
	ext := strings.ToLower(filepath.Ext(path))
	// Don't read unsupported file types
	if ext != ".org" && ext != ".md" && ext != ".txt" {
		return note, nil
	}

	// Read file content
	content, err := os.ReadFile(path)
	if err != nil {
		// If we can't read the file, just use filename metadata
		return note, nil
	}

	// Try to extract title from content
	if title := extractTitleFromContent(string(content), ext); title != "" {
		note.Title = title
	}

	return note, nil
}

// ExtractFrontMatter reads a file and parses its front matter.
// Returns the parsed FrontMatter and the detected FileType.
func ExtractFrontMatter(path string) (*frontmatter.FrontMatter, frontmatter.FileType, error) {
	ext := strings.ToLower(filepath.Ext(path))

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read file: %w", err)
	}

	return frontmatter.Unmarshal(string(content), ext)
}

// UpdateFrontMatter updates the front matter in a file.
func UpdateFrontMatter(path string, fm *frontmatter.FrontMatter, fileType frontmatter.FileType) error {
	// Only update frontmatter for supported file types
	if !SupportsFrontMatter(path) {
		return nil
	}

	// Read current content
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Apply front matter to content
	newContent, err := util.Apply(string(content), fm, fileType)
	if err != nil {
		return fmt.Errorf("failed to apply front matter: %w", err)
	}

	// Write back to file
	return os.WriteFile(path, []byte(newContent), 0644)
}

// LoadAll walks a directory and extracts metadata from all denote files.
func LoadAll(dir string) (metadata.Results, error) {
	var notes metadata.Results

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		note, err := ExtractMetadata(path)
		if err != nil {
			return err
		}

		// Only include files with valid identifiers
		if note.Identifier != "" {
			notes = append(notes, note)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return notes, nil
}
