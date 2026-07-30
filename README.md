# notie

Tiny local notes CLI. Everything lives in `~/.notie` as plain markdown — no
database, no sync, no accounts. Journals, tasks, important notes, things to
remember, a shell audit trail, and voice notes, all from the terminal.

Built with the Go standard library only; a single standalone binary.

## Quick start

```sh
git clone <repo-url> notie
cd notie
./setup.sh
```

The setup script builds the binary, puts it on your `PATH`, creates `~/.notie`,
and optionally wires up the zsh shell-audit hook. See [Setup](#setup) for the
manual steps.

## Usage

```
notie add "text"        append to today's journal (~/.notie/<date>/journal.md)
notie add <date> [HH:MM] "text"
                        add to an older day, in chronological position
notie did <date> "text" record a task that was already done on that day
notie log "cmd"         append a shell command to today's audit trail
notie addi "text"       append to important.md
notie remember "text"   append to remember.md
notie task <0|1|2> "text"  add a task (0 high · 1 normal · 2 low), then show tasks
notie radd              record voice, transcribe, append to today's journal
                        (also: rjournal · raddi/rimportant · rremember · rtask)
notie task              interactive task list (done tasks hidden by default)
notie task list         plain list of last 100 tasks, grouped by priority
notie task done <id>    mark task done       (also: open, del)
notie journal           interactive journal browser (dates sidebar + / search)
notie shell             interactive shell-audit browser
notie important         interactive important-notes browser
notie remember          interactive remember-notes list
notie cache [<date>]    build datecache.md one-line summaries for past days
                        (a date re-summarizes just that day)
notie show [what]       print a file (journal|shell|remember|important|task|datecache|YYYY-MM-DD)
notie upgrade           clone the public repo, rebuild, and replace this binary
```

`notie upgrade` shallow-clones the public repo to a temp dir, builds it with the
Go toolchain, and installs the fresh binary over the running one (in place, via
an atomic rename). Needs `git` and `go` on your `PATH`. Set `NOTIE_REPO` to
upgrade from a fork or local checkout instead of the default public repo.

TUI keys: `j`/`k` move · `gg`/`G` top/bottom · `x` toggle (tasks)
· `0`/`1`/`2` set priority (tasks) · `.` show/hide done (tasks) · `dd` delete
· `a` add · `/` search · `n`/`N` next/prev · `q` or `:q` quit
· `:ff <pat>` find date files · `:fg <pat>` find mentions

In the journal browser, `a` adds to the **selected** day, so scrolling back and
pressing `a` is the interactive way to write up a day you missed.

Task lists are grouped by priority (`!0` → `!1` → `!2`), oldest first within a
group; tasks predating priorities sort last.

## Writing things up late

Both retroactive commands take a past date and refuse a future one:

```sh
notie add 2026-07-23 "reviewed the Pomerium upgrade plan"   # timestamped now
notie add 2026-07-23 14:05 "reviewed the upgrade plan"      # explicit time
notie did 2026-07-23 "rotated the staging DB credentials"   # already-done task
```

Entries are inserted in chronological position rather than appended, so a day
always reads in order. If the day already has a `datecache.md` summary, the
command says so — run `notie cache <date>` to rebuild that one line.

## Data layout

```
~/.notie/
├── 2026-07-22/
│   ├── journal.md      timestamped journal entries for the day
│   └── shell.md        shell audit trail for the day
├── task.md             tasks with ids, e.g. "- [ ] #12 !1 buy milk (added 2026-07-22)"
├── important.md        dated important notes
├── remember.md         dated things to remember
├── datecache.md        one-line-per-day summaries built by `notie cache`
└── .task_seq           task id counter
```

Set `NOTIE_DIR` to store notes somewhere other than `~/.notie`.

## Setup

### Requirements

- **Go 1.25+** — the only build requirement. Don't have it? `setup.sh` offers
  to install it via Homebrew, or downloads the official toolchain to
  `~/.cache/notie` and uses it just for the build (nothing installed
  system-wide).
- **macOS** for voice notes (recording uses ffmpeg's avfoundation input);
  everything else works anywhere Go runs.

### Build & install

```sh
go build -o notie .
mv notie ~/.local/bin/   # or anywhere on your PATH
```

### Shell audit trail (optional)

`notie shell` shows a per-day log of every command you ran. It's fed by a zsh
preexec hook — add this to `~/.zshrc` (or let `setup.sh` do it):

```zsh
# notie shell audit trail
_notie_log() { command notie log "$1" >/dev/null 2>&1 }
autoload -Uz add-zsh-hook
add-zsh-hook preexec _notie_log
```

### Claude Code skill (optional)

The repo ships a [`notie-review`](.claude/skills/notie-review/SKILL.md) skill for
[Claude Code](https://claude.com/claude-code). It teaches Claude to build a
weekly/monthly/quarterly review from your notie journals and tasks, and to
reconcile your task list through native `notie` commands. `setup.sh` offers to
install it globally to `~/.claude/skills/` so it works from any directory; to do
it by hand:

```sh
cp -R .claude/skills/notie-review ~/.claude/skills/
```

### Voice notes (optional, macOS)

`notie radd` and friends record the mic and transcribe on-device:

- `brew install ffmpeg` — records the microphone.
- Transcription uses the [`hear`](https://github.com/sveinbjornt/hear) CLI
  (Apple Speech, `brew install hear`) if installed, otherwise falls back to
  `whisper-cli` (`brew install whisper-cpp`) with a local model from
  [huggingface.co/ggerganov/whisper.cpp](https://huggingface.co/ggerganov/whisper.cpp)
  placed in `~/.cache/whisper/` (or pointed to by `NOTIE_WHISPER_MODEL`).
- Grant your terminal microphone permission the first time you record.

### Daily summaries (optional)

`notie cache` writes a one-line summary per past day into `datecache.md`,
covering both that day's journal and the tasks closed on it — so a day spent
only ticking tasks off still gets a line. If the
[`claude`](https://claude.com/claude-code) CLI is installed it's used for nicer
summaries; otherwise entries are joined verbatim. Idempotent and catch-up
safe — run it whenever, e.g. from a daily cron/launchd job.
