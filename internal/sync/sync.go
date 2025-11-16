package sync

import (
	"denote/internal/denote"
	"denote/internal/fs"
	"log"
	"path/filepath"
	"regexp"
	"strings"

	"9fans.net/go/acme"
	"9fans.net/go/plan9/client"
)

// denoteFilePattern matches denote filenames: YYYYMMDDTHHMMSS--title__tags.ext
var denoteFilePattern = regexp.MustCompile(`^(\d{8}T\d{6})--`)

// WatchAcmeLog monitors Acme's log for Put events and syncs denote file
// front matter changes to the 9P filesystem metadata.
func WatchAcmeLog() {
	for {
		alog, err := acme.Log()
		if err != nil {
			log.Printf("sync: failed to open acme log: %v", err)
			return
		}

		for {
			ev, err := alog.Read()
			if err != nil {
				log.Printf("sync: error reading acme log: %v", err)
				alog.Close()
				break
			}

			// Only handle Put events
			if ev.Op != "put" {
				continue
			}

			// Check if this is a denote file
			if !isDenoteFile(ev.Name) {
				continue
			}

			// Extract identifier from filename
			identifier := extractIdentifier(ev.Name)
			if identifier == "" {
				continue
			}

			// Sync front matter to 9P metadata
			if err := syncFrontMatter(ev.Name, identifier); err != nil {
				log.Printf("sync: failed to sync %s: %v", ev.Name, err)
			}
		}
	}
}

// isDenoteFile checks if the file is in the denote directory
func isDenoteFile(path string) bool {
	denoteDir := denote.GetDir()
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return strings.HasPrefix(absPath, denoteDir)
}

// extractIdentifier extracts the identifier from a denote filename
func extractIdentifier(path string) string {
	filename := filepath.Base(path)
	matches := denoteFilePattern.FindStringSubmatch(filename)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// syncFrontMatter reads the file's front matter and writes it to 9P metadata
func syncFrontMatter(path, identifier string) error {
	// Extract front matter from file
	fm, err := denote.ExtractFrontMatter(path)
	if err != nil {
		return err
	}

	// Write to 9P filesystem
	return fs.With9P(func(f *client.Fsys) error {
		// Write title (triggers 'u' event)
		titlePath := identifier + "/title"
		if err := fs.WriteFile(f, titlePath, fm.Title); err != nil {
			return err
		}

		// Write keywords (triggers 'u' event)
		keywords := strings.Join(fm.Tags, ",")
		keywordsPath := identifier + "/keywords"
		if err := fs.WriteFile(f, keywordsPath, keywords); err != nil {
			return err
		}

		// Trigger rename event
		eventPath := identifier + "/event"
		if err := fs.WriteFile(f, eventPath, "r"); err != nil {
			return err
		}

		return nil
	})
}
