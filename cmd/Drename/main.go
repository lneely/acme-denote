package main

import (
	p9client "denote/internal/p9/client"
	"denote/pkg/encoding/frontmatter"
	"denote/pkg/util"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"9fans.net/go/acme"
	"9fans.net/go/plan9/client"
)

var denoteDir = os.Getenv("HOME") + "/doc"

func main() {
	// Check if running in service mode (called by OnRename callback)
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "--id=") {
		identifier := strings.TrimPrefix(os.Args[1], "--id=")
		if err := serviceMode(identifier); err != nil {
			fmt.Fprintf(os.Stderr, "drename: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Window mode (called by user from acme window)
	if err := windowMode(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "drename: %v\n", err)
		os.Exit(1)
	}
}

// serviceMode handles rename when called by OnRename callback
// This replicates the logic from HandleRenameEvent
func serviceMode(identifier string) error {
	return p9client.With9P(func(f *client.Fsys) error {
		// Get new path from 9P server
		newPath, err := p9client.ReadFile(f, "n/"+identifier+"/path")
		if err != nil {
			return fmt.Errorf("failed to read path: %w", err)
		}

		if newPath == "" {
			// Path is not set yet. This can happen during note creation.
			return nil
		}

		// Find old file on disk (matches with or without signature)
		pattern := filepath.Join(denoteDir, identifier+"*")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return fmt.Errorf("failed to find file: %w", err)
		}

		if len(matches) == 0 {
			// File doesn't exist on disk. Nothing to rename.
			return nil
		}
		oldPath := matches[0]

		// Rename if different
		if oldPath != newPath {
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

			if err := os.Rename(oldPath, newPath); err != nil {
				return fmt.Errorf("failed to rename file from %s to %s: %w", oldPath, newPath, err)
			}

			// Update window tag for renamed file
			if wins, err := acme.Windows(); err == nil {
				for _, w := range wins {
					if err := updateWindowName(w.ID, oldPath, newPath); err != nil {
						log.Printf("failed to update window %d: %v", w.ID, err)
					}
				}
			}
		}

		return nil
	})
}

// windowMode handles rename when called by user from acme window
func windowMode(args []string) error {
	// Get $winid
	winidStr := os.Getenv("winid")
	if winidStr == "" {
		return fmt.Errorf("$winid not set - must be called from acme window")
	}
	winid, err := strconv.Atoi(winidStr)
	if err != nil {
		return fmt.Errorf("invalid $winid: %w", err)
	}

	// Open the window
	win, err := acme.Open(winid, nil)
	if err != nil {
		return fmt.Errorf("failed to open window: %w", err)
	}
	defer win.CloseFiles()

	// Read window name from tag
	tag, err := win.ReadAll("tag")
	if err != nil {
		return fmt.Errorf("failed to read window tag: %w", err)
	}
	windowName := strings.Fields(string(tag))[0]

	// Extract identifier from window name
	identifierPattern := regexp.MustCompile(`(\d{8}T\d{6})`)
	matches := identifierPattern.FindStringSubmatch(windowName)
	if len(matches) == 0 {
		return fmt.Errorf("no denote identifier found in window name: %s", windowName)
	}
	identifier := matches[1]

	// Parse arguments: 'Title' ==signature tag1,tag2,...
	input := strings.Join(args, " ")

	// Extract title (must be single-quoted)
	if !strings.HasPrefix(input, "'") {
		return fmt.Errorf("title must be single-quoted")
	}

	closeQuote := strings.Index(input[1:], "'")
	if closeQuote == -1 {
		return fmt.Errorf("title must be single-quoted (missing closing quote)")
	}

	title := input[1 : closeQuote+1]
	if title == "" {
		return fmt.Errorf("title cannot be empty")
	}

	// Extract signature and tags (everything after closing quote)
	remainder := strings.TrimSpace(input[closeQuote+2:])
	var signature string
	var tags []string

	if remainder != "" {
		// Check if signature is present (starts with ==)
		if strings.HasPrefix(remainder, "==") {
			// Find end of signature (space before tags)
			spaceIdx := strings.Index(remainder, " ")
			if spaceIdx == -1 {
				// No tags, just signature
				signature = remainder[2:] // Skip ==
			} else {
				signature = remainder[2:spaceIdx] // Skip ==
				remainder = strings.TrimSpace(remainder[spaceIdx+1:])
			}
		}

		// Extract tags if present
		if remainder != "" && !strings.HasPrefix(remainder, "==") {
			// Validate tags
			tagPattern := regexp.MustCompile(`^([\p{Ll}\p{Nd}]+,)*[\p{Ll}\p{Nd}]+$`)
			if !tagPattern.MatchString(remainder) {
				return fmt.Errorf("tags must be comma-delimited lowercase unicode words (no spaces)")
			}
			tags = strings.Split(remainder, ",")
		}
	}

	// Read window body to check for frontmatter
	body, err := win.ReadAll("body")
	if err != nil {
		return fmt.Errorf("failed to read window body: %w", err)
	}

	// Detect file type from window name
	ext := strings.ToLower(filepath.Ext(windowName))
	supportsFrontmatter := ext == ".org" || ext == ".md" || ext == ".txt"

	// Update frontmatter if applicable
	if supportsFrontmatter && len(body) > 0 {
		// Parse existing frontmatter to preserve Identifier and FileType
		existing, fileType, err := frontmatter.Unmarshal(string(body), ext)
		if err != nil {
			return fmt.Errorf("failed to parse existing frontmatter: %w", err)
		}

		// Update only the fields that changed from rename
		existing.Title = title
		existing.Tags = tags
		existing.Signature = signature

		// Apply updated frontmatter to content
		newContent, err := util.Apply(string(body), existing, fileType)
		if err != nil {
			return fmt.Errorf("failed to update frontmatter: %w", err)
		}

		// Write updated content back to window
		if err := win.Addr("0,$"); err != nil {
			return fmt.Errorf("failed to select window content: %w", err)
		}
		if _, err := win.Write("data", []byte(newContent)); err != nil {
			return fmt.Errorf("failed to write updated content: %w", err)
		}
		if err := win.Ctl("dirty"); err != nil {
			return fmt.Errorf("failed to mark window dirty: %w", err)
		}
	}

	// Write to 9P metadata (triggers OnUpdate and OnRename callbacks)
	return p9client.With9P(func(f *client.Fsys) error {
		// Write title
		if err := p9client.WriteFile(f, "n/"+identifier+"/title", title); err != nil {
			return fmt.Errorf("failed to write title: %w", err)
		}

		// Write keywords
		keywords := strings.Join(tags, ",")
		if err := p9client.WriteFile(f, "n/"+identifier+"/keywords", keywords); err != nil {
			return fmt.Errorf("failed to write keywords: %w", err)
		}

		// Write signature
		if err := p9client.WriteFile(f, "n/"+identifier+"/signature", signature); err != nil {
			return fmt.Errorf("failed to write signature: %w", err)
		}

		return nil
	})
}

// updateWindowName updates the window tag name from oldPath to newPath
func updateWindowName(id int, oldPath, newPath string) error {
	win, err := acme.Open(id, nil)
	if err != nil {
		return err
	}
	defer win.CloseFiles()

	tag, err := win.ReadAll("tag")
	if err != nil {
		return err
	}

	if !strings.Contains(string(tag), oldPath) {
		return nil // Window doesn't have this path, skip
	}

	if err := win.Ctl("name " + newPath); err != nil {
		return fmt.Errorf("failed to rename window: %w", err)
	}

	return nil
}
