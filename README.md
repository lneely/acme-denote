# acme-denote

> (Denote) is based on the idea that notes should follow a predictable and descriptive file-naming scheme. The file name must offer a clear indication of what the note is about, without reference to any other metadata. Denote basically streamlines the creation of such files while providing facilities to link between them.

The goal of this program is to provide an implementation of prot's [denote](https://github.com/protesilaos/denote) for Acme.

## Quick Start

### Install
```sh
mk install
```

### Setup
Start the Denote program by middle-clicking `Denote` anywhere in acme. This will open the `/Denote/` window. Notes are stored in `~/doc` by default, simply edit this in `main.go` to change your denote base directory.

## Usage

### Create a note
In Acme, you can highlight something like:
```
'new note title' tag1,tag2,tagN
```

Pass it as input to the `New` tag with the `2-1` chord. This will create a new Acme window with Denote frontmatter and an appropriate file path. **Important:** the actual file is not created until the `Put` command is executed. (Tags are optional but recommended).

### Open a note

First, be sure to set up the [plumbing rules](./PLUMBING.md). Then, simply right-click on any identifier in the `/Denote/` window.

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

### Journal

Create or open today's journal:
```
Journal
```

Journal for other times:
```
Journal +1d        # tomorrow
Journal -7d        # 7 days ago
Journal +3h        # 3 hours from now
Journal -30m       # 30 minutes ago
```

Supports: `d` (days), `h` (hours), `m` (minutes). You can use this to create multiple entries on the same day. To find an existing journal entry, use the search function in `Denote` described above. You can also pass the args in with the `2-1` chord.

### Metadata Editing
You may edit metadata directly in the Denote window.

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

## Sync

Sometimes it's necessary to update the Denote metadata from the filesystem. Middle-click `Sync` to do this.

## File Format

By default notes are markdown files with YAML frontmatter:
- Identifier: `YYYYMMDDTHHMMSS`
- Filename: `<id>--<title>__<tags>.md`
- Search works on title and content

You may change this by changing `ftype` in main.go.

## Encryption Support

This program supports encrypted notes by integrating with [acme-crypt](https://github.com/lneely/acme-crypt). Set up the [plumbing rules](./PLUMBING.md) to read encrypted notes, and use `CryptPut` to write them.
