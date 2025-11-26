package metadata

import (
	"slices"
	"strings"
	"testing"
)

// TestFormatTags validates tag formatting for different file types
// Maps to dt-denote--format-front-matter from original tests
func TestFormatTags(t *testing.T) {
	tests := []struct {
		name     string
		tags     []string
		fileType FileType
		want     string
	}{
		{
			name:     "org with multiple tags",
			tags:     []string{"tag1", "tag2"},
			fileType: FileTypeOrg,
			want:     ":tag1:tag2:",
		},
		{
			name:     "org with single tag",
			tags:     []string{"single"},
			fileType: FileTypeOrg,
			want:     ":single:",
		},
		{
			name:     "org with empty tags",
			tags:     []string{},
			fileType: FileTypeOrg,
			want:     "",
		},
		{
			name:     "md-yaml with multiple tags",
			tags:     []string{"tag1", "tag2"},
			fileType: FileTypeMdYaml,
			want:     "[tag1, tag2]",
		},
		{
			name:     "md-yaml with single tag",
			tags:     []string{"single"},
			fileType: FileTypeMdYaml,
			want:     "[single]",
		},
		{
			name:     "md-yaml with empty tags",
			tags:     []string{},
			fileType: FileTypeMdYaml,
			want:     "",
		},
		{
			name:     "md-toml with multiple tags",
			tags:     []string{"tag1", "tag2"},
			fileType: FileTypeMdToml,
			want:     "[tag1, tag2]",
		},
		{
			name:     "md-toml with empty tags",
			tags:     []string{},
			fileType: FileTypeMdToml,
			want:     "",
		},
		{
			name:     "txt with multiple tags",
			tags:     []string{"tag1", "tag2"},
			fileType: FileTypeTxt,
			want:     "tag1 tag2",
		},
		{
			name:     "txt with single tag",
			tags:     []string{"single"},
			fileType: FileTypeTxt,
			want:     "single",
		},
		{
			name:     "txt with empty tags",
			tags:     []string{},
			fileType: FileTypeTxt,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatTags(tt.tags, tt.fileType)
			if got != tt.want {
				t.Errorf("FormatTags(%v, %q) = %q, want %q", tt.tags, tt.fileType, got, tt.want)
			}
		})
	}
}

// TestFrontMatterBytes validates front matter formatting for all file types
// Maps to dt-denote--format-front-matter from original tests
func TestFrontMatterBytes(t *testing.T) {
	identifier := "20240101T120000"
	title := "Test Note"
	tags := []string{"tag1", "tag2"}

	tests := []struct {
		name            string
		fileType        FileType
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:     "org format",
			fileType: FileTypeOrg,
			wantContains: []string{
				"#+title:      Test Note",
				"#+filetags:   :tag1:tag2:",
				"#+identifier: 20240101T120000",
				"#+date:",
			},
		},
		{
			name:     "md-yaml format",
			fileType: FileTypeMdYaml,
			wantContains: []string{
				"---",
				"title:      Test Note",
				"tags:       [tag1, tag2]",
				"identifier: 20240101T120000",
				"date:",
			},
		},
		{
			name:     "md-toml format",
			fileType: FileTypeMdToml,
			wantContains: []string{
				"+++",
				"title      = Test Note",
				"tags       = [tag1, tag2]",
				"identifier = 20240101T120000",
				"date       =",
			},
		},
		{
			name:     "txt format",
			fileType: FileTypeTxt,
			wantContains: []string{
				"title:      Test Note",
				"tags:       tag1 tag2",
				"identifier: 20240101T120000",
				"date:",
				"---------------------------",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := NewFrontMatter(title, "", tags, tt.fileType, identifier)
			got := string(fm.Bytes())

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("FrontMatter.Bytes() missing %q\nGot:\n%s", want, got)
				}
			}

			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("FrontMatter.Bytes() should not contain %q\nGot:\n%s", notWant, got)
				}
			}
		})
	}
}

