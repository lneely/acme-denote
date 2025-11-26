package metadata

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Templates for denote frontmatter (org, md, txt)
var templates = map[FileType]string{
	FileTypeOrg: `#+title:      %s
#+date:       %s
#+filetags:   %s
#+identifier: %s
#+signature:  %s

`,
	FileTypeMdYaml: `---
title:      %s
date:       %s
tags:       %s
identifier: %s
signature:  %s
---

`,
	FileTypeMdToml: `+++
title      = %s
date       = %s
tags       = %s
identifier = %s
signature  = %s
+++

`,
	FileTypeTxt: `title:      %s
date:       %s
tags:       %s
identifier: %s
signature:  %s
---------------------------

`,
}

// FileType represents supported file formats for denote notes
type FileType string

const (
	FileTypeOrg    FileType = "org"
	FileTypeMdYaml FileType = "md-yaml"
	FileTypeMdToml FileType = "md-toml"
	FileTypeTxt    FileType = "txt"
)

// fileExtensions contains the list of file extensions
// for which Denote should add front matter.
var fileExtensions = map[FileType]string{
	FileTypeOrg:    ".org",
	FileTypeMdYaml: ".md",
	FileTypeMdToml: ".md",
	FileTypeTxt:    ".txt",
}

// GetExtension returns the file extension for a given file type.
func GetExtension(fileType FileType) string {
	return fileExtensions[fileType]
}

