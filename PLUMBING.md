# Plumbing Integration for Denote

This document describes how to integrate the Denote command with the Plan 9 plumber to enable right-click opening of denote notes.

## Overview

The plumbing rules allow you to right-click on `denote:<identifier>` text in acme, or on `<identifier>` in the `/Denote/` window, and have it automatically open the corresponding note.

## Denote Rule

Add these rules to your `$HOME/lib/plumbing` file:

```
# declarations of ports without rules
plumb to denote

# Denote identifiers - open note by identifier
type is text
data matches 'denote:([0-9]+T[0-9]+)'
plumb to denote
plumb start Denote $0
```

## Usage

Once configured, you can right-click (button 3) on any text matching the pattern:

```
denote:20251103T165653
```

Or simply right click an identifier in the `Denote` window. You may use the `denote:` pattern to cross-link notes.


