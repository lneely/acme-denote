package disk

import (
	"denote/pkg/metadata"
	p9client "denote/internal/p9/client"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"9fans.net/go/plan9/client"
)

// HandleUpdateEvent handles 'u' events from the 9P server.
// When metadata is updated in 9P, sync it to the filesystem.
func HandleUpdateEvent(f *client.Fsys, identifier, denoteDir string) error {
	fields, err := p9client.ReadFields(f, identifier, "title", "keywords", "signature", "path")
	if err != nil {
		return err
	}

	title := fields["title"]
	keywords := fields["keywords"]
	signature := fields["signature"]
	path := fields["path"]

	var tags []string
	if keywords != "" {
		for _, tag := range strings.Split(keywords, ",") {
			tags = append(tags, strings.TrimSpace(tag))
		}
	}

	// Only update frontmatter if file exists (skip if not yet created via Put)
	if _, err := os.Stat(path); err == nil && SupportsFrontMatter(path) {
		ext := strings.ToLower(filepath.Ext(path))
		var fileType metadata.FileType
		switch ext {
		case ".org":
			fileType = metadata.FileTypeOrg
		case ".md":
			fileType = metadata.FileTypeMdYaml
		case ".txt":
			fileType = metadata.FileTypeTxt
		}

		fm := &metadata.FrontMatter{
			Title:      title,
			Tags:       tags,
			Identifier: identifier,
			Signature:  signature,
			FileType:   fileType,
		}

		if err := UpdateFrontMatter(path, fm); err != nil {
			log.Printf("failed to update front matter for %s: %v", identifier, err)
		}
	}

	// After updating content, check if a rename is needed
	dir := filepath.Dir(path)
	if dir == "." {
		dir = denoteDir
	}

	// Get original filename and extract extension by removing the denote components
	originalFilename := filepath.Base(path)
	// Find where the extension starts (after tags or after title)
	extStart := strings.Index(originalFilename, ".")
	if extStart == -1 {
		extStart = len(originalFilename)
	}
	fullExt := originalFilename[extStart:]

	filename := metadata.BuildFilename(identifier, signature, title, tags, fullExt)
	newPath := filepath.Join(dir, filename)

	if newPath != path {
		newPath = filepath.Clean(newPath)
		absNew, err := filepath.Abs(newPath)
		if err != nil {
			return fmt.Errorf("failed to resolve new path: %w", err)
		}
		absBase, err := filepath.Abs(denoteDir)
		if err != nil {
			return fmt.Errorf("failed to resolve base directory: %w", err)
		}
		if !strings.HasPrefix(absNew, absBase) {
			return fmt.Errorf("path traversal attempt: path outside base")
		}

		if err := p9client.WriteFile(f, "n/"+identifier+"/path", newPath); err != nil {
			return fmt.Errorf("failed to update path in 9p: %w", err)
		}
	}

	return nil
}

// HandleRenameEvent handles 'r' events from the 9P server.
// Delegates to Drename binary to perform the actual rename.
func HandleRenameEvent(f *client.Fsys, identifier, denoteDir string) error {
	cmd := exec.Command("Drename", "--id="+identifier)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("rename failed: %w", err)
	}
	return nil
}

// HandleDeleteEvent handles 'd' events from the 9P server.
// When a note is deleted in 9P, delete the file from the filesystem.
func HandleDeleteEvent(f *client.Fsys, identifier, denoteDir string) error {
	// Get path from metadata to find correct directory
	path, err := p9client.ReadFile(f, "n/"+identifier+"/path")
	if err != nil || path == "" {
		// Fallback to denoteDir if path not set
		path = denoteDir
	}

	// Use directory from metadata path
	dir := filepath.Dir(path)
	if dir == "." {
		dir = denoteDir
	}

	pattern := filepath.Join(dir, identifier+"*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to find file: %w", err)
	}

	if len(matches) == 0 {
		return nil
	}

	if len(matches) > 1 {
		log.Printf("warning: multiple files match identifier %s, deleting first: %s", identifier, matches[0])
	}

	if err := os.Remove(matches[0]); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}
