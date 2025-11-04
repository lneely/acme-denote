# acme-denote

Denote note management for the acme editor. Implements the [denote](https://protesilaos.com/emacs/denote) file naming convention with search, filtering, and journaling capabilities.

## File Naming Convention

Notes follow the denote naming pattern:
```
IDENTIFIER--TITLE__KEYWORDS.EXT
IDENTIFIER==SIGNATURE--TITLE__KEYWORDS.EXT  (with optional signature)
```

Where:
- `IDENTIFIER`: Timestamp in format `YYYYMMDDTHHmmss`
- `SIGNATURE`: Optional signature field
- `TITLE`: Hyphen-separated title (spaces become hyphens)
- `KEYWORDS`: Underscore-separated tags
- `EXT`: File extension (`.org`, `.md`, `.txt`)

Example: `20251103T183000--meeting-notes__work_project.md`

## Commands

### Denote

Search and manage denote notes.

**Usage in acme:**
```
Denote                              # List all notes (sorted by ID, newest first)
Denote tag:meeting                  # Notes tagged 'meeting'
Denote date:/2025.*/                # Notes from 2025
Denote date:/202510.*/ !tag:journal # October 2025, not journal
Denote new 'My Note' tag1 tag2      # Create new note
Denote sort:title                   # List all notes sorted alphabetically
Denote tag:journal sort:title       # Journal entries sorted by title
```

**Filter syntax:**
- `date:/regex/` - Match date/identifier
- `title:/regex/` - Match title (from filename or file content)
- `tag:/regex/` - Match tags/keywords
- `/regex/` - Match any field
- `!filter` - Negate filter
- `plain-text` - Exact match (no regex)

**Sort options:**
- `sort:id` - Sort by identifier/date (default, newest first)
- `sort:date` - Sort by date (same as ID)
- `sort:title` - Sort alphabetically by title

**Creating notes:**
```
Denote new 'Meeting Notes' work project
Denote new -f org 'Todo List' tasks
Denote new -f txt 'Journal Entry' journal
```

Supported file types: `org`, `md-yaml`, `md-toml`, `txt`

### Journal

Daily journaling with automatic date-based titles.

**Usage in acme:**
```
Journal              # Find or create today's entry
Journal add          # Create additional entry for today
Journal add 20251025 # Create entry for specific date (YYYYMMDD)
```

Journal entries are automatically:
- Titled with format: "Monday 3 November 2025 16:56"
- Tagged with `journal`
- Stored in `$JOURNAL_DIR` subdirectory (default: `journal/`)

## Acme Integration

Both commands output to a `+Denote` window with clickable file paths. Results show:
```
20251103T183000--meeting-notes__work_project.md : Meeting Notes (work, project) [/home/user/doc/20251103T183000--meeting-notes__work_project.md]

20251103T090000--monday-3-november-2025-09-00__journal.md : Daily Standup (journal) [/home/user/doc/journal/20251103T090000--monday-3-november-2025-09-00__journal.md]
```

**Workflows:**

*Simple execution:*
1. Middle-click `Denote tag:work` in tag bar or scratch window
2. Results appear in `+Denote` window
3. Right-click file paths to open notes

*Mouse chording (argument passing):*
1. Type or select filter arguments: `tag:mytag date:20251010`
2. Middle+left chord on `Denote` command
3. Selected text becomes arguments to Denote

*Creating notes:*
1. Middle-click `Denote new 'Title' tags` in scratch window
2. Or chord: select `'My Title' work urgent`, middle+left on `Denote new`
2. Or chord: select `new 'My Title' work urgent`, middle+left on `Denote`

*Quick journal access:*
1. Middle-click `Journal` anywhere
2. Shows today's entry in the search result, creating it if necessary

## Configuration

**Environment variables:**
- `DENOTE_DIR` - Base directory for notes (default: `~/doc`)
- `JOURNAL_DIR` - Subdirectory for journal entries (default: `journal`)

**Example:**
```bash
export DENOTE_DIR="$HOME/notes"
export JOURNAL_DIR="daily"
```

## Installation

```bash
mk install
```

Installs `Denote` and `Journal` to `~/bin/`.

## Front Matter

Notes include front matter based on file type:

**Markdown (YAML):**
```yaml
---
title:      "Meeting Notes"
date:       2025-11-03 Mon 18:30
tags:       [work, project]
identifier: "20251103T183000"
---
```

**Org-mode:**
```org
#+title:      Meeting Notes
#+date:       [2025-11-03 Mon 18:30]
#+filetags:   :work:project:
#+identifier: 20251103T183000
```

**Text:**
```
title:      Meeting Notes
date:       2025-11-03 Mon 18:30
tags:       work project
identifier: 20251103T183000
```

## Title Extraction

The `title:` field in search results is extracted from:
1. Front matter (`#+title:` for org, `title:` for markdown)
2. First heading (`* Heading` for org, `# Heading` for markdown)
3. Filename title component (fallback)

This allows searching by actual document titles, not just filenames.
