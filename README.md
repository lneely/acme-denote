# acme-denote

> (Denote) is based on the idea that notes should follow a predictable and descriptive file-naming scheme. The file name must offer a clear indication of what the note is about, without reference to any other metadata. Denote basically streamlines the creation of such files while providing facilities to link between them.

The goal of this program is to provide an implementation of prot's [denote](https://github.com/protesilaos/denote) for Acme.

## Quick Start

This README assumes you already have [plan9port](https://github.com/9fans/plan9port) installed.

```sh
mk install
```

This installs `Denote` and all its extensions to $HOME/bin.

### Setup

Start the Denote program by middle-clicking `Denote` anywhere in acme. This will open the `/Denote/` window. Notes are stored in `~/doc` by default, simply edit this in `main.go` to change your denote base directory.

## Usage

### Create a note
In Acme, you can highlight something like:
```
'new note title' tag1,tag2,tagN
```

Pass it as input to the `New` tag with the `2-1` chord. This will create a new Acme window with Denote frontmatter and an appropriate file path. **Important:** the actual file is not created until the `Put` command is executed. (Tags are optional but recommended).

You can organize notes into subdirectories using the `/:` separator in the title:
```
'journal/:today's entry' journal
'projects/:new project' project,idea
'meetings/:standup notes' work,meeting
```

The text before `/:` becomes the subdirectory path (e.g., `journal/`, `projects/`, `meetings/`), and the text after becomes the note title. Subdirectories are created automatically if they don't exist. This is useful for organizing related notes together.

### Open a note

First, be sure to set up the [plumbing rules](./PLUMBING.md). Then, simply right-click on any identifier in the `/Denote/` window.

### Edit a note

Edit a note just like any other text file. Use `Put` to save. The Denote metadata will be refreshed automatically.

### Delete a note

To delete a note, highlight its identifier in the `/Denote/` window:
```
20251112T221141
```

Pass it as input to the `Remove` tag with the `2-1` chord. This will delete the note file from the filesystem and remove it from the index.

### Search notes
Type some search pattern. Examples:

```
tag:tag1 tag:tag2
title:'my title'
'my title'
tag1
20251120
```

Highlight this and pass it as input to the `Look` command with the `2-1` chord. This will filter the list of entries to those that match the search query. Executing `Look` without arguments resets the search filter. You may also right-click in the Denote window on titles or tags to jump between matches.

### Metadata Editing

Quality of life feature: you may edit metadata directly in the Denote window.

```
-------------------------------------------------------
/Denote/ Del Snarf | Look New Put Remove Sync
-------------------------------------------------------
20251112T221141 | my dummy file 7 | dummy
20251112T221140 | my dummy file 6 | dummy
20251112T221139 | my dummy file 5 | dummy
20251112T221138 | my dummy file 4 | dummy
20251112T221137 | my dummy file 3 | dummy
20251112T221136 | my dummy file 2 | dummy
20251112T221135 | my dummy file 1 | dummy
```

```
-------------------------------------------------------
/Denote/ Del Snarf | Look New Put Remove Sync
-------------------------------------------------------
20251112T221141 | you can edit | dummy
20251112T221140 | the titles | dummy
20251112T221139 | my dummy file 5 | or,the
20251112T221138 | my dummy file 4 | tags
20251112T221137 | or both at the same | time
20251112T221136 | my dummy file 2 | dummy
20251112T221135 | my dummy file 1 | dummy
```

Middle-click `Put` to write all metadata changes. This will rename files and, when possible, update front matter.

### Sync

Sometimes it's necessary to update the Denote metadata from the filesystem. Middle-click `Sync` to do this.

## File Format

By default notes are markdown files with YAML frontmatter:
- Identifier: `YYYYMMDDTHHMMSS`
- Filename: `<id>--<title>__<tags>.md`
- Search works on title and content

You may change this by changing `ftype` in main.go.

## Encryption Support

This program supports encrypted notes by integrating with [acme-crypt](https://github.com/lneely/acme-crypt). Set up the [plumbing rules](./PLUMBING.md) to read encrypted notes, and use `CryptPut` to write them.

## Signature Support

Denote supports an optional signature component in filenames: `ID==SIGNATURE--TITLE__TAGS.ext`. Signatures are useful for sequential numbering, context markers, or priorities.

**Plumbing Configuration:**

To plumb files with signatures, you must add `=` to the file pattern character classes in your plumbing rules. Update these three patterns:

**Original patterns:**
```
data matches '([.a-zA-Z�-\uffff0-9_/\-@]*[a-zA-Z�-\uffff0-9_/\-])':$twocolonaddr,$twocolonaddr
data matches '([.a-zA-Z�-\uffff0-9_/\-@]*[a-zA-Z�-\uffff0-9_/\-])':$twocolonaddr
data matches '([.a-zA-Z�-\uffff0-9_/\-@]*[a-zA-Z�-\uffff0-9_/\-])('$addr')?'
```

**Modified patterns (add `=` to character classes):**
```
data matches '([.a-zA-Z�-\uffff0-9_/\-@=]*[a-zA-Z�-\uffff0-9_/\-=])':$twocolonaddr,$twocolonaddr
data matches '([.a-zA-Z�-\uffff0-9_/\-@=]*[a-zA-Z�-\uffff0-9_/\-=])':$twocolonaddr
data matches '([.a-zA-Z�-\uffff0-9_/\-@=]*[a-zA-Z�-\uffff0-9_/\-=])('$addr')?'
```

## Extensions

Some extensions have also been ported. While these extensions, like the main program, try to stay as true as possible to the original program and be as feature-complete as possible, they are intentionally *not* exact replicas.

### Djournal

Create or open journal entries with date-based titles. Supports daily, weekly, monthly, or yearly journals. Concept from prot's [denote-journal](https://github.com/protesilaos/denote-journal).

**Configuration:**

Customize journal behavior by editing variables in the `Djournal` script:

```rc
# Journal interval: daily, weekly, monthly, yearly
interval=daily

# Title format: full, date, day, year
# Examples:
#   full: "wednesday 12 november 2025 22:11"
#   date: "wednesday 12 november 2025"
#   day:  "wednesday"
#   year: "2025"
titleformat=full

# Signature (optional, added to filename as ==signature)
signature=''
```

**Basic Usage:**

Open today's journal entry (creates if it doesn't exist):

```
Djournal
```

Journal entries are stored in `journal/` subdirectory with the `journal` tag.

**Time Navigation:**

Navigate to past or future dates using time offsets:

```
Djournal +2d    # 2 days from now
Djournal -1d    # yesterday
Djournal -3h    # 3 hours ago
Djournal +30m   # 30 minutes from now
```

Supported units: `d` (days), `h` (hours), `m` (minutes)

### Dmerge

Merge notes together or move regions of text between notes while maintaining referential integrity. Concept from prot's [denote-merge](https://github.com/protesilaos/denote-merge).


**Configuration:**

Customize annotations by editing variables in the `Dmerge` script:

```rc
# Annotation for merged files (set to '' to disable)
annotation='MERGED FILE:'

# Annotation for merged regions (set to '' to disable)
regionannotation='MERGED REGION:'
```

**File Merge**

Merge an entire note into another note:

```
Dmerge <source-id> <dest-id>
```

This will:
- Strip frontmatter from source note
- Append source content to destination note with annotation
- Update all backlinks pointing to source to point to destination
- Delete the source note file

Example:
```
Dmerge 20251125T120000 20251125T130000
```

**Region Merge**

Move a selected region of text from one note to another. Select text in an Acme window, then:

```
Dmerge <dest-id>
```

This will:
- Move the selected text to the destination note
- Replace the selection with a link to the destination: `denote:<dest-id>`
- Leave the source note open (dirty) for you to `Put`

**With Formatting:**

You can wrap the merged region in various formats:

```
Dmerge <dest-id> <format>
```

Available formats:
- `plain` - No formatting (default)
- `plain-indented` - Add 4-space indentation
- `org-src` - Org source block (`#+begin_src` / `#+end_src`)
- `org-quote` - Org quote block (`#+begin_quote` / `#+end_quote`)
- `org-example` - Org example block (`#+begin_example` / `#+end_example`)
- `md-quote` - Markdown blockquote (prefix with `> `)
- `md-fence` - Markdown fenced code block (` ``` `)

Examples:
```
Dmerge 20251125T130000 org-quote
Dmerge 20251125T130000 md-fence
```
