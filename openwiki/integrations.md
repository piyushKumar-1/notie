---
type: Concept
title: Integrations
description: Optional runtime integrations that extend notie beyond the core binary — voice recording, the zsh shell-audit hook, and the Claude Code notie-review skill.
tags: [integrations, voice, shell, claude, transcription]
---

# Integrations

The core `notie` binary is self-contained, but it detects and uses several optional external tools and hooks. Each integration is opt-in and degrades gracefully when its dependency is missing.

## Voice notes

Voice notes are implemented in [`record.go`](record.go) and exposed through the `r*` commands:

- `radd` / `rjournal` — append to today's journal
- `raddi` / `rimportant` — append to `important.md`
- `rremember` — append to `remember.md`
- `rtask` — create a task (asks for priority after transcription)

### Recording

Recording uses **ffmpeg** with the macOS `avfoundation` input:

```sh
ffmpeg -nostdin -hide_banner -loglevel error -y \
  -f avfoundation -i :default -ac 1 -ar 16000 \
  /tmp/notie-rec-<pid>.wav
```

The user stops recording by pressing `Enter`, at which point the process is sent `SIGINT`.

### Transcription

[`transcribe()`](record.go) prefers `hear` (Apple Speech) when installed, otherwise falls back to `whisper-cli` with a local Whisper model.

- `hear` — `brew install hear`
- `whisper-cli` — `brew install whisper-cpp`; download a model to `~/.cache/whisper/` or set `NOTIE_WHISPER_MODEL`

`whisperModel()` picks the largest available model from a hard-coded preference list:

1. `ggml-large-v3-turbo.bin`
2. `ggml-medium.bin`
3. `ggml-small.bin`
4. `ggml-base.bin`

Transcripts are collapsed to a single line, and non-speech artifacts like `[BLANK_AUDIO]` are rejected as "heard nothing".

### What voice tasks add

| Command | Destination |
|---|---|
| `radd` / `rjournal` | [`cmdAdd()`](main.go) → today's `journal.md` |
| `raddi` / `rimportant` | [`cmdDated()`](main.go) → `important.md` |
| `rremember` | [`cmdDated()`](main.go) → `remember.md` |
| `rtask` | asks for priority, then [`cmdTask()`](main.go) → `task.md` |

See [`record.go`](record.go) for the full flow.

## Shell audit trail

The shell audit trail logs every interactive command to `~/.notie/<date>/shell.md`. It is normally driven by a zsh `preexec` hook rather than by hand.

### Hook installation

[`setup.sh`](setup.sh) offers to append the following to `~/.zshrc`:

```zsh
# notie shell audit trail
_notie_log() { command notie log "$1" >/dev/null 2>&1 }
autoload -Uz add-zsh-hook
add-zsh-hook preexec _notie_log
```

The wrapper suppresses output so the hook never clutters the prompt.

### Storage format

[`cmdLog()`](main.go) writes lines like:

```markdown
- 09:14 (~/.notie) $ notie add "fixed caching bug"
```

The working directory is abbreviated to `~` when it is under `$HOME`. Empty or whitespace-only commands are silently dropped.

### Browser

`notie shell` launches the same [day-browser TUI](tui.md#day-browser) used by `notie journal`, configured with [`shellBrowser()`](tui_browser.go).

## Claude Code skill: `notie-review`

The repository includes a [Claude Code skill](.claude/skills/notie-review/SKILL.md) that generates weekly/monthly/quarterly reviews from the user's notes and applies task edits through native `notie` commands.

### What the skill does

1. Locates the notes directory (`${NOTIE_DIR:-$HOME/.notie}`).
2. Resolves the requested period (weekly/monthly/quarterly, calendar-to-date by default).
3. Gathers journals, tasks, `important.md`, and `remember.md` for that period.
4. Writes an "Achievements / Tasks / Highlights / By the numbers" review.
5. Optionally reconciles tasks through commands like `notie task done <id>`.

### Why it matters

The skill encodes the line formats and date-arithmetic rules that notie uses, so Claude Code can read and reason about notes without invoking `notie` for every lookup. It is the only consumer besides the binary that parses `task.md`, `datecache.md`, and journal files.

### Installation

[`setup.sh`](setup.sh) can copy `SKILL.md` into `~/.claude/skills/notie-review/`. Manual install:

```sh
cp -R .claude/skills/notie-review ~/.claude/skills/
```

See [`operations.md`](operations.md) for the full setup flow and [`source-map.md`](source-map.md) for the skill's relationship to the rest of the codebase.
