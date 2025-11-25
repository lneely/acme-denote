package disk

import (
	"denote/internal/metadata"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SupportsFrontMatter returns true if the file extension supports frontmatter.
func SupportsFrontMatter(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".org" || ext == ".md" || ext == ".txt"
}

// ExtractMetadata extracts metadata from a file (combines filename and content parsing).
// This is the I/O wrapper around metadata's pure functions.
func ExtractMetadata(path string) (*metadata.Metadata, error) {
	// Parse filename (no I/O)
	note := metadata.ParseFilename(path)

	// Check if we should try to read file content for title
	ext := strings.ToLower(filepath.Ext(path))
	// Don't read encrypted files or unsupported types
	if ext == ".gpg" || (ext != ".org" && ext != ".md" && ext != ".txt") {
		return note, nil
	}

	// Read file content
	content, err := os.ReadFile(path)
	if err != nil {
		// If we can't read the file, just use filename metadata
		return note, nil
	}

	// Try to extract title from content
	if title := metadata.ExtractTitleFromContent(string(content), ext); title != "" {
		note.Title = title
	}

	return note, nil
}

// ExtractFrontMatter reads a file and parses its front matter.
func ExtractFrontMatter(path string) (*metadata.FrontMatter, error) {
	ext := strings.ToLower(filepath.Ext(path))

	// Don't try to parse front matter from encrypted files
	if ext == ".gpg" {
		return &metadata.FrontMatter{}, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return metadata.ParseFrontMatter(string(content), ext)
}

// UpdateFrontMatter updates the front matter in a file.
func UpdateFrontMatter(path string, fm *metadata.FrontMatter) error {
	// Only update frontmatter for supported file types
	if !SupportsFrontMatter(path) {
		return nil
	}

	// Read current content
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Update front matter (pure function)
	newContent, err := metadata.UpdateFrontMatter(string(content), fm)
	if err != nil {
		return fmt.Errorf("failed to update front matter: %w", err)
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
