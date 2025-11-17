# Plumbing Integration for Denote

This document describes how to integrate the Denote command with the Plan 9 plumber to enable right-click opening of denote notes.

## Overview

The plumbing rules allow you to right-click on `denote:<identifier>` text in acme and have it automatically open the corresponding note.

## Setup

Add these rules to your `$HOME/lib/plumbing` file:

```
# declarations of ports without rules
plumb to denote

# Denote identifiers - open note by identifier
type is text
data matches 'denote:([0-9]+T[0-9]+)'
plumb to denote
plumb start Denote $1
```

### Rule Placement

- The port declaration (`plumb to denote`) should go with other port declarations near the top
- The pattern matching rule should be placed **before** generic file matching rules to ensure it fires first

## Reload Rules

After editing `$HOME/lib/plumbing`, reload the rules:

```rc
cat $HOME/lib/plumbing | 9p write plumb/rules
```

## Usage

Once configured, you can right-click (button 3) on any text matching the pattern:

```
denote:20251103T165653
```

This will execute `Denote open 20251103T165653` and open the note in acme.

## Pattern Details

The regex pattern `denote:([0-9]+T[0-9]+)` matches:
- The literal text `denote:`
- One or more digits
- The letter `T`
- One or more digits

This matches the standard Denote identifier format: `YYYYMMddTHHmmss`

## Troubleshooting

If right-clicking doesn't work:

1. Verify rules are loaded: `9p read plumb/rules | grep denote`
2. Check the command exists: `which Denote`
3. Test manually: `plumb "denote:20251103T165653"`
4. Ensure the pattern uses `+` not `{n}` quantifiers (Plan 9 regex limitation)
