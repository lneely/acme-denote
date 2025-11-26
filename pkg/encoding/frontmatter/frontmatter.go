package frontmatter

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
}

// Marshal returns the formatted frontmatter content as bytes
func Marshal(fm *FrontMatter, fileType FileType) []byte {
	template := templates[fileType]
	dateStr := time.Now().Format("2006-01-02 Mon 15:04")

	// For org-mode, wrap date in brackets for timestamp
	if fileType == FileTypeOrg {
		dateStr = "[" + dateStr + "]"
	}

	keywordsStr := FormatTags(fm.Tags, fileType)
	content := fmt.Sprintf(template, fm.Title, dateStr, keywordsStr, fm.Identifier, fm.Signature)
	return []byte(content)
}

// Unmarshal extracts front matter from file content.
// ext should be the file extension (e.g., ".md", ".org", ".txt").
// Returns the parsed frontmatter and the detected FileType.
func Unmarshal(content string, ext string) (*FrontMatter, FileType, error) {
	ext = strings.ToLower(ext)
	text := content

	fm := &FrontMatter{}
	var fileType FileType

	switch ext {
	case ".org":
		fileType = FileTypeOrg
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
			fileType = FileTypeMdYaml
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
				fileType = FileTypeMdToml
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
		fileType = FileTypeTxt
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

	return fm, fileType, nil
}

// New creates a new FrontMatter struct from given parameters
func New(title, signature string, tags []string, identifier string) *FrontMatter {
	return &FrontMatter{
		Title:      title,
		Tags:       tags,
		Identifier: identifier,
		Signature:  signature,
	}
}