// TestFrontMatterBytesEmptyTags validates front matter formatting with no tags
func TestFrontMatterBytesEmptyTags(t *testing.T) {
	identifier := "20240101T120000"
	title := "Test Note"
	tags := []string{}

	tests := []struct {
		name     string
		fileType FileType
		wantTags string
	}{
		{
			name:     "org with empty tags",
			fileType: FileTypeOrg,
			wantTags: "#+filetags:",
		},
		{
			name:     "md-yaml with empty tags",
			fileType: FileTypeMdYaml,
			wantTags: "tags:",
		},
		{
			name:     "md-toml with empty tags",
			fileType: FileTypeMdToml,
			wantTags: "tags       =",
		},
		{
			name:     "txt with empty tags",
			fileType: FileTypeTxt,
			wantTags: "tags:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fm := NewFrontMatter(title, "", tags, tt.fileType, identifier)
			got := string(fm.Bytes())

			if !strings.Contains(got, tt.wantTags) {
				t.Errorf("FrontMatter.Bytes() with empty tags should contain %q\nGot:\n%s",
					tt.wantTags, got)
			}
		})
	}
}

// TestParseFrontMatter validates front matter parsing from content
func TestParseFrontMatter(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		ext            string
		wantTitle      string
		wantTags       []string
		wantIdentifier string
		wantFileType   FileType
	}{
		{
			name: "org format",
			content: `#+title: Org Note
#+date: [2024-01-01 Mon 12:00]
#+filetags: :work:emacs:
#+identifier: 20240101T120000

* First Heading`,
			ext:            ".org",
			wantTitle:      "Org Note",
			wantTags:       []string{"work", "emacs"},
			wantIdentifier: "20240101T120000",
			wantFileType:   FileTypeOrg,
		},
		{
			name: "org with single tag",
			content: `#+title: Single Tag
#+filetags: :single:
#+identifier: 20240101T120000`,
			ext:            ".org",
			wantTitle:      "Single Tag",
			wantTags:       []string{"single"},
			wantIdentifier: "20240101T120000",
			wantFileType:   FileTypeOrg,
		},
		{
			name: "org without tags",
			content: `#+title: No Tags
#+identifier: 20240101T120000`,
			ext:            ".org",
			wantTitle:      "No Tags",
			wantTags:       nil,
			wantIdentifier: "20240101T120000",
			wantFileType:   FileTypeOrg,
		},
		{
			name: "markdown yaml",
			content: `---
title: Markdown Note
date: 2024-01-01
tags: [work, personal]
identifier: 20240101T120000
---

# Content`,
			ext:            ".md",
			wantTitle:      "Markdown Note",
			wantTags:       []string{"work", "personal"},
			wantIdentifier: "20240101T120000",
			wantFileType:   FileTypeMdYaml,
		},
		{
			name: "markdown yaml with quoted title",
			content: `---
title: "Quoted Title"
tags: [test]
identifier: 20240101T120000
---`,
			ext:            ".md",
			wantTitle:      "Quoted Title",
			wantTags:       []string{"test"},
			wantIdentifier: "20240101T120000",
			wantFileType:   FileTypeMdYaml,
		},
		{
			name: "markdown toml",
			content: `+++
title = TOML Note
date = 2024-01-01
tags = [rust, go]
identifier = 20240101T120000
+++

Content`,
			ext:            ".md",
			wantTitle:      "TOML Note",
			wantTags:       []string{"rust", "go"},
			wantIdentifier: "20240101T120000",
			wantFileType:   FileTypeMdToml,
		},
		{
			name: "txt format",
			content: `title: Plain Text
date: 2024-01-01
tags: simple plain
identifier: 20240101T120000
---------------------------

Content here`,
			ext:            ".txt",
			wantTitle:      "Plain Text",
			wantTags:       []string{"simple", "plain"},
			wantIdentifier: "20240101T120000",
			wantFileType:   FileTypeTxt,
		},
		{
			name: "txt without tags",
			content: `title: No Tags Text
identifier: 20240101T120000
---------------------------`,
			ext:            ".txt",
			wantTitle:      "No Tags Text",
			wantTags:       nil,
			wantIdentifier: "20240101T120000",
			wantFileType:   FileTypeTxt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFrontMatter(tt.content, tt.ext)
			if err != nil {
				t.Fatalf("ParseFrontMatter() error = %v", err)
			}

			if got.Title != tt.wantTitle {
				t.Errorf("ParseFrontMatter().Title = %q, want %q", got.Title, tt.wantTitle)
			}

			if !slices.Equal(got.Tags, tt.wantTags) {
				t.Errorf("ParseFrontMatter().Tags = %v, want %v", got.Tags, tt.wantTags)
			}

			if got.Identifier != tt.wantIdentifier {
				t.Errorf("ParseFrontMatter().Identifier = %q, want %q", got.Identifier, tt.wantIdentifier)
			}

			if got.FileType != tt.wantFileType {
				t.Errorf("ParseFrontMatter().FileType = %q, want %q", got.FileType, tt.wantFileType)
			}
		})
	}
}

