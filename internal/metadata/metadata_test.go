package metadata

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestGenerateIdentifier validates timestamp format (YYYYMMDDThhmmss)
// Maps to dt-denote-identifier-p from original tests
func TestGenerateIdentifier(t *testing.T) {
	id := GenerateIdentifier()

	// Validate format
	pattern := regexp.MustCompile(`^\d{8}T\d{6}$`)
	if !pattern.MatchString(id) {
		t.Errorf("GenerateIdentifier() = %q, want format YYYYMMDDThhmmss", id)
	}

	// Validate it's a valid timestamp
	_, err := time.Parse("20060102T150405", id)
	if err != nil {
		t.Errorf("GenerateIdentifier() = %q, not a valid timestamp: %v", id, err)
	}
}

// TestSlugifyTitle validates title slugification
// Maps to dt-denote-sluggify-title and dt-denote-sluggify from original tests
func TestSlugifyTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "basic spaces to hyphens",
			input: "Hello World",
			want:  "hello-world",
		},
		{
			name:  "underscores to hyphens",
			input: "Test_File Name",
			want:  "test-file-name",
		},
		{
			name:  "remove punctuation",
			input: "Test File!",
			want:  "test-file",
		},
		{
			name:  "mixed case",
			input: "Mixed CASE 123",
			want:  "mixed-case-123",
		},
		{
			name:  "special characters",
			input: "Special@#$Chars",
			want:  "specialchars",
		},
		{
			name:  "multiple spaces",
			input: "Multiple   Spaces",
			want:  "multiple---spaces",
		},
		{
			name:  "leading and trailing spaces",
			input: "  Trim Me  ",
			want:  "--trim-me--",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "numbers only",
			input: "12345",
			want:  "12345",
		},
		{
			name:  "hyphens preserved",
			input: "already-hyphenated",
			want:  "already-hyphenated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slugifyTitle(tt.input)
			if got != tt.want {
				t.Errorf("slugifyTitle(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestFormatKeywords validates keyword formatting for filenames
// Maps to dt-denote-sluggify-keywords from original tests
func TestFormatKeywords(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{
			name:  "multiple keywords",
			input: []string{"tag1", "tag2"},
			want:  "__tag1_tag2",
		},
		{
			name:  "single keyword",
			input: []string{"single"},
			want:  "__single",
		},
		{
			name:  "empty keywords",
			input: []string{},
			want:  "",
		},
		{
			name:  "nil keywords",
			input: nil,
			want:  "",
		},
		{
			name:  "three keywords",
			input: []string{"one", "two", "three"},
			want:  "__one_two_three",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatKeywords(tt.input)
			if got != tt.want {
				t.Errorf("formatKeywords(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestBuildFilename validates filename construction
// Maps to dt-denote-format-file-name from original tests
func TestBuildFilename(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		title      string
		keywords   []string
		ext        string
		want       string
	}{
		{
			name:       "complete filename with keywords",
			identifier: "20231225T120000",
			title:      "My Title",
			keywords:   []string{"tag1", "tag2"},
			ext:        ".md",
			want:       "20231225T120000--my-title__tag1_tag2.md",
		},
		{
			name:       "filename without keywords",
			identifier: "20231225T120000",
			title:      "My Title",
			keywords:   []string{},
			ext:        ".md",
			want:       "20231225T120000--my-title.md",
		},
		{
			name:       "filename with special chars in title",
			identifier: "20231225T120000",
			title:      "Special!@# Title",
			keywords:   []string{"work"},
			ext:        ".org",
			want:       "20231225T120000--special-title__work.org",
		},
		{
			name:       "org format",
			identifier: "20240101T000000",
			title:      "Org Note",
			keywords:   []string{"emacs"},
			ext:        ".org",
			want:       "20240101T000000--org-note__emacs.org",
		},
		{
			name:       "txt format",
			identifier: "20240101T000000",
			title:      "Plain Text",
			keywords:   []string{},
			ext:        ".txt",
			want:       "20240101T000000--plain-text.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildFilename(tt.identifier, tt.title, tt.keywords, tt.ext)
			if got != tt.want {
				t.Errorf("BuildFilename() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParseFilename validates filename parsing
// Maps to dt-denote-retrieve-filename-* tests from original
func TestParseFilename(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		wantIdentifier string
		wantTitle      string
		wantTags       []string
	}{
		{
			name:           "complete filename with tags",
			path:           "/home/notes/20231225T120000--my-title__tag1_tag2.md",
			wantIdentifier: "20231225T120000",
			wantTitle:      "my title",
			wantTags:       []string{"tag1", "tag2"},
		},
		{
			name:           "filename without tags",
			path:           "/home/notes/20231225T120000--simple-title.md",
			wantIdentifier: "20231225T120000",
			wantTitle:      "simple title",
			wantTags:       nil,
		},
		{
			name:           "filename with single tag",
			path:           "20240101T000000--note__work.org",
			wantIdentifier: "20240101T000000",
			wantTitle:      "note",
			wantTags:       []string{"work"},
		},
		{
			name:           "identifier only",
			path:           "20240101T000000.txt",
			wantIdentifier: "20240101T000000",
			wantTitle:      "",
			wantTags:       nil,
		},
		{
			name:           "multi-word title",
			path:           "20240101T000000--multi-word-title__personal_ideas.md",
			wantIdentifier: "20240101T000000",
			wantTitle:      "multi word title",
			wantTags:       []string{"personal", "ideas"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFilename(tt.path)

			if got.Identifier != tt.wantIdentifier {
				t.Errorf("ParseFilename(%q).Identifier = %q, want %q",
					tt.path, got.Identifier, tt.wantIdentifier)
			}

			if got.Title != tt.wantTitle {
				t.Errorf("ParseFilename(%q).Title = %q, want %q",
					tt.path, got.Title, tt.wantTitle)
			}

			if !SlicesEqual(got.Tags, tt.wantTags) {
				t.Errorf("ParseFilename(%q).Tags = %v, want %v",
					tt.path, got.Tags, tt.wantTags)
			}

			if got.Path != tt.path {
				t.Errorf("ParseFilename(%q).Path = %q, want %q",
					tt.path, got.Path, tt.path)
			}
		})
	}
}

// TestExtractTitleFromContent validates title extraction from file content
func TestExtractTitleFromContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		ext     string
		want    string
	}{
		{
			name: "org mode with title directive",
			content: `#+title: My Org Note
#+date: [2024-01-01]

* First Heading`,
			ext:  ".org",
			want: "My Org Note",
		},
		{
			name: "org mode fallback to heading",
			content: `#+date: [2024-01-01]

* Heading Title

Some content`,
			ext:  ".org",
			want: "Heading Title",
		},
		{
			name: "markdown yaml front matter",
			content: `---
title: Markdown Note
date: 2024-01-01
---

# First Heading`,
			ext:  ".md",
			want: "Markdown Note",
		},
		{
			name: "markdown yaml with quotes",
			content: `---
title: "Quoted Title"
date: 2024-01-01
---`,
			ext:  ".md",
			want: "Quoted Title",
		},
		{
			name: "markdown fallback to heading",
			content: `---
date: 2024-01-01
---

# Heading Title

Content here`,
			ext:  ".md",
			want: "Heading Title",
		},
		{
			name: "markdown heading without front matter",
			content: `# Simple Title

Content`,
			ext:  ".md",
			want: "Simple Title",
		},
		{
			name:    "unsupported extension",
			content: "Some content",
			ext:     ".pdf",
			want:    "",
		},
		{
			name:    "empty content",
			content: "",
			ext:     ".md",
			want:    "",
		},
		{
			name:    "no title found",
			content: "Just some text without structure",
			ext:     ".md",
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractTitleFromContent(tt.content, tt.ext)
			if got != tt.want {
				t.Errorf("ExtractTitleFromContent() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSort validates sorting functionality
func TestSort(t *testing.T) {
	notes := Results{
		{Identifier: "20240103T120000", Title: "Charlie"},
		{Identifier: "20240101T120000", Title: "Alice"},
		{Identifier: "20240102T120000", Title: "Bob"},
	}

	t.Run("sort by id ascending", func(t *testing.T) {
		testData := make(Results, len(notes))
		copy(testData, notes)

		Sort(testData, SortById, SortOrderAsc)

		if testData[0].Identifier != "20240101T120000" {
			t.Errorf("First item identifier = %q, want %q", testData[0].Identifier, "20240101T120000")
		}
		if testData[2].Identifier != "20240103T120000" {
			t.Errorf("Last item identifier = %q, want %q", testData[2].Identifier, "20240103T120000")
		}
	})

	t.Run("sort by id descending", func(t *testing.T) {
		testData := make(Results, len(notes))
		copy(testData, notes)

		Sort(testData, SortById, SortOrderDesc)

		if testData[0].Identifier != "20240103T120000" {
			t.Errorf("First item identifier = %q, want %q", testData[0].Identifier, "20240103T120000")
		}
		if testData[2].Identifier != "20240101T120000" {
			t.Errorf("Last item identifier = %q, want %q", testData[2].Identifier, "20240101T120000")
		}
	})

	t.Run("sort by title ascending", func(t *testing.T) {
		testData := make(Results, len(notes))
		copy(testData, notes)

		Sort(testData, SortByTitle, SortOrderAsc)

		if testData[0].Title != "Alice" {
			t.Errorf("First item title = %q, want %q", testData[0].Title, "Alice")
		}
		if testData[2].Title != "Charlie" {
			t.Errorf("Last item title = %q, want %q", testData[2].Title, "Charlie")
		}
	})

	t.Run("sort by title descending", func(t *testing.T) {
		testData := make(Results, len(notes))
		copy(testData, notes)

		Sort(testData, SortByTitle, SortOrderDesc)

		if testData[0].Title != "Charlie" {
			t.Errorf("First item title = %q, want %q", testData[0].Title, "Charlie")
		}
		if testData[2].Title != "Alice" {
			t.Errorf("Last item title = %q, want %q", testData[2].Title, "Alice")
		}
	})

	t.Run("sort by title case insensitive", func(t *testing.T) {
		testData := Results{
			{Identifier: "1", Title: "zebra"},
			{Identifier: "2", Title: "Apple"},
			{Identifier: "3", Title: "banana"},
		}

		Sort(testData, SortByTitle, SortOrderAsc)

		// Should be: Apple, banana, zebra (case-insensitive)
		if testData[0].Title != "Apple" {
			t.Errorf("First item title = %q, want %q", testData[0].Title, "Apple")
		}
		if testData[1].Title != "banana" {
			t.Errorf("Second item title = %q, want %q", testData[1].Title, "banana")
		}
		if testData[2].Title != "zebra" {
			t.Errorf("Third item title = %q, want %q", testData[2].Title, "zebra")
		}
	})
}

// TestResultsBytes validates serialization
func TestResultsBytes(t *testing.T) {
	tests := []struct {
		name   string
		input  Results
		want   string
	}{
		{
			name: "single note with tags",
			input: Results{
				{
					Identifier: "20240101T120000",
					Title:      "Test Note",
					Tags:       []string{"tag1", "tag2"},
				},
			},
			want: "20240101T120000 | Test Note | tag1,tag2\n",
		},
		{
			name: "note without tags",
			input: Results{
				{
					Identifier: "20240101T120000",
					Title:      "Simple Note",
					Tags:       []string{},
				},
			},
			want: "20240101T120000 | Simple Note | \n",
		},
		{
			name: "note without title",
			input: Results{
				{
					Identifier: "20240101T120000",
					Title:      "",
					Tags:       []string{"tag"},
				},
			},
			want: "20240101T120000 | (untitled) | tag\n",
		},
		{
			name: "multiple notes",
			input: Results{
				{Identifier: "20240101T120000", Title: "First", Tags: []string{"a"}},
				{Identifier: "20240102T120000", Title: "Second", Tags: []string{"b", "c"}},
			},
			want: "20240101T120000 | First | a\n20240102T120000 | Second | b,c\n",
		},
		{
			name:  "empty results",
			input: Results{},
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(tt.input.Bytes())
			if got != tt.want {
				t.Errorf("Results.Bytes() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestResultsFromString validates parsing from string format
func TestResultsFromString(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Results
		wantErr bool
	}{
		{
			name:  "single note with tags",
			input: "20240101T120000 | Test Note | tag1,tag2",
			want: Results{
				{Identifier: "20240101T120000", Title: "Test Note", Tags: []string{"tag1", "tag2"}},
			},
			wantErr: false,
		},
		{
			name:  "note without tags",
			input: "20240101T120000 | Simple Note | ",
			want: Results{
				{Identifier: "20240101T120000", Title: "Simple Note", Tags: []string{}},
			},
			wantErr: false,
		},
		{
			name:  "multiple notes",
			input: "20240101T120000 | First | a\n20240102T120000 | Second | b,c",
			want: Results{
				{Identifier: "20240101T120000", Title: "First", Tags: []string{"a"}},
				{Identifier: "20240102T120000", Title: "Second", Tags: []string{"b", "c"}},
			},
			wantErr: false,
		},
		{
			name:    "empty input",
			input:   "",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "wrong column count",
			input:   "20240101T120000 | Title",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "empty identifier",
			input:   " | Title | tags",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid tags with spaces",
			input:   "20240101T120000 | Title | tag with spaces",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "invalid tags with uppercase",
			input:   "20240101T120000 | Title | Tag1,tag2",
			want:    nil,
			wantErr: true,
		},
		{
			name:  "valid lowercase unicode tags",
			input: "20240101T120000 | Title | tag1,测试,αβγ",
			want: Results{
				{Identifier: "20240101T120000", Title: "Title", Tags: []string{"tag1", "测试", "αβγ"}},
			},
			wantErr: false,
		},
		{
			name:  "blank lines ignored",
			input: "20240101T120000 | First | a\n\n20240102T120000 | Second | b",
			want: Results{
				{Identifier: "20240101T120000", Title: "First", Tags: []string{"a"}},
				{Identifier: "20240102T120000", Title: "Second", Tags: []string{"b"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r Results
			got, err := r.FromString(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("Results.FromString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if tt.wantErr {
				return
			}

			if len(got) != len(tt.want) {
				t.Fatalf("Results.FromString() length = %d, want %d", len(got), len(tt.want))
			}

			for i := range got {
				if got[i].Identifier != tt.want[i].Identifier {
					t.Errorf("Result[%d].Identifier = %q, want %q", i, got[i].Identifier, tt.want[i].Identifier)
				}
				if got[i].Title != tt.want[i].Title {
					t.Errorf("Result[%d].Title = %q, want %q", i, got[i].Title, tt.want[i].Title)
				}
				if !SlicesEqual(got[i].Tags, tt.want[i].Tags) {
					t.Errorf("Result[%d].Tags = %v, want %v", i, got[i].Tags, tt.want[i].Tags)
				}
			}
		})
	}
}

// TestSlicesEqual validates slice comparison
func TestSlicesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		b    []string
		want bool
	}{
		{
			name: "equal slices",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "b", "c"},
			want: true,
		},
		{
			name: "different length",
			a:    []string{"a", "b"},
			b:    []string{"a", "b", "c"},
			want: false,
		},
		{
			name: "different content",
			a:    []string{"a", "b", "c"},
			b:    []string{"a", "x", "c"},
			want: false,
		},
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "both empty",
			a:    []string{},
			b:    []string{},
			want: true,
		},
		{
			// slices.Equal treats nil and empty slices as equal (both have len 0)
			name: "nil vs empty",
			a:    nil,
			b:    []string{},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SlicesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("SlicesEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// TestGenerateNote validates note generation (integration test)
func TestGenerateNote(t *testing.T) {
	dir := "/tmp/notes"
	title := "Test Note"
	keywords := []string{"test", "example"}
	fileType := "md-yaml"

	path, content := GenerateNote(dir, title, keywords, fileType)

	// Validate path format
	if !strings.HasPrefix(path, dir+"/") {
		t.Errorf("GenerateNote() path = %q, should start with %q", path, dir+"/")
	}

	// Validate filename contains identifier and title
	if !strings.Contains(path, "--test-note__test_example.md") {
		t.Errorf("GenerateNote() path = %q, should contain proper filename", path)
	}

	// Validate content has front matter
	if !strings.Contains(content, "---") {
		t.Errorf("GenerateNote() content should contain YAML front matter")
	}
	if !strings.Contains(content, "title:") {
		t.Errorf("GenerateNote() content should contain title field")
	}
	if !strings.Contains(content, "[test, example]") {
		t.Errorf("GenerateNote() content should contain formatted tags")
	}
}
