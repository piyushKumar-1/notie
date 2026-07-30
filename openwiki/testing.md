---
type: Testing
title: notie testing
description: How to test notie changes today — manual checks, regression risks, and guidance for adding tests to the dependency-free Go codebase.
tags: [testing, go, manual-testing, regression]
---

# notie testing

notie ships without automated tests today. Because the codebase is small and the surface is primarily file IO and terminal rendering, correctness has been verified manually during development. This page documents the recommended manual checks and where tests would add the most value.

## Manual smoke test

After building, run a short end-to-end exercise:

```sh
# clean test store
export NOTIE_DIR=/tmp/notie-test
rm -rf "$NOTIE_DIR"

notie add "first journal entry"
notie add 2026-07-22 10:00 "retroactive entry"
notie add 2026-07-22 09:00 "earlier retroactive entry"   # should sort before the 10:00 line
notie did 2026-07-22 "closed a ticket"
notie task 1 "test task"
notie task 0 "urgent task"
notie task done $(notie task list | grep 'test task' | sed 's/.*#\([0-9]*\).*/\1/')
notie cache
notie show datecache
cat "$NOTIE_DIR"/task.md
cat "$NOTIE_DIR"/2026-07-22/journal.md
```

Verify:

- Retroactive entries are ordered chronologically in `journal.md`.
- Done tasks carry a `(done YYYY-MM-DD)` suffix.
- `datecache.md` contains a line for `2026-07-22` mentioning both the journal entries and the completed task.
- Task priorities sort as `!0` before `!1` before `!2`.

## Regression hotspots

When changing any of these areas, run the corresponding checks carefully:

| Area | Files | What to check |
|---|---|---|
| Markdown parsing | `main.go`, `tui_task.go`, `tui_notes.go`, `tui_browser.go` | Existing notes still load, search/filter still match, legacy task lines parse |
| Retroactive writes | `main.go` | `addJournal` preserves order; `staleCacheHint` prints for cached days; future dates rejected |
| Task CRUD | `main.go`, `tui_task.go` | ID counter stays monotonic; done/open toggles do not duplicate stamps; priority re-sorting works |
| Cache builder | `main.go` | Catch-up on missed days; task-only days get a line; `notie cache <date>` refreshes without duplicates |
| Terminal UI | `term.go`, `tui_*.go` | TUIs open in a real terminal, fall back to plain output when piped, restore terminal state on quit |
| Voice notes | `record.go` | Recording and transcription still route to the right file; cancellation discards cleanly |

## Suggested first unit tests

If adding tests, the highest-value targets are pure functions in `main.go`:

- `addJournal` — assert insertion order for retroactive entries.
- `taskPri` and task sorting — assert `!0` < `!1` < `!2` < legacy.
- `doneTasksByDate` — assert only `[x]` tasks with `(done ...)` are mapped.
- `cmdCache` write gating — assert no-op when nothing changed, refresh when forced, removal when a day's content is emptied.
- `nextID` — assert self-healing when `.task_seq` is behind `task.md`.

Tests should create a temporary directory and set `NOTIE_DIR` to it, then assert on file contents. Since the project uses only the Go standard library, tests can live alongside production code (`*_test.go`) with no new dependencies.

## CI

The OpenWiki GitHub workflow (`.github/workflows/openwiki-update.yml`) runs `openwiki code --update --print` on a schedule. It does not currently run `go test` because none exist; if tests are added, add a `go test ./...` step to the workflow.

## Useful commands

```sh
go build -o notie .
go vet ./...
NOTIE_DIR=/tmp/notie-test ./notie add "vet passed"
```
