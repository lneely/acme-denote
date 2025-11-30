# Extending acme-denote

`acme-denote` is extended by building programs upon its 9P API.

```
denote/
├── ctl          (write-only)  Control commands
├── dir          (read-only)   Current denote directory path
├── index        (read-only)   List of all notes (respects active filter)
├── new          (write-only)  Create new notes
└── n/
    └── <identifier>/
        ├── backlinks    (read-only)   Notes linking to this note
        ├── ctl          (write-only)  Per-note control commands
        ├── keywords     (read-write)  Comma-separated tags
        ├── path         (read-write)  Filesystem path
        └── title        (read-write)  Note title
```

Below is a minimal example of an extension that counts notes by a given tag.

```rc
#!/usr/local/plan9/bin/rc

# Dcount - Count notes by tag
if(~ $#* 0) {
    echo 'usage: Dcount <tag>' >[1=2]
    exit 1
}

tag=$1
echo 'filter tag:'$tag | 9p write denote/ctl || {
    echo 'failed to filter' >[1=2]
    exit 1
}

count=`{9p read denote/index | wc -l}
echo 'filter' | 9p write denote/ctl
echo 'Notes with tag' $tag':' $count
exit 0
```

**Note:** The example above is written with the Plan 9 `rc` shell syntax. An extension can be written in any language or shell syntax. The key requirement is the ability to perform 9P client operations (read, write, stat, ls, etc.)


## Common Patterns

Patterns exhibited by existing extensions illustrated below (also `rc` syntax).

### Read-Only Query (e.g., Dbkp)

```rc
denotedir=`{9p read denote/dir}
# Operate on $denotedir
tar czf backup.tar.gz $denotedir
```

### Filter and Process (e.g., hypothetical Dqry)

```rc
echo 'filter tag:project' | 9p write denote/ctl
results=`{9p read denote/index}
echo 'filter' | 9p write denote/ctl  # Reset filter
# Process $results
```

Filter syntax:
- `tag:tagname` - Filter by tag
- `title:pattern` - Filter by title (use quotes for spaces: `title:"my note"`)
- `date:YYYYMMDD` - Filter by date
- `!tag:tagname` - Exclude tag
- Multiple filters space-separated: `tag:work !tag:draft`

### Create and Open (e.g., Djournal)

```rc
# Create note
identifier=`{echo '''journal entry'' journal' | 9p write denote/new}

# Get the path
notepath=`{9p read denote/n/$identifier/path}

# Open in acme or plumb
plumb $notepath
```

### Metadata Manipulation (e.g., Dmerge)

```rc
# Read source content
sourcepath=`{9p read denote/n/$sourceid/path}
content=`{cat $sourcepath}

# Update destination
destpath=`{9p read denote/n/$destid/path}
echo $content >> $destpath

# Delete source via ctl
echo 'd' | 9p write denote/n/$sourceid/ctl
```

### Switching Directories (e.g., Dsilo)

Write to `denote/ctl`:
```rc
echo 'cd /path/to/new/silo' | 9p write denote/ctl
```

### Finding Backlinks (e.g., Dmerge)

```rc
9p read denote/n/20251127T120000/backlinks
```

Returns notes that link to this identifier (same format as `index`).
