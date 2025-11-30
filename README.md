# acme-denote

> (Denote) is based on the idea that notes should follow a predictable and descriptive file-naming scheme. The file name must offer a clear indication of what the note is about, without reference to any other metadata. Denote basically streamlines the creation of such files while providing facilities to link between them.

The goal of this program is to provide an implementation of prot's [denote](https://github.com/protesilaos/denote) for Acme.

## Use at Own Risk

While I use this program daily and do my best to fix bugs quickly, it is relatively new and I don't promise that it's bug-free. If you use this program, use good sense and keep regular backups of your denote directory. ([Dbkp](#dbkp) provided for convenience.)

## Does it work on Plan 9?

Short answer: Maybe? I don't know. I use [plan9port](https://github.com/9fans/plan9port). If there are any regular Plan 9 users out there, I would be interested to know if it does, though. :)

## Quick Start

This README assumes you already have [plan9port](https://github.com/9fans/plan9port) installed.

```sh
mk install
```

This installs `Denote` and all its extensions to $HOME/bin.

### Setup

Start the Denote program by middle-clicking `Denote` anywhere in acme. This will open the `/Denote/` window.

**Configuration:**

Notes are stored in `~/doc` by default. To change the default denote directory, edit `pkg/config/config.go`:

```go
var DefaultDenoteDir = os.Getenv("HOME") + "/doc"
```

You can also switch between different directories at runtime using the `Dsilo` command (see [Dsilo](#dsilo)).

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
/Denote/ Del Snarf | Look New Put Remove Get
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
/Denote/ Del Snarf | Look New Put Remove Get
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

### Get

Reload all notes from disk, discarding any uncommitted changes in the 9P metadata. Middle-click `Get` to do this. This is useful when notes are modified outside of Acme or when you want to discard metadata changes.

### Encrypted Notes

You can integrate encrypted notes with [acme-crypt](https://github.com/lneely/acme-crypt) `CryptGet` and `CryptPut` commands. This allows you to work with encrypted files (e.g., GPG-encrypted) directly from acme-denote.

Create a note with `New` as usual:

```
'secret note' private
```

This opens a window with a path like `/home/user/doc/20251128T120000--secret-note__private.md`. Add the appropriate encryption extension to the window path (e.g., `.gpg` for GPG encryption):

```
/home/user/doc/20251128T120000--secret-note__private.md.gpg
```

Write your note content, add `CryptPut` to the window tag, and middle-click it to save the encrypted file to disk. Then middle-click `Get` in the `/Denote/` window to refresh the 9P index from disk.

**Warning:** If you accidentally click `Put` after `CryptPut`, the file will be overwritten with unencrypted content. If this happens, use `CryptPut` again to re-encrypt the file.

### Drn

Update note metadata (title, tags, signature). Drn has two modes of operation:

**Window Mode** - Rename from an active note window:

From any open note window, highlight and execute:

```
Drn 'new title' tag1,tag2
Drn ==newsig 'new title' tag1,tag2
```

- Works on the window's buffer content
- **Regular notes** (.md, .org, .txt): Updates frontmatter in buffer
- The window must be `Put` after Drn to persist changes to disk

**Interactive Mode** - Rename by identifier:

From the `/Denote/` window or anywhere in Acme, highlight and execute:

```
Drn 20251112T221141 'new title' tag1,tag2
Drn 20251112T221141 ==newsig 'new title' tag1,tag2
```

- Operates directly on the file
- **Regular notes** (.md, .org, .txt): Updates frontmatter in file + renames file
- **Binary files** (PDFs, images): Just renames file
- Changes are applied immediately to disk

## File Format

By default notes are markdown files with YAML frontmatter:
- Identifier: `YYYYMMDDTHHMMSS`
- Filename: `<id>--<title>__<tags>.md`
- Search works on title and content

You may change this by changing `ftype` in main.go.

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

### Content Search (grep)

You can easily grep the current denote directory to search for content. Use the following pattern, example with `ripgrep`:

```
rg -Hn <search-expr> `{9p read denote/dir}
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

Supported units: `d` (days), `h` (hours), `m` (minutes). Hours and minutes is not useful except in an sub-daily intervals (e.g., hourly, not yet supported).

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

### Dbkp

Backup and restore your denote directory. Backups are timestamped tar.gz archives stored in `$HOME/Dbkp/` by default.

**Configuration:**

Customize the backup directory by editing the variable in the `Dbkp` script:

```rc
# Configuration
backupdir=$HOME/Dbkp
```

**Create Backup:**

Create a timestamped backup of your denote directory:

```
Dbkp
```

This creates `$backupdir/YYYYMMddTHHmmss-<denote-dir-name>-dbkp.tar.gz`.

**List Backups:**

List all available backups:

```
Dbkp l
```

Output shows timestamps, one per line:

```
20251127T140558
20251127T142011
20251127T143725
```

**Restore Backup:**

Restore a backup to `$denoteDir/restore/YYYYMMddTHHmmss/`:

```
Dbkp 20251127T140558
```

The backup is extracted to a timestamped subdirectory within the restore directory, making it easy to identify when the backup was created.

### Dsilo

Concept from prot's [denote-silo](https://github.com/protesilaos/denote-silo). Switch between different denote directories (silos) at runtime without restarting the program.

**Usage:**

Show the current silo:
```
Dsilo
```

Switch to a different silo:
```
Dsilo /path/to/another/directory
```

**What happens when you switch silos:**

- The 9P filesystem immediately points to the new directory
- All existing notes are cleared from memory
- New notes are loaded from the new directory
- All subsequent operations (create, update, delete, search) use the new directory
- The Acme log watcher tracks files in the new directory

This is useful for maintaining separate collections of notes (e.g., personal notes, work notes, project notes) and switching between them seamlessly.

## Possible Future Work

### Templates

Not a core feature of acme-denote, but could be implemented as an extension (e.g., `Dtmpl <path>` or `<tmpl> | Dtmpl`). Possible approach:

- New Go binary, `cmd/Dtmpl/`
- Use `text/template`
- Read template and active window body
- Build template variable to value mapping
- Define common variables, e.g., `{{date}}`, `{{author}}`
- Render template and write to the active window body (`echo <content> | 9p write acme/$winid/body`)

### Support Query "Links"

Support `denote-query:<query>` style "links" with plumbing (e.g., `denote-query:project`). Possible approach:

- New `rc` script, e.g., `Dqry`
  - Writes the query to `denote/filter` to mutate the index
  - Reads the index into a search results window
  - Writes empty query to `denote/filter` to reset the index
- Setup a new plumbing rule `denote-query:<expr>` to run `Dqry`
