---
type: Quickstart
title: notie quickstart
description: Entrypoint for understanding the notie local notes CLI — a single Go binary that stores journals, tasks, reminders, and a shell audit trail as plain markdown in ~/.notie.
tags: [notie, quickstart, cli, go, markdown]
---

# notie quickstart

`notie` is a tiny local notes CLI: one Go binary, no database, no sync, no accounts. Everything lives as plain markdown under `~/.notie` (or `$NOTIE_DIR`).

What it tracks:

- **Daily journals** — `notie add` appends a timestamped entry to today's `journal.md`; supports [retroactive dating](workflows.md).
- **Tasks** — prioritized (`!0/!1/!2`) task list with an interactive TUI, native keyboard shortcuts, and `done/open/del` subcommands.
- **Important / Remember notes** — flat dated lists with their own browsers.
- **Shell audit trail** — a zsh `preexec` hook writes every interactive command to `shell.md` for the day.
- **Voice notes** — `notie radd` records the mic via `ffmpeg` and transcribes with Apple `hear` or local `whisper-cpp`.
- **Daily summaries** — `notie cache` builds one-line `datecache.md` summaries for past days using the journal plus completed tasks.

Start with [architecture.md](architecture.md) for the codebase layout and design choices, then browse [source-map.md](source-map.md) for a file-by-file guide.

## Common commands

```sh
# Journaling
notie add "deployed the new checkout flow"
notie add 2026-07-23 14:05 "reviewed the upgrade plan"
notie did 2026-07-23 "rotated the staging DB credentials"
notie journal                    # interactive day browser

# Tasks
notie task 1 "buy milk"
notie task                       # interactive TUI
notie task done 12
notie task list

# Other notes
notie addi "call design team"    # important
notie remember "renew passport"  # remember list
notie log "kubectl apply ..."    # shell audit trail (usually via zsh hook)

# Voice
notie radd                       # record → transcribe → today's journal
notie rtask                      # record → transcribe → task

# Summaries
notie cache                      # catch-up summaries for past days
notie cache 2026-07-23           # rebuild one day after retroactive edits
```

TUI basics: `j`/`k` move, `gg`/`G` top/bottom, `/` search, `a` add, `q`/`:q` quit. Task keys also include `x` toggle done, `0`/`1`/`2` set priority, `.` show/hide done, `dd` delete. Browser-only extras: `:ff <pat>` find dates, `:fg <pat>` grep content.

## Repository layout

```
notie/
├── main.go              # command dispatch + core file ops
├── record.go            # voice-note recording & transcription
├── tui_browser.go       # day-browser TUI (journal / shell / important)
├── tui_task.go          # interactive task list
├── tui_notes.go         # interactive flat-list browser (remember)
├── term.go              # raw-mode terminal control (darwin ioctls)
├── theme.go             # ANSI colors, icons, text helpers
├── setup.sh             # build + install + optional hooks
├── README.md
└── .claude/skills/notie-review/SKILL.md   # Claude Code skill for reviews
```

## Documentation sections

- [Architecture](architecture.md) — design philosophy, storage model, TUI layers, and how to extend the tool.
- [Data model](data-model.md) — the exact line formats for journals, tasks, notes, shell audit, and datecache.
- [Workflows](workflows.md) — how writes mutate files and the invariants they preserve.
- [Operations / runbook](operations.md) — setup, environment variables, integrations, troubleshooting, and backups.
- [Integrations](integrations.md) — zsh hook, Claude summaries, voice notes, and the `notie-review` Claude skill.
- [Testing](testing.md) — how to validate changes and what is covered today.
- [Source map](source-map.md) — what each source file owns and where to start when changing behavior.

## Backlog

- Performance / scale guidance — notie intentionally targets personal-scale files; document only if growth becomes a concern.
- Linux/BSD raw-mode TUI support — `term.go` uses Darwin ioctls today; core commands compile everywhere, but the day browser and task TUI only work on macOS. Document porting approach if implemented.