// TestParseFrontMatterMissingFields validates handling of missing fields
func TestParseFrontMatterMissingFields(t *testing.T) {
	content := `---
date: 2024-01-01
---`

	got, err := ParseFrontMatter(content, ".md")
	if err != nil {
		t.Fatalf("ParseFrontMatter() error = %v", err)
	}

	// Should have empty values, not error
	if got.Title != "" {
		t.Errorf("ParseFrontMatter().Title = %q, want empty", got.Title)
	}
	if got.Identifier != "" {
		t.Errorf("ParseFrontMatter().Identifier = %q, want empty", got.Identifier)
	}
	if got.Tags != nil {
		t.Errorf("ParseFrontMatter().Tags = %v, want nil", got.Tags)
	}
}

// TestApply validates front matter updates
func TestApply(t *testing.T) {
	tests := []struct {
		name         string
		original     string
		fm           *FrontMatter
		wantContains []string
		wantPreserve string
	}{
		{
			name: "update org front matter",
			original: `#+title: Old Title
#+date: [2024-01-01 Mon 12:00]
#+filetags: :old:
#+identifier: 20240101T120000

* Original Content
This should be preserved`,
			fm: &FrontMatter{
				Title:      "New Title",
				Tags:       []string{"new", "updated"},
				Identifier: "20240101T120000",
				FileType:   FileTypeOrg,
			},
			wantContains: []string{
				"#+title:      New Title",
				"#+filetags:   :new:updated:",
				"* Original Content",
				"This should be preserved",
			},
			wantPreserve: "* Original Content",
		},
		{
			name: "update markdown yaml front matter",
			original: `---
title: Old Title
tags: [old]
identifier: 20240101T120000
---

# Original Heading
Content preserved`,
			fm: &FrontMatter{
				Title:      "New Title",
				Tags:       []string{"new"},
				Identifier: "20240101T120000",
				FileType:   FileTypeMdYaml,
			},
			wantContains: []string{
				"---",
				"title:      New Title",
				"tags:       [new]",
				"# Original Heading",
				"Content preserved",
			},
			wantPreserve: "# Original Heading",
		},
		{
			name: "update markdown toml front matter",
			original: `+++
title = Old Title
tags = [old]
identifier = 20240101T120000
+++

Content here`,
			fm: &FrontMatter{
				Title:      "New Title",
				Tags:       []string{"updated"},
				Identifier: "20240101T120000",
				FileType:   FileTypeMdToml,
			},
			wantContains: []string{
				"+++",
				"title      = New Title",
				"tags       = [updated]",
				"Content here",
			},
			wantPreserve: "Content here",
		},
		{
			name: "update txt front matter",
			original: `title: Old Title
tags: old
identifier: 20240101T120000
---------------------------

Text content`,
			fm: &FrontMatter{
				Title:      "New Title",
				Tags:       []string{"new"},
				Identifier: "20240101T120000",
				FileType:   FileTypeTxt,
			},
			wantContains: []string{
				"title:      New Title",
				"tags:       new",
				"---------------------------",
				"Text content",
			},
			wantPreserve: "Text content",
		},
		{
			name:     "add front matter when missing (org)",
			original: `* Original Heading`,
			fm: &FrontMatter{
				Title:      "Added Title",
				Tags:       []string{"added"},
				Identifier: "20240101T120000",
				FileType:   FileTypeOrg,
			},
			wantContains: []string{
				"#+title:      Added Title",
				"#+filetags:   :added:",
				"* Original Heading",
			},
			wantPreserve: "* Original Heading",
		},
		{
			name:     "add front matter when missing (md-yaml)",
			original: `# Original Heading`,
			fm: &FrontMatter{
				Title:      "Added Title",
				Tags:       []string{"added"},
				Identifier: "20240101T120000",
				FileType:   FileTypeMdYaml,
			},
			wantContains: []string{
				"---",
				"title:      Added Title",
				"tags:       [added]",
				"# Original Heading",
			},
			wantPreserve: "# Original Heading",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Apply(tt.original, tt.fm)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Apply() missing %q\nGot:\n%s", want, got)
				}
			}

			// Verify original content is preserved
			if !strings.Contains(got, tt.wantPreserve) {
				t.Errorf("Apply() didn't preserve %q\nGot:\n%s", tt.wantPreserve, got)
			}
		})
	}
}

