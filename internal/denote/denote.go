package denote

import (
	"denote/internal/fs"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

// GetDir returns the denote directory path
func GetDir() string {
	return fmt.Sprintf("%s/doc", os.Getenv("HOME"))
}

// readFile reads entire content from a 9P file
func readFile(f *client.Fsys, path string) (string, error) {
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

// read9PFields reads multiple 9P fields for a note
func read9PFields(f *client.Fsys, identifier string, fields ...string) (map[string]string, error) {
	result := make(map[string]string)
	for _, field := range fields {
		val, err := readFile(f, identifier+"/"+field)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", field, err)
		}
		result[field] = val
	}
	return result, nil
}

// Search returns metadata for notes by reading from the 9P index
func Search(filters []*fs.Filter) (fs.Results, error) {
	var results fs.Results

	err := fs.With9P(func(f *client.Fsys) error {
		// Read index to get identifiers
		indexContent, err := readFile(f, "index")
		if err != nil {
			return fmt.Errorf("failed to read index: %w", err)
		}

		lines := strings.Split(indexContent, "\n")
		for _, line := range lines {
			if line == "" {
				continue
			}

			parts := strings.Split(line, "|")
			if len(parts) < 1 {
				continue
			}

			identifier := strings.TrimSpace(parts[0])
			meta := &fs.Metadata{
				Identifier: identifier,
			}

			// Read remaining fields from note directory
			if path, err := readFile(f, identifier+"/path"); err == nil {
				meta.Path = path
			}
			if ext, err := readFile(f, identifier+"/extension"); err == nil {
				meta.Extension = ext
			}
			if keywords, err := readFile(f, identifier+"/keywords"); err == nil && keywords != "" {
				meta.Tags = strings.Split(keywords, ",")
				for i := range meta.Tags {
					meta.Tags[i] = strings.TrimSpace(meta.Tags[i])
				}
			}

			// Read title - 9P server already returns file content title if available,
			// otherwise filename-based title
			if title, err := readFile(f, identifier+"/title"); err == nil {
				meta.Title = title
			}

			// Apply filters
			match := true
			for _, filt := range filters {
				if !filt.IsMatch(meta) {
					match = false
					break
				}
			}

			if match {
				results = append(results, meta)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return results, nil
}

// Templates for denote frontmatter (org, md, txt)
var templates = map[string]string{
	"org": `#+title:      %s
#+date:       %s
#+filetags:   %s
#+identifier: %s

`,
	"md-yaml": `---
title:      %s
date:       %s
tags:       %s
identifier: %s
---

`,
	"md-toml": `+++
title      = %s
date       = %s
tags       = %s
identifier = %s
+++

`,
	"txt": `title:      %s
date:       %s
tags:       %s
identifier: %s
---------------------------

`,
}

//	contains the list of file extension
//
// for which Denote should add front matter.
var fileExtensions = map[string]string{
	"org":     ".org",
	"md-yaml": ".md",
	"md-toml": ".md",
	"txt":     ".txt",
}

func formatTags(tags []string, fileType string) string {
	if len(tags) == 0 {
		return ""
	}
	switch fileType {
	case "org":
		return ":" + strings.Join(tags, ":") + ":"
	case "md-yaml", "md-toml":
		return "[" + strings.Join(tags, ", ") + "]"
	default:
		return strings.Join(tags, " ")
	}
}

// FrontMatter represents parsed front matter from a note
type FrontMatter struct {
	Title      string
	Tags       []string
	Identifier string
	FileType   string // org, md-yaml, md-toml, txt
}

// ExtractFrontMatter extracts front matter from a file
func ExtractFrontMatter(path string) (*FrontMatter, error) {
	ext := strings.ToLower(filepath.Ext(path))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)

	fm := &FrontMatter{}

	switch ext {
	case ".org":
		fm.FileType = "org"
		if m := regexp.MustCompile(`(?m)^#\+title:\s*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Title = strings.TrimSpace(m[1])
		}
		if m := regexp.MustCompile(`(?m)^#\+filetags:\s*:(.+):$`).FindStringSubmatch(text); m != nil {
			fm.Tags = strings.Split(m[1], ":")
		}
		if m := regexp.MustCompile(`(?m)^#\+identifier:\s*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Identifier = strings.TrimSpace(m[1])
		}

	case ".md":
		// Try YAML first
		yamlRe := regexp.MustCompile(`(?ms)^---\n(.*?)\n---`)
		if m := yamlRe.FindStringSubmatch(text); m != nil {
			fm.FileType = "md-yaml"
			yamlContent := m[1]
			if m := regexp.MustCompile(`(?m)^title:\s*["']?(.+?)["']?$`).FindStringSubmatch(yamlContent); m != nil {
				fm.Title = strings.TrimSpace(m[1])
			}
			if m := regexp.MustCompile(`(?m)^tags:\s*\[(.+?)\]$`).FindStringSubmatch(yamlContent); m != nil {
				tags := strings.Split(m[1], ",")
				for i, t := range tags {
					tags[i] = strings.TrimSpace(t)
				}
				fm.Tags = tags
			}
			if m := regexp.MustCompile(`(?m)^identifier:\s*["']?(.+?)["']?$`).FindStringSubmatch(yamlContent); m != nil {
				fm.Identifier = strings.TrimSpace(m[1])
			}
		} else {
			// Try TOML
			tomlRe := regexp.MustCompile(`(?ms)^\+\+\+\n(.*?)\n\+\+\+`)
			if m := tomlRe.FindStringSubmatch(text); m != nil {
				fm.FileType = "md-toml"
				tomlContent := m[1]
				if m := regexp.MustCompile(`(?m)^title\s*=\s*["']?(.+?)["']?$`).FindStringSubmatch(tomlContent); m != nil {
					fm.Title = strings.TrimSpace(m[1])
				}
				if m := regexp.MustCompile(`(?m)^tags\s*=\s*\[(.+?)\]$`).FindStringSubmatch(tomlContent); m != nil {
					tags := strings.Split(m[1], ",")
					for i, t := range tags {
						tags[i] = strings.TrimSpace(t)
					}
					fm.Tags = tags
				}
				if m := regexp.MustCompile(`(?m)^identifier\s*=\s*["']?(.+?)["']?$`).FindStringSubmatch(tomlContent); m != nil {
					fm.Identifier = strings.TrimSpace(m[1])
				}
			}
		}

	case ".txt":
		fm.FileType = "txt"
		if m := regexp.MustCompile(`(?m)^title:\s*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Title = strings.TrimSpace(m[1])
		}
		if m := regexp.MustCompile(`(?m)^tags:\s*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Tags = strings.Fields(m[1])
		}
		if m := regexp.MustCompile(`(?m)^identifier:\s*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Identifier = strings.TrimSpace(m[1])
		}
	}

	return fm, nil
}

// updateFrontMatter updates front matter in a file
func updateFrontMatter(path string, fm *FrontMatter) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(data)

	dateStr := time.Now().Format("2006-01-02 Mon 15:04")
	keywordsStr := formatTags(fm.Tags, fm.FileType)

	// For org-mode, wrap date in brackets for timestamp
	if fm.FileType == "org" {
		dateStr = "[" + dateStr + "]"
	}

	template := templates[fm.FileType]
	newFrontMatter := fmt.Sprintf(template, fm.Title, dateStr, keywordsStr, fm.Identifier)

	var newText string
	switch fm.FileType {
	case "org":
		// Find end of front matter (first blank line or non-#+ line)
		lines := strings.Split(text, "\n")
		endIdx := 0
		for i, line := range lines {
			if i > 0 && (line == "" || !strings.HasPrefix(line, "#+")) {
				endIdx = i
				break
			}
		}
		if endIdx > 0 {
			newText = newFrontMatter + strings.Join(lines[endIdx:], "\n")
		} else {
			newText = newFrontMatter + text
		}

	case "md-yaml":
		// Replace YAML front matter
		re := regexp.MustCompile(`(?s)^---\n.*?\n---\n*`)
		if re.MatchString(text) {
			newText = re.ReplaceAllString(text, newFrontMatter)
		} else {
			newText = newFrontMatter + text
		}

	case "md-toml":
		// Replace TOML front matter
		re := regexp.MustCompile(`(?s)^\+\+\+\n.*?\n\+\+\+\n*`)
		if re.MatchString(text) {
			newText = re.ReplaceAllString(text, newFrontMatter)
		} else {
			newText = newFrontMatter + text
		}

	case "txt":
		// Replace text front matter
		re := regexp.MustCompile(`(?s)^title:.*?\n-+\n*`)
		if re.MatchString(text) {
			newText = re.ReplaceAllString(text, newFrontMatter)
		} else {
			newText = newFrontMatter + text
		}
	default:
		return fmt.Errorf("unsupported file type: %s", fm.FileType)
	}

	return os.WriteFile(path, []byte(newText), 0644)
}

// rename renames a note file according to denote convention
func rename(oldPath, title string, keywords []string, identifier string) (string, error) {
	if identifier == "" {
		identifier = time.Now().Format("20060102T150405")
	}

	titleSlug := strings.ReplaceAll(strings.ToLower(title), " ", "-")
	titleSlug = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(titleSlug, "")

	var keywordsPart string
	if len(keywords) > 0 {
		keywordsPart = "__" + strings.Join(keywords, "_")
	}

	ext := filepath.Ext(oldPath)
	dir := filepath.Dir(oldPath)
	filename := fmt.Sprintf("%s--%s%s%s", identifier, titleSlug, keywordsPart, ext)
	newPath := filepath.Join(dir, filename)

	if err := os.Rename(oldPath, newPath); err != nil {
		return "", err
	}

	return newPath, nil
}

func NewNote(dir, title string, keywords []string, fileType, identifier string) (string, error) {
	// Use provided identifier or generate new one
	if identifier == "" {
		identifier = time.Now().Format("20060102T150405")
	}

	// Format file name
	titleSlug := strings.ReplaceAll(strings.ToLower(title), " ", "-")
	titleSlug = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(titleSlug, "")

	var keywordsPart string
	if len(keywords) > 0 {
		keywordsPart = "__" + strings.Join(keywords, "_")
	}

	ext := fileExtensions[fileType]
	filename := fmt.Sprintf("%s--%s%s%s", identifier, titleSlug, keywordsPart, ext)
	path := filepath.Join(dir, filename)

	// Generate front matter
	template := templates[fileType]
	dateStr := time.Now().Format("2006-01-02 Mon 15:04")
	keywordsStr := formatTags(keywords, fileType)

	content := fmt.Sprintf(template, title, dateStr, keywordsStr, identifier)

	// Write file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}

	return path, nil
}

// NewNoteEncrypted creates a new encrypted denote note using CryptPut
func NewNoteEncrypted(dir, title string, keywords []string, fileType, identifier string) (string, error) {
	if _, err := exec.LookPath("CryptPut"); err != nil {
		fmt.Fprintf(os.Stderr, "CryptPut is not installed.\ngit clone https://github.com/lneely/acme-crypt\n")
		return "", fmt.Errorf("CryptPut not available")
	}

	// Use provided identifier or generate new one
	if identifier == "" {
		identifier = time.Now().Format("20060102T150405")
	}

	// Format file name (without .gpg extension - CryptPut will add it)
	titleSlug := strings.ReplaceAll(strings.ToLower(title), " ", "-")
	titleSlug = regexp.MustCompile(`[^a-z0-9-]`).ReplaceAllString(titleSlug, "")

	var keywordsPart string
	if len(keywords) > 0 {
		keywordsPart = "__" + strings.Join(keywords, "_")
	}

	ext := fileExtensions[fileType]
	filename := fmt.Sprintf("%s--%s%s%s", identifier, titleSlug, keywordsPart, ext)
	plainPath := filepath.Join(dir, filename)

	template := templates[fileType]
	dateStr := time.Now().Format("2006-01-02 Mon 15:04")
	keywordsStr := formatTags(keywords, fileType)
	content := fmt.Sprintf(template, title, dateStr, keywordsStr, identifier)

	cmd := exec.Command("CryptPut", plainPath)
	cmd.Stdin = strings.NewReader(content)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to create encrypted file: %v", err)
	}
	encryptedPath := plainPath + ".gpg"

	return encryptedPath, nil
}

// Open opens a file using the appropriate method based on file extension
func Open(filePath string) error {
	if strings.HasSuffix(strings.ToLower(filePath), ".gpg") {
		if _, err := exec.LookPath("CryptGet"); err != nil {
			fmt.Fprintf(os.Stderr, "CryptGet is not installed.\ngit clone https://github.com/lneely/acme-crypt\n")
			return fmt.Errorf("CryptGet not available")
		}
		cmd := exec.Command("CryptGet", filePath)
		return cmd.Run()
	} else {
		cmd := exec.Command("plumb", filePath)
		return cmd.Run()
	}
}

// IdentifierToPath converts a denote identifier to a file path
func IdentifierToPath(identifier string) (string, error) {
	filter, err := fs.NewFilter(fmt.Sprintf("date:%s", identifier))
	if err != nil {
		return "", fmt.Errorf("invalid identifier: %w", err)
	}

	notes, err := Search([]*fs.Filter{filter})
	if err != nil {
		return "", fmt.Errorf("failed to search for note: %w", err)
	}

	if len(notes) == 0 {
		return "", fmt.Errorf("no note found with identifier %s", identifier)
	}

	return notes[0].Path, nil
}

func EventListener() {
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

				// Parse event: "identifier action"
				parts := strings.Fields(event)
				if len(parts) != 2 {
					log.Printf("invalid event format: %s", event)
					continue
				}

				identifier := parts[0]
				action := parts[1]

				// Handle events
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
	// Read current metadata via 9P
	fields, err := read9PFields(f, identifier, "title", "keywords", "path", "extension")
	if err != nil {
		return err
	}

	title := fields["title"]
	keywords := fields["keywords"]
	path := fields["path"]
	ext := fields["extension"]

	// Parse tags from keywords
	var tags []string
	if keywords != "" {
		for _, tag := range strings.Split(keywords, ",") {
			tags = append(tags, strings.TrimSpace(tag))
		}
	}

	// Check if this is a denote file (has front matter)
	isDenoteFile := ext == ".org" || ext == ".md" || ext == ".txt"

	if isDenoteFile {
		// Determine file type from extension
		var fileType string
		switch ext {
		case ".org":
			fileType = "org"
		case ".md":
			// Need to detect YAML vs TOML - default to YAML
			fileType = "md-yaml"
		case ".txt":
			fileType = "txt"
		}

		// Update front matter
		fm := &FrontMatter{
			Title:      title,
			Tags:       tags,
			Identifier: identifier,
			FileType:   fileType,
		}

		if err := updateFrontMatter(path, fm); err != nil {
			log.Printf("failed to update front matter for %s: %v", identifier, err)
		}
	}
	// For non-denote files (PDFs, etc.), metadata is already updated by 9P write

	return nil
}

func handleRenameEvent(f *client.Fsys, identifier string) error {
	// Read current metadata via 9P
	fields, err := read9PFields(f, identifier, "title", "keywords", "path")
	if err != nil {
		return err
	}

	title := fields["title"]
	keywords := fields["keywords"]
	path := fields["path"]

	// Parse tags from keywords
	var tags []string
	if keywords != "" {
		for _, tag := range strings.Split(keywords, ",") {
			tags = append(tags, strings.TrimSpace(tag))
		}
	}

	// Rename file
	newPath, err := rename(path, title, tags, identifier)
	if err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	// Update path in in-memory metadata if it changed
	if newPath != path {
		if err := fs.UpdateNotePath(identifier, newPath); err != nil {
			return fmt.Errorf("failed to update path: %w", err)
		}
	}

	return nil
}

func handleNewEvent(identifier string) error {
	// Retrieve metadata from in-memory state
	metadata, err := fs.GetNote(identifier)
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	// Get denote directory
	denoteDir := GetDir()

	// Create new note file using existing NewNote function
	path, err := NewNote(denoteDir, metadata.Title, metadata.Tags, "md-yaml", identifier)
	if err != nil {
		return fmt.Errorf("failed to create note file: %w", err)
	}

	// Update the path in the metadata
	if err := fs.UpdateNotePath(identifier, path); err != nil {
		return fmt.Errorf("failed to update path in metadata: %w", err)
	}

	log.Printf("created new note: %s at %s", identifier, path)
	return nil
}

func handleDeleteEvent(identifier string) error {
	// Get denote directory
	denoteDir := GetDir()

	// Find file matching pattern: <identifier>--*
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

	// Delete the file
	if err := os.Remove(matches[0]); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	log.Printf("deleted note: %s", matches[0])
	return nil
}
