---
type: Concept
title: Architecture
description: Single-binary design, stdlib-only constraints, data layout under ~/.notie, and markdown file formats used by the notie CLI.
tags: [architecture, data-model, markdown, go]
---

# Architecture

`notie` is deliberately small: a single Go binary with no third-party dependencies, storing all user data as plain markdown files in a directory tree.

## Single-binary, stdlib-only design

- The module is `module notie` requiring Go 1.25+ ([`go.mod`](go.mod)).
- All source compiles with the Go standard library only.
- Terminal raw mode is implemented with direct Darwin `ioctl` calls in [`term.go`](term.go) instead of importing `golang.org/x/term`.
- Optional external programs (`ffmpeg`, `hear`, `whisper-cli`, `claude`) are discovered at runtime via `exec.LookPath`; their absence degrades gracefully or emits a clear error.

This constraint keeps deployment trivial: `go build -o notie .` produces a standalone executable.

## Data layout

Everything lives under `${NOTIE_DIR:-$HOME/.notie}`:

```text
~/.notie/
├── 2026-07-22/
│   ├── journal.md      # timestamped journal entries for the day
│   └── shell.md        # shell audit trail for the day
├── task.md             # tasks with IDs and priorities
├── important.md        # dated important notes
├── remember.md         # dated reminders
├── datecache.md        # one-line-per-day summaries built by `notie cache`
└── .task_seq           # task id counter
```

Per-day directories are named `YYYY-MM-DD`. Files are markdown mostly for human readability; the program parses them with regular expressions rather than a markdown parser.

## File formats

| File | Location | Line format |
|------|----------|-------------|
| Journal | `<dir>/<YYYY-MM-DD>/journal.md` | `- HH:MM — text` |
| Shell audit | `<dir>/<YYYY-MM-DD>/shell.md` | `- HH:MM (location) $ command` |
| Tasks | `<dir>/task.md` | `- [ ] #<id> !<0\|1\|2> desc (added YYYY-MM-DD)` · done tasks append ` (done YYYY-MM-DD)` |
| Important | `<dir>/important.md` | `- YYYY-MM-DD HH:MM — text` |
| Remember | `<dir>/remember.md` | `- YYYY-MM-DD HH:MM — text` |
| Date cache | `<dir>/datecache.md` | `- YYYY-MM-DD: one-line summary` |

The em dash in journal/note lines is `—` (U+2014). Task IDs are stable integers kept in `.task_seq`; [`nextID`](main.go) self-heals against the maximum ID already present in `task.md`.

## Core abstractions

- **Commands** in [`main.go`](main.go) parse arguments and dispatch to implementation functions such as `cmdAdd`, `cmdTask`, `cmdCache`.
- **Day browser** in [`tui_browser.go`](tui_browser.go) is a generic `browserCfg`/`browserTUI` reused for journal, shell, and important views.
- **List TUI** in [`tui_notes.go`](tui_notes.go) powers remember/important list views.
- **Task TUI** in [`tui_task.go`](tui_task.go) is specialized for priority grouping and task state changes.
- **Voice pipeline** in [`record.go`](record.go) records WAV audio, transcribes it, then routes the text to the same writers used by the CLI commands.

## Chronology and ordering

Journal entries are inserted in chronological position when added to a past date, not blindly appended. This is handled by `addJournal` in [`main.go`](main.go), which scans existing `- HH:MM —` lines and splices the new line at the correct timestamp. Today is always appended. Task lines are append-only; the TUI re-sorts them visually by priority.

## Statelessness

There is no background process, lock file, or database. Concurrent writes from multiple processes could race on the same markdown file, but the design assumes a single user on a single machine writing from one terminal at a time.
