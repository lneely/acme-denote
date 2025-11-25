package metadata

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Templates for denote frontmatter (org, md, txt)
var templates = map[string]string{
	"org": `#+title:      %s
#+date:       %s
#+filetags:   %s
#+identifier: %s
#+signature:  %s

`,
	"md-yaml": `---
title:      %s
date:       %s
tags:       %s
identifier: %s
signature:  %s
---

`,
	"md-toml": `+++
title      = %s
date       = %s
tags       = %s
identifier = %s
signature  = %s
+++

`,
	"txt": `title:      %s
date:       %s
tags:       %s
identifier: %s
signature:  %s
---------------------------

`,
}

// FileExtensions contains the list of file extensions
// for which Denote should add front matter.
var FileExtensions = map[string]string{
	"org":     ".org",
	"md-yaml": ".md",
	"md-toml": ".md",
	"txt":     ".txt",
}

// FormatTags formats tags according to file type
func FormatTags(tags []string, fileType string) string {
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
	Signature  string
	FileType   string // org, md-yaml, md-toml, txt
}

// ParseFrontMatter extracts front matter from file content.
// ext should be the file extension (e.g., ".md", ".org", ".txt").
func ParseFrontMatter(content string, ext string) (*FrontMatter, error) {
	ext = strings.ToLower(ext)
	text := content

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
		if m := regexp.MustCompile(`(?m)^#\+signature:\s*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Signature = strings.TrimSpace(m[1])
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
			if m := regexp.MustCompile(`(?m)^signature:\s*["']?(.+?)["']?$`).FindStringSubmatch(yamlContent); m != nil {
				fm.Signature = strings.TrimSpace(m[1])
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
				if m := regexp.MustCompile(`(?m)^signature\s*=\s*["']?(.+?)["']?$`).FindStringSubmatch(tomlContent); m != nil {
					fm.Signature = strings.TrimSpace(m[1])
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
		if m := regexp.MustCompile(`(?m)^signature:\s*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Signature = strings.TrimSpace(m[1])
		}
	}

	return fm, nil
}

// UpdateFrontMatter updates front matter in file content and returns the new content.
// originalContent is the current file content, fm is the new front matter to apply.
func UpdateFrontMatter(originalContent string, fm *FrontMatter) (string, error) {
	text := originalContent

	dateStr := time.Now().Format("2006-01-02 Mon 15:04")
	keywordsStr := FormatTags(fm.Tags, fm.FileType)

	// For org-mode, wrap date in brackets for timestamp
	if fm.FileType == "org" {
		dateStr = "[" + dateStr + "]"
	}

	template := templates[fm.FileType]
	newFrontMatter := fmt.Sprintf(template, fm.Title, dateStr, keywordsStr, fm.Identifier, fm.Signature)

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
		// Skip any blank lines after frontmatter since template includes one
		for endIdx < len(lines) && lines[endIdx] == "" {
			endIdx++
		}
		if endIdx > 0 {
			newText = newFrontMatter + strings.Join(lines[endIdx:], "\n")
		} else {
			newText = newFrontMatter + text
		}

	case "md-yaml":
		// Replace YAML front matter
		re := regexp.MustCompile(`(?s)^---\n.*?\n---\n`)
		if re.MatchString(text) {
			newText = re.ReplaceAllString(text, newFrontMatter)
		} else {
			newText = newFrontMatter + text
		}

	case "md-toml":
		// Replace TOML front matter
		re := regexp.MustCompile(`(?s)^\+\+\+\n.*?\n\+\+\+\n`)
		if re.MatchString(text) {
			newText = re.ReplaceAllString(text, newFrontMatter)
		} else {
			newText = newFrontMatter + text
		}

	case "txt":
		// Replace text front matter
		re := regexp.MustCompile(`(?s)^title:.*?\n-+\n`)
		if re.MatchString(text) {
			newText = re.ReplaceAllString(text, newFrontMatter)
		} else {
			newText = newFrontMatter + text
		}
	default:
		return "", fmt.Errorf("unsupported file type: %s", fm.FileType)
	}

	return newText, nil
}

// Generate generates front matter content for given parameters
func Generate(title, signature string, tags []string, fileType, identifier string) string {
	template := templates[fileType]
	dateStr := time.Now().Format("2006-01-02 Mon 15:04")

	// For org-mode, wrap date in brackets for timestamp
	if fileType == "org" {
		dateStr = "[" + dateStr + "]"
	}

	keywordsStr := FormatTags(tags, fileType)
	return fmt.Sprintf(template, title, dateStr, keywordsStr, identifier, signature)
}
