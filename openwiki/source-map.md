---
type: Reference
title: Source map
description: File-by-file guide to the notie codebase — what each source file owns, key functions, and where to start when changing behavior.
tags: [source-map, reference, code-organization]
---

# Source map

`notie` is a small Go program split by concern. There are no packages; everything lives in `package main`.

## File inventory

### [`main.go`](main.go)

The CLI entrypoint and all core command implementations. Roughly 700 lines.

Key responsibilities:

- Argument dispatch in [`main()`](main.go)
- Data directory resolution (`NOTIE_DIR`) via [`notieDir()`](main.go)
- Journal writing and retroactive insertion via [`addJournal()`](main.go)
- Task CRUD, IDs, and priority grouping via [`cmdTask()`](main.go), [`taskEdit()`](main.go), [`nextID()`](main.go)
- Date-cache generation via [`cmdCache()`](main.go)
- Shell audit logging via [`cmdLog()`](main.go)
- Plain-text `show` commands via [`cmdShow()`](main.go) and [`cmdShowShell()`](main.go)

Start here for: new CLI commands, changes to file formats, date logic, or task semantics.

### [`record.go`](record.go)

Voice recording and transcription. Isolated so the core binary can be read without knowing about ffmpeg or Whisper.

Key responsibilities:

- [`recordWav()`](record.go) — ffmpeg + `avfoundation` microphone capture
- [`transcribe()`](record.go) — `hear` preferred, `whisper-cli` fallback
- [`whisperModel()`](record.go) — model selection and `NOTIE_WHISPER_MODEL`
- [`cmdRecord()`](record.go) — user confirmation/correction and routing to the right store

Start here for: new voice targets, alternative transcription backends, macOS recording changes.

### [`term.go`](term.go)

Raw terminal mode implemented with direct Darwin `ioctl` syscalls. Avoids any external terminal library.

Key responsibilities:

- [`enterRaw()`](term.go) / [`restoreTerm()`](term.go)
- [`termSize()`](term.go) via `TIOCGWINSZ`
- [`isTTY()`](term.go)
- [`readKey()`](term.go) — maps escape sequences to vim-style keys

Start here for: TTY detection fixes, new key bindings, porting to Linux/other terminals.

### [`theme.go`](theme.go)

ANSI color constants and shared text-rendering utilities used by all three TUIs.

Key responsibilities:

- Color/icon constants (e.g. `cAccent`, `iTaskDone`)
- Unicode-aware truncation and padding (`runeLen`, `truncRunes`, `padTo`)
- Word wrapping (`wrapRunes`)
- Case-insensitive highlight (`highlight`)
- Title-bar and cursor-row rendering helpers (`titleBar`, `cursorRow`)

Start here for: visual style changes, new color schemes, rendering bugs with wide characters.

### [`tui_browser.go`](tui_browser.go)

Day-browser TUI. Backs `notie journal`, `notie shell`, and `notie important`.

Key responsibilities:

- [`browserCfg`](tui_browser.go) adapter struct
- [`journalBrowser()`](tui_browser.go), [`shellBrowser()`](tui_browser.go), [`importantBrowser()`](tui_browser.go) configurations
- Sidebar/content split, scrolling, search (`/`, `:ff`, `:fg`)
- Adding entries to a selected day

Start here for: new browseable data sources, search behavior, day-browser UX.

### [`tui_notes.go`](tui_notes.go)

Flat-list TUI for `remember.md` and `important.md` (currently only `notie remember` uses it).

Key responsibilities:

- [`notesCfg`](tui_notes.go) and [`runNotesTUI()`](tui_notes.go)
- Line-based note display with date/time metadata
- Add, delete, search

Start here for: new flat-list note types or converting `notie important` to the list view.

### [`tui_task.go`](tui_task.go)

Interactive task list. Backs `notie task` (no arguments, TTY).

Key responsibilities:

- [`taskTUI`](tui_task.go) struct and [`runTaskTUI()`](tui_task.go)
- Priority grouping, sorting, and done-task filtering
- Toggle done, set priority, add, delete, search

Start here for: task UI changes, new task operations, task sorting/grouping tweaks.

### [`setup.sh`](setup.sh)

Bash build/install/optional-integration script. See [`operations.md`](operations.md) for behavior.

### [`.claude/skills/notie-review/SKILL.md`](.claude/skills/notie-review/SKILL.md)

Claude Code skill for periodic reviews. See [`integrations.md`](integrations.md) for details.

### Supporting files

| File | Purpose |
|---|---|
| [`go.mod`](go.mod) | Module definition; requires Go 1.25 |
| [`README.md`](README.md) | User-facing command reference and setup guide |
| [`.gitignore`](.gitignore) | Ignores the built binary, editor files, coverage output |
| [`.github/workflows/openwiki-update.yml`](.github/workflows/openwiki-update.yml) | Scheduled OpenWiki refresh via GitHub Actions |

## Common change paths

| Want to change... | Start in |
|---|---|
| Add a new command | [`main.go`](main.go) `main()` switch and a new `cmd*` function |
| Change task line format | [`main.go`](main.go) `cmdTask`, `cmdDid`, `taskEdit`; also update [`tui_task.go`](tui_task.go) `taskFullRe` and the Claude skill |
| Change journal line format | [`main.go`](main.go) `addJournal`; update `entryRe` and `browserEntryRe` |
| Add a new voice target | [`record.go`](record.go) `cmdRecord` switch |
| Add a new TUI | reuse [`term.go`](term.go), [`theme.go`](theme.go); model on [`tui_task.go`](tui_task.go) or [`tui_browser.go`](tui_browser.go) |
| Port to Linux | [`term.go`](term.go) ioctl constants and possibly [`record.go`](record.go) ffmpeg input |
| Change install behavior | [`setup.sh`](setup.sh) |

## Test gaps

As of this writing there are no `_test.go` files. The highest-value tests would be:

1. `addJournal` retroactive insertion ordering.
2. `cmdCache` idempotency and task-done-date inclusion.
3. `taskEdit` / `nextID` consistency.
4. TUI-independent regexes: `entryRe`, `taskFullRe`, `dcLineRe`, `noteLineRe`.
5. `cmdLog` working-directory abbreviation and empty-input handling.
