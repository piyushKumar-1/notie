---
type: Runbook
title: Operations
description: How to build, install, configure, and troubleshoot notie; covering setup.sh, environment variables, optional dependencies, and common failure modes.
tags: [operations, build, install, setup, troubleshooting]
---

# Operations

This page covers building, installing, configuring, and troubleshooting `notie`. For day-to-day command usage, see [`workflows.md`](workflows.md). For how the code is organized, see [`source-map.md`](source-map.md).

## Requirements

- **Go 1.25+** — the only hard build requirement ([`go.mod`](go.mod)).
- **macOS** for voice notes (uses ffmpeg's `avfoundation` input); everything else works anywhere Go runs.

Optional runtime dependencies:

| Dependency | Used for | Install |
|---|---|---|
| `ffmpeg` | Voice recording | `brew install ffmpeg` |
| `hear` | On-device transcription (preferred) | `brew install hear` |
| `whisper-cli` | Fallback transcription | `brew install whisper-cpp` + model |
| `claude` CLI | Nicer `notie cache` summaries | [Claude Code](https://claude.com/claude-code) |

## Build & install

### Quick path

```sh
git clone <repo-url> notie
cd notie
./setup.sh
```

### Manual path

```sh
go build -o notie .
mv notie ~/.local/bin/   # or anywhere on PATH
mkdir -p ~/.notie
```

### `--yes` / non-interactive setup

```sh
./setup.sh --yes
```

This accepts all defaults without prompting.

## Environment variables

| Variable | Purpose | Default |
|---|---|---|
| `NOTIE_DIR` | Notes storage directory | `$HOME/.notie` |
| `NOTIE_WHISPER_MODEL` | Override Whisper model path | best model in `~/.cache/whisper` |

All command implementations resolve the data directory through [`notieDir()`](main.go), so `NOTIE_DIR` affects every command consistently.

## `setup.sh` walkthrough

[`setup.sh`](setup.sh) performs the following steps:

1. **Check Go.** Prefers system `go`, offers Homebrew install, or downloads the official toolchain to `~/.cache/notie` for build-only use.
2. **Build.** Runs `go build -o notie .`
3. **Install.** Copies the binary to `~/.local/bin` (or `/usr/local/bin` if writable) and warns if the directory is not on `PATH`.
4. **Create notes directory.** Uses `${NOTIE_DIR:-$HOME/.notie}`.
5. **Offer zsh shell-audit hook.** Adds the `preexec` hook to `~/.zshrc` if accepted.
6. **Offer Claude Code skill install.** Copies `.claude/skills/notie-review` to `~/.claude/skills/`.
7. **Report optional dependencies.** Checks for `ffmpeg`, `hear`/`whisper-cli`, and `claude`.

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `notie radd` says "recording needs ffmpeg" | ffmpeg not installed or not on PATH | `brew install ffmpeg` |
| "no audio captured — check the microphone permission" | Terminal lacks mic access | Grant microphone permission in **System Settings → Privacy & Security → Microphone** |
| "whisper model missing" | No Whisper model in `~/.cache/whisper` | Download one from [huggingface.co/ggerganov/whisper.cpp](https://huggingface.co/ggerganov/whisper.cpp) or set `NOTIE_WHISPER_MODEL` |
| `notie cache` produces joined text instead of a summary | `claude` CLI not installed | Install Claude Code or accept the fallback |
| Task edits seem to have no effect | Editing `task.md` by hand | Use `notie task done/open/del/add` only — hand-edits desync `.task_seq` and IDs |
| Retroactive entry but `datecache.md` looks stale | A cached day was edited | Run `notie cache <YYYY-MM-DD>` to rebuild that day's summary |
| Future-date refused | `pastDate()` rejects dates > today | Use today's date or a past date |

## Backup / migration

Because everything is plain markdown in one directory, backups are trivial:

```sh
# backup
tar czf notie-backup-$(date +%F).tar.gz ~/.notie
# move to another machine
rsync -av ~/.notie newhost:~/.notie
```

The only generated state files are `.task_seq` and `datecache.md`; everything else is append-only user data.