// TestApplyEmptyTags validates updating with empty tags
func TestApplyEmptyTags(t *testing.T) {
	original := `---
title: Test
tags: [old, tags]
identifier: 20240101T120000
---

Content`

	fm := &FrontMatter{
		Title:      "Test",
		Tags:       []string{},
		Identifier: "20240101T120000",
		FileType:   FileTypeMdYaml,
	}

	got, err := Apply(original, fm)
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	// Should have empty tags field, not omit it
	if !strings.Contains(got, "tags:") {
		t.Errorf("Apply() should include tags field even when empty")
	}

	// Content should be preserved
	if !strings.Contains(got, "Content") {
		t.Errorf("Apply() should preserve content")
	}
}

// TestApplyUnsupportedType validates error handling
func TestApplyUnsupportedType(t *testing.T) {
	fm := &FrontMatter{
		Title:      "Test",
		Tags:       []string{"test"},
		Identifier: "20240101T120000",
		FileType:   "unsupported",
	}

	_, err := Apply("content", fm)
	if err == nil {
		t.Error("Apply() should error on unsupported file type")
	}

	if !strings.Contains(err.Error(), "unsupported file type") {
		t.Errorf("Apply() error = %v, want 'unsupported file type'", err)
	}
}

// TestGetExtension validates file extension mapping
func TestGetExtension(t *testing.T) {
	tests := []struct {
		fileType FileType
		want     string
	}{
		{FileTypeOrg, ".org"},
		{FileTypeMdYaml, ".md"},
		{FileTypeMdToml, ".md"},
		{FileTypeTxt, ".txt"},
	}

	for _, tt := range tests {
		t.Run(string(tt.fileType), func(t *testing.T) {
			got := GetExtension(tt.fileType)
			if got != tt.want {
				t.Errorf("GetExtension(%q) = %q, want %q", tt.fileType, got, tt.want)
			}
		})
	}
}

// TestFrontMatterRoundTrip validates generate -> parse -> update cycle
func TestFrontMatterRoundTrip(t *testing.T) {
	fileTypes := []FileType{FileTypeOrg, FileTypeMdYaml, FileTypeMdToml, FileTypeTxt}
	title := "Test Note"
	tags := []string{"tag1", "tag2"}
	identifier := "20240101T120000"

	for _, fileType := range fileTypes {
		t.Run(string(fileType), func(t *testing.T) {
			// Generate front matter
			fm := NewFrontMatter(title, "", tags, fileType, identifier)
			content := string(fm.Bytes())

			// Parse it back
			ext := GetExtension(fileType)
			fm, err := ParseFrontMatter(content, ext)
			if err != nil {
				t.Fatalf("ParseFrontMatter() error = %v", err)
			}

			// Verify parsed values
			if fm.Title != title {
				t.Errorf("Parsed title = %q, want %q", fm.Title, title)
			}
			if !slices.Equal(fm.Tags, tags) {
				t.Errorf("Parsed tags = %v, want %v", fm.Tags, tags)
			}
			if fm.Identifier != identifier {
				t.Errorf("Parsed identifier = %q, want %q", fm.Identifier, identifier)
			}
			if fm.FileType != fileType {
				t.Errorf("Parsed fileType = %q, want %q", fm.FileType, fileType)
			}

			// Update with new values
			newTitle := "Updated Title"
			newTags := []string{"new"}
			fm.Title = newTitle
			fm.Tags = newTags

			updated, err := Apply(content, fm)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}

			// Parse again and verify
			fm2, err := ParseFrontMatter(updated, ext)
			if err != nil {
				t.Fatalf("ParseFrontMatter() after update error = %v", err)
			}

			if fm2.Title != newTitle {
				t.Errorf("After update title = %q, want %q", fm2.Title, newTitle)
			}
			if !slices.Equal(fm2.Tags, newTags) {
				t.Errorf("After update tags = %v, want %v", fm2.Tags, newTags)
			}
		})
	}
}
