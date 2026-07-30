---
type: Concept
title: Core workflows
description: End-to-end workflows for adding journal entries, retroactive dating, managing tasks, building the date cache, and displaying stored data in notie.
tags: [workflows, commands, journal, tasks, datecache]
---

# Core workflows

The CLI in [`main.go`](main.go) exposes a small set of commands. Each command maps to a function that reads or writes the markdown store directly.

## Adding journal entries

`notie add` is the primary entry point for journal entries. It dispatches to `cmdAdd` → `addJournal`.

```sh
notie add "shipped the cache builder"
notie add 2026-07-23 14:05 "retroactive entry"
```

Argument parsing in `main` sniffs an optional date and optional time:

1. If the first argument matches `YYYY-MM-DD`, it becomes the target date.
2. If the next argument matches `HH:MM`, it becomes the timestamp.
3. Everything else is the body.

If no date is provided, today is used. If no time is provided, the current clock is used. Past dates are allowed; future dates are rejected by `pastDate`.

`addJournal` inserts the new `- HH:MM — text` line in chronological position when the target day's file already exists, preserving order for retroactive edits. For a new day it simply appends after the `# Journal — <date>` header.

## Recording already-done work

`notie did <YYYY-MM-DD> "text"` records a task that was completed on a past date. It creates a done task line with matching `added` and `done` stamps:

```text
- [x] #7 !1 closed the auth issue (added 2026-07-23) (done 2026-07-23)
```

Priority is fixed at normal (`!1`) because a completed task's priority is moot, but the marker keeps the line parseable by `taskFullRe`. The `added` stamp must precede the `done` stamp or `doneTasksByDate` silently drops the task.

## Managing tasks

Task commands route through `cmdTask`:

| Command | Behavior |
|---------|----------|
| `notie task 0 "text"` | Add high-priority task |
| `notie task 1 "text"` | Add normal-priority task |
| `notie task 2 "text"` | Add low-priority task |
| `notie task` | Launch interactive task TUI when stdin is a TTY, else print grouped list |
| `notie task list` | Print grouped list of last 100 tasks |
| `notie task done <id>` | Mark task done and stamp today |
| `notie task open <id>` | Reopen task and remove done stamp |
| `notie task del <id>` | Delete task line permanently |

Tasks are grouped by priority (`!0` → `!1` → `!2`, legacy unmarked tasks last), oldest first within a group. IDs come from `.task_seq` and are self-healed against the maximum ID in `task.md`.

## Shell audit trail

`notie log "cmd"` appends a command to today's `shell.md`. It is silent on success and tolerates empty input because it is normally invoked from a zsh `preexec` hook. Each line records the timestamp and abbreviated working directory:

```text
- 09:14 (~) $ kubectl get pods
```

See [Integrations](integrations.md) for hook installation.

## Building the date cache

`notie cache` walks the notes directory and produces one-line summaries in `datecache.md` for every past day that has either a journal or a completed task. `notie cache <YYYY-MM-DD>` re-summarizes a single day, typically after a retroactive edit.

The summary pipeline:

1. `doneTasksByDate` maps completion dates to task descriptions.
2. `journalEntries` strips timestamps from a day's journal lines.
3. `summarize` prefers the `claude` CLI (headless, `haiku` model, 2-minute timeout) for natural-language summaries; otherwise it joins entries verbatim.

A day spent only completing tasks still gets a line, so the cache catches task-only days even when no journal directory exists.

## Displaying data

`notie show [what]` prints files directly:

| Argument | Output |
|----------|--------|
| `journal` | Today's journal, or the most recent journal day |
| `shell [date]` | Today's shell log, or the most recent shell day |
| `important`, `remember`, `datecache` | The corresponding flat file |
| `task`, `todo` | Grouped task list |
| `YYYY-MM-DD` | Journal for that date |

When a TTY is detected, `notie journal`, `notie shell`, `notie important`, and `notie task` launch their respective TUIs instead of printing.

## Retroactive edits and stale cache hints

Both `notie add <date>` and `notie did <date>` check `cachedDate` in [`main.go`](main.go). If the target day already has a `datecache.md` line, the command prints a hint suggesting `notie cache <date>`. Today is never cached, so no hint is printed for today.
