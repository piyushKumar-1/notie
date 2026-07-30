---
type: Data Model
title: notie data model
description: Exact file layout and line formats used by notie in ~/.notie — journals, tasks, important/remember notes, shell audit, datecache, and the task id counter.
tags: [data-model, markdown, storage, files]
---

# notie data model

Everything notie stores is plain Markdown in `~/.notie` (override with `NOTIE_DIR`). The only non-Markdown file is `.task_seq`, which holds the highest used task id.

## Directory layout

```
~/.notie/
├── 2026-07-22/
│   ├── journal.md      timestamped entries for the day
│   └── shell.md        shell audit trail for the day
├── task.md             tasks with ids and priorities
├── important.md        dated important notes
├── remember.md         dated things to remember
├── datecache.md        one-line-per-day summaries
└── .task_seq           task id counter
```

Per-day directories are only created when there is content for that day. A day can exist in `datecache.md` even without a directory, because a day spent only closing tasks still generates a summary.

## Journal entries (`<date>/journal.md`)

Header line:
```markdown
# Journal — 2026-07-22
```

Entry lines:
```markdown
- 14:05 — reviewed the Pomerium upgrade plan
```

- The timestamp is `HH:MM`.
- The separator is an em dash (`—`, U+2014).
- Retroactive inserts (see [workflows](workflows.md)) preserve chronological order by placing the new line after the last earlier timestamp.

## Tasks (`task.md`)

Header line:
```markdown
# Tasks
```

Open task:
```markdown
- [ ] #12 !1 buy milk (added 2026-07-22)
```

Done task:
```markdown
- [x] #12 !1 buy milk (added 2026-07-22) (done 2026-07-23)
```

Field meanings:

| Field | Meaning |
|-------|---------|
| `#12` | Unique task id, assigned from `.task_seq` |
| `!1`  | Priority: `0` high, `1` normal, `2` low |
| `added 2026-07-22` | Creation date |
| `done 2026-07-23` | Completion date (only when `[x]`) |

Legacy tasks created before priorities may lack `!0/!1/!2` and sort after all prioritized tasks. Legacy done tasks may also lack `(done ...)`, in which case their completion date is unknown and they are skipped by the datecache.

## Important and Remember notes

Both files use the same line format:

```markdown
- 2026-07-22 09:41 — call design team
```

The date/time prefix makes them sortable and allows the [notie-review skill](integrations.md) to filter by period. `important.md` is rendered in the day browser by grouping identical dates; `remember.md` is rendered as a flat list.

## Shell audit trail (`<date>/shell.md`)

Header:
```markdown
# Shell — 2026-07-22
```

Entry:
```markdown
- 14:05 (~Code/project) $ kubectl get pods
```

The working directory is abbreviated with `~` when inside `$HOME`. Entries come from the zsh preexec hook calling `notie log`.

## Datecache (`datecache.md`)

One summary line per past day:

```markdown
# Date cache

- 2026-07-22: reviewed the Pomerium upgrade plan; closed issue #904
```

- Today is never cached.
- The summary is generated from the day's journal plus tasks closed that day.
- If the `claude` CLI is available, it produces the one-liner; otherwise entries are joined verbatim.
- Running `notie cache <date>` drops and rebuilds a single line, which is useful after retroactive edits.

## Task id counter (`.task_seq`)

A single ASCII integer written by `nextID()` after bumping. The function also self-heals by scanning `task.md` for the maximum existing id, so hand-edited files will not collide.