// FormatTags formats tags according to file type
func FormatTags(tags []string, fileType FileType) string {
	if len(tags) == 0 {
		return ""
	}
	switch fileType {
	case FileTypeOrg:
		return ":" + strings.Join(tags, ":") + ":"
	case FileTypeMdYaml, FileTypeMdToml:
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
	FileType   FileType
}

// Bytes returns the formatted frontmatter content as bytes
func (fm *FrontMatter) Bytes() []byte {
	template := templates[fm.FileType]
	dateStr := time.Now().Format("2006-01-02 Mon 15:04")

	// For org-mode, wrap date in brackets for timestamp
	if fm.FileType == FileTypeOrg {
		dateStr = "[" + dateStr + "]"
	}

	keywordsStr := FormatTags(fm.Tags, fm.FileType)
	content := fmt.Sprintf(template, fm.Title, dateStr, keywordsStr, fm.Identifier, fm.Signature)
	return []byte(content)
}

// ParseFrontMatter extracts front matter from file content.
// ext should be the file extension (e.g., ".md", ".org", ".txt").
func ParseFrontMatter(content string, ext string) (*FrontMatter, error) {
	ext = strings.ToLower(ext)
	text := content

	fm := &FrontMatter{}

	switch ext {
	case ".org":
		fm.FileType = FileTypeOrg
		if m := regexp.MustCompile(`(?m)^#\+title:[ \t]*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Title = strings.TrimSpace(m[1])
		}
		if m := regexp.MustCompile(`(?m)^#\+filetags:[ \t]*:(.+):$`).FindStringSubmatch(text); m != nil {
			fm.Tags = strings.Split(m[1], ":")
		}
		if m := regexp.MustCompile(`(?m)^#\+identifier:[ \t]*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Identifier = strings.TrimSpace(m[1])
		}
		if m := regexp.MustCompile(`(?m)^#\+signature:[ \t]*(.*)$`).FindStringSubmatch(text); m != nil {
			fm.Signature = strings.TrimSpace(m[1])
		}

	case ".md":
		// Try YAML first
		yamlRe := regexp.MustCompile(`(?ms)^---\n(.*?)\n---`)
		if m := yamlRe.FindStringSubmatch(text); m != nil {
			fm.FileType = FileTypeMdYaml
			yamlContent := m[1]
			if m := regexp.MustCompile(`(?m)^title:[ \t]*["']?(.+?)["']?$`).FindStringSubmatch(yamlContent); m != nil {
				fm.Title = strings.TrimSpace(m[1])
			}
			if m := regexp.MustCompile(`(?m)^tags:[ \t]*\[(.+?)\]$`).FindStringSubmatch(yamlContent); m != nil {
				tags := strings.Split(m[1], ",")
				for i, t := range tags {
					tags[i] = strings.TrimSpace(t)
				}
				fm.Tags = tags
			}
			if m := regexp.MustCompile(`(?m)^identifier:[ \t]*["']?(.+?)["']?$`).FindStringSubmatch(yamlContent); m != nil {
				fm.Identifier = strings.TrimSpace(m[1])
			}
			if m := regexp.MustCompile(`(?m)^signature:[ \t]*["']?(.*)["']?$`).FindStringSubmatch(yamlContent); m != nil {
				fm.Signature = strings.TrimSpace(m[1])
			}
		} else {
			// Try TOML
			tomlRe := regexp.MustCompile(`(?ms)^\+\+\+\n(.*?)\n\+\+\+`)
			if m := tomlRe.FindStringSubmatch(text); m != nil {
				fm.FileType = FileTypeMdToml
				tomlContent := m[1]
				if m := regexp.MustCompile(`(?m)^title[ \t]*=[ \t]*["']?(.+?)["']?$`).FindStringSubmatch(tomlContent); m != nil {
					fm.Title = strings.TrimSpace(m[1])
				}
				if m := regexp.MustCompile(`(?m)^tags[ \t]*=[ \t]*\[(.+?)\]$`).FindStringSubmatch(tomlContent); m != nil {
					tags := strings.Split(m[1], ",")
					for i, t := range tags {
						tags[i] = strings.TrimSpace(t)
					}
					fm.Tags = tags
				}
				if m := regexp.MustCompile(`(?m)^identifier[ \t]*=[ \t]*["']?(.+?)["']?$`).FindStringSubmatch(tomlContent); m != nil {
					fm.Identifier = strings.TrimSpace(m[1])
				}
				if m := regexp.MustCompile(`(?m)^signature[ \t]*=[ \t]*["']?(.*)["']?$`).FindStringSubmatch(tomlContent); m != nil {
					fm.Signature = strings.TrimSpace(m[1])
				}
			}
		}

	case ".txt":
		fm.FileType = FileTypeTxt
		if m := regexp.MustCompile(`(?m)^title:[ \t]*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Title = strings.TrimSpace(m[1])
		}
		if m := regexp.MustCompile(`(?m)^tags:[ \t]*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Tags = strings.Fields(m[1])
		}
		if m := regexp.MustCompile(`(?m)^identifier:[ \t]*(.+)$`).FindStringSubmatch(text); m != nil {
			fm.Identifier = strings.TrimSpace(m[1])
		}
		if m := regexp.MustCompile(`(?m)^signature:[ \t]*(.*)$`).FindStringSubmatch(text); m != nil {
			fm.Signature = strings.TrimSpace(m[1])
		}
	}

	return fm, nil
}

// Apply applies front matter to file content, replacing existing front matter if present.
// originalContent is the current file content, fm is the new front matter to apply.
func Apply(originalContent string, fm *FrontMatter) (string, error) {
	text := originalContent
	newFrontMatter := string(fm.Bytes())

	var newText string
	switch fm.FileType {
	case FileTypeOrg:
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

	case FileTypeMdYaml:
		// Replace YAML front matter (match trailing blank lines to avoid duplication)
		re := regexp.MustCompile(`(?s)^---\n.*?\n---\n\n*`)
		if re.MatchString(text) {
			newText = re.ReplaceAllString(text, newFrontMatter)
		} else {
			newText = newFrontMatter + text
		}

	case FileTypeMdToml:
		// Replace TOML front matter (match trailing blank lines to avoid duplication)
		re := regexp.MustCompile(`(?s)^\+\+\+\n.*?\n\+\+\+\n\n*`)
		if re.MatchString(text) {
			newText = re.ReplaceAllString(text, newFrontMatter)
		} else {
			newText = newFrontMatter + text
		}

	case FileTypeTxt:
		// Replace text front matter (match trailing blank lines to avoid duplication)
		re := regexp.MustCompile(`(?s)^title:.*?\n-+\n\n*`)
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

// NewFrontMatter creates a new FrontMatter struct from given parameters
func NewFrontMatter(title, signature string, tags []string, fileType FileType, identifier string) *FrontMatter {
	return &FrontMatter{
		Title:      title,
		Tags:       tags,
		Identifier: identifier,
		Signature:  signature,
		FileType:   fileType,
	}
}
