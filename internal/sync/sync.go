/*
The sync package synchronizes the underlying filesystem with the 9P virtual filesystem by loading
the denote metadata from the underlying filesystem, and refreshing the in-memory metadata. Files
with denote front matter use the front matter, and all other files use the metadata encoded in
the file name.
*/
package sync

import (
	p9client "denote/internal/p9/client"
	p9server "denote/internal/p9/server"
	"denote/internal/tmpl"
	"fmt"
	"log"
	"os"
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

			// Check if note exists in 9P
			exists := noteExists(identifier)

			if exists {
				// Existing note: sync front matter (triggers 'u' event)
				if err := syncFrontMatter(ev.Name, identifier); err != nil {
					log.Printf("sync: failed to sync %s: %v", ev.Name, err)
				}
			} else {
				// New note: register with 9P (triggers 'n' event)
				if err := registerNewNote(ev.Name); err != nil {
					log.Printf("sync: failed to register %s: %v", ev.Name, err)
				}
			}
		}
	}
}

// isDenoteFile checks if the file is in the denote directory
func isDenoteFile(path string) bool {
	denoteDir := os.Getenv("HOME") + "/doc"
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

// noteExists checks if a note exists in 9P
func noteExists(identifier string) bool {
	err := p9client.With9P(func(f *client.Fsys) error {
		titlePath := "n/" + identifier + "/title"
		fid, err := f.Open(titlePath, 0)
		if err != nil {
			return err
		}
		fid.Close()
		return nil
	})
	return err == nil
}

// registerNewNote writes to /new to register a new note (triggers 'n' event)
func registerNewNote(path string) error {
	meta := p9server.ExtractMetadata(path)

	// Format as 'title' tag1,tag2
	var newInput string
	if len(meta.Tags) > 0 {
		newInput = fmt.Sprintf("'%s' %s", meta.Title, strings.Join(meta.Tags, ","))
	} else {
		newInput = fmt.Sprintf("'%s'", meta.Title)
	}

	return p9client.With9P(func(f *client.Fsys) error {
		return p9client.WriteFile(f, "new", newInput)
	})
}

// syncFrontMatter reads the file's front matter and writes it to 9P metadata
func syncFrontMatter(path, identifier string) error {
	// Extract front matter from file
	fm, err := tmpl.Extract(path)
	if err != nil {
		return err
	}

	// Write to 9P filesystem
	return p9client.With9P(func(f *client.Fsys) error {
		// Write title (triggers 'u' event)
		titlePath := "n/" + identifier + "/title"
		if err := p9client.WriteFile(f, titlePath, fm.Title); err != nil {
			return err
		}

		// Write keywords (triggers 'u' event)
		keywords := strings.Join(fm.Tags, ",")
		keywordsPath := "n/" + identifier + "/keywords"
		if err := p9client.WriteFile(f, keywordsPath, keywords); err != nil {
			return err
		}

		// Trigger rename event
		ctlPath := "n/" + identifier + "/ctl"
		if err := p9client.WriteFile(f, ctlPath, "r"); err != nil {
			return err
		}

		return nil
	})
}

// SyncAll syncs all notes in the denote directory
func SyncAll() error {
	return p9client.With9P(func(f *client.Fsys) error {
		// Read index to get all identifiers
		indexFid, err := f.Open("index", 0)
		if err != nil {
			return err
		}
		defer indexFid.Close()

		var content []byte
		buf := make([]byte, 8192)
		for {
			n, err := indexFid.Read(buf)
			if err != nil && err.Error() != "EOF" {
				return err
			}
			if n == 0 {
				break
			}
			content = append(content, buf[:n]...)
		}

		// Parse identifiers from index
		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Parse identifier from line (format: "denote:ID | title | tags")
			parts := strings.SplitN(line, "|", 2)
			if len(parts) < 1 {
				continue
			}

			identifier := strings.TrimSpace(strings.TrimPrefix(parts[0], "denote:"))
			if identifier == "" {
				continue
			}

			// Read path and extension
			pathFid, err := f.Open("n/"+identifier+"/path", 0)
			if err != nil {
				log.Printf("sync: failed to read path for %s: %v", identifier, err)
				continue
			}
			pathBuf := make([]byte, 8192)
			n, _ := pathFid.Read(pathBuf)
			pathFid.Close()
			path := strings.TrimSpace(string(pathBuf[:n]))

			extFid, err := f.Open("n/"+identifier+"/extension", 0)
			if err != nil {
				log.Printf("sync: failed to read extension for %s: %v", identifier, err)
				continue
			}
			extBuf := make([]byte, 8192)
			n, _ = extFid.Read(extBuf)
			extFid.Close()
			ext := strings.TrimSpace(string(extBuf[:n]))

			// Route to appropriate sync function
			if ext == ".org" || ext == ".md" || ext == ".txt" {
				if err := syncDenoteFile(f, path, identifier); err != nil {
					log.Printf("sync: failed to sync denote file %s: %v", identifier, err)
				}
			} else {
				if err := syncNonDenoteFile(f, path, identifier); err != nil {
					log.Printf("sync: failed to sync non-denote file %s: %v", identifier, err)
				}
			}
		}

		return nil
	})
}

// syncDenoteFile syncs a text file with front matter
func syncDenoteFile(f *client.Fsys, path, identifier string) error {
	// Extract front matter from file
	fm, err := tmpl.Extract(path)
	if err != nil {
		return err
	}

	// Write title (triggers 'u' event)
	titlePath := "n/" + identifier + "/title"
	if err := p9client.WriteFile(f, titlePath, fm.Title); err != nil {
		return err
	}

	// Write keywords (triggers 'u' event)
	keywords := strings.Join(fm.Tags, ",")
	keywordsPath := "n/" + identifier + "/keywords"
	if err := p9client.WriteFile(f, keywordsPath, keywords); err != nil {
		return err
	}

	// Trigger rename event
	ctlPath := "n/" + identifier + "/ctl"
	if err := p9client.WriteFile(f, ctlPath, "r"); err != nil {
		return err
	}

	return nil
}

// syncNonDenoteFile syncs a binary file (metadata from filename)
func syncNonDenoteFile(f *client.Fsys, path, identifier string) error {
	// Extract metadata from filename
	meta := p9server.ExtractMetadata(path)

	// Write title (triggers 'u' event)
	titlePath := "n/" + identifier + "/title"
	if err := p9client.WriteFile(f, titlePath, meta.Title); err != nil {
		return err
	}

	// Write keywords (triggers 'u' event)
	keywords := strings.Join(meta.Tags, ",")
	keywordsPath := "n/" + identifier + "/keywords"
	if err := p9client.WriteFile(f, keywordsPath, keywords); err != nil {
		return err
	}

	// Don't trigger rename - filename is already correct

	return nil
}
