---
name: notie-review
description: >-
  Generate a weekly, monthly, or quarterly review of the user's notie notes —
  what they achieved over a period — from their journals and tasks, grouping
  tasks into done vs still-incomplete, and optionally applying task edits (mark
  done, reopen, delete, add) through native notie commands. Use whenever the
  user asks for a notie review/recap/summary, "what did I get done this
  week/month/quarter", a progress or accomplishments report over a time period,
  a standup or retro built from their notes, or asks to reconcile / tidy / clean
  up their notie tasks after reviewing. Trigger even if the user doesn't say the
  word "notie" but clearly means their local notes, journal, or task list.
---

# notie-review

Produce a period review (weekly / monthly / quarterly) of the user's local
`notie` notes, and — when asked — reconcile their task list. The review draws on
**journals** and **tasks** as the primary sources, with **important.md** and
**remember.md** folded in as supporting context.

You (the model running this skill) write the summary yourself by reading the
files. Do **not** shell out to the `claude` CLI or `notie cache` to generate
prose. Reserve `notie` commands for the two things only `notie` should do:
listing tasks and editing them.

## How notie stores data (so you can parse it)

Notes directory: `${NOTIE_DIR:-$HOME/.notie}`. Always honor `NOTIE_DIR`.

| What | Path | Line format |
|------|------|-------------|
| Journal (per day) | `<dir>/<YYYY-MM-DD>/journal.md` | `- HH:MM — <text>` |
| Tasks | `<dir>/task.md` | open: `- [ ] #<id> <desc> (added YYYY-MM-DD)` · done: `- [x] #<id> <desc> (added …) (done YYYY-MM-DD)` |
| Important | `<dir>/important.md` | `- YYYY-MM-DD HH:MM — <text>` |
| Remember | `<dir>/remember.md` | `- YYYY-MM-DD HH:MM — <text>` |
| Date cache | `<dir>/datecache.md` | `- YYYY-MM-DD: <one-line summary>` |
| Shell audit | `<dir>/<YYYY-MM-DD>/shell.md` | `- HH:MM (loc) $ <cmd>` (not used unless asked) |

Task parsing: a task line matches `^- \[[ x]\] #(\d+) `; `[x]` = done; the
optional ` (done YYYY-MM-DD)` suffix carries the completion date. Legacy done
tasks may lack that suffix — treat them as done with an unknown date.

The em dash in journal/note lines is `—` (U+2014), not a hyphen.

## Step 1 — Locate the notes directory

```sh
DIR="${NOTIE_DIR:-$HOME/.notie}"
```

If `$DIR` doesn't exist, tell the user there are no notie notes yet and stop.

## Step 2 — Resolve the review period

Default to **calendar-to-date** and **state the resolved range at the top of the
review, inviting correction** — date interpretation is the easiest thing to get
subtly wrong, so make it visible.

| Request | Default range (today = T) |
|---------|---------------------------|
| weekly / "this week" | Monday of T's week → T |
| monthly / "this month" | 1st of T's month → T |
| quarterly / "this quarter" | first day of T's quarter → T |

Quarters: Q1 Jan–Mar, Q2 Apr–Jun, Q3 Jul–Sep, Q4 Oct–Dec.

A named or explicit period **overrides** the default and uses the *full* range:
"last week" (previous Mon–Sun), "June" / "June 2026" (whole month), "Q2"
(Apr 1–Jun 30), or an explicit `YYYY-MM-DD..YYYY-MM-DD`.

Compute boundaries with `date` rather than by hand — this is macOS (BSD `date`):

```sh
T=$(date +%F)
# Monday of this week (BSD date):
week_start=$(date -v-mon +%F 2>/dev/null || date -v monday +%F)
month_start=$(date +%Y-%m-01)
# Quarter start:
q_month=$(( ( ($(date +%-m) - 1) / 3 ) * 3 + 1 ))
quarter_start=$(printf '%d-%02d-01' "$(date +%Y)" "$q_month")
```

Dates are plain `YYYY-MM-DD` strings, so a date `d` is in range when
`start <= d <= end` by lexical comparison — no parsing needed.

## Step 3 — Gather the data (read-only)

1. **Journal days in range.** List candidate day directories and keep those in
   range, then read each day's journal:
   ```sh
   ls -1 "$DIR" | grep -E '^[0-9]{4}-[0-9]{2}-[0-9]{2}$' | awk -v a="$start" -v b="$end" '$1>=a && $1<=b'
   ```
   Read `<dir>/<date>/journal.md` for each. For **older** days you can lean on
   `<dir>/datecache.md` one-liners to keep context small — but note the current
   day is never cached, so always read today's journal directly.

2. **Tasks.** Read `<dir>/task.md` and classify every task line:
   - **Completed this period** — `[x]` whose `(done <date>)` is in range.
   - **Still open, added this period** — `[ ]` whose `(added <date>)` is in range.
   - **Still open, carried over** — `[ ]` whose `(added <date>)` is *before* the
     range start. Compute how long it's been open (T − added) and flag as aging.
   - Ignore `[x]` tasks completed outside the range for the achievements list,
     but you may count them in the totals.

3. **Important & remember.** Read `<dir>/important.md` and `<dir>/remember.md`
   if present; keep only lines whose leading `YYYY-MM-DD` falls in range.

## Step 4 — Write the review

Synthesize from what you gathered. Follow this template:

```
# notie review — <Period label> (<start> → <end>)

## Achievements
<A short narrative of what got done, grounded in the journals and the tasks
completed this period, with important/remember entries woven in. Group related
work into themes rather than replaying every line. 2–5 short paragraphs or
themed bullet clusters.>

## Tasks
### ✅ Completed this period (N)
- #<id> <desc>  (done <date>)

### ⏳ Still open (N)
**Added this period (N):**
- #<id> <desc>  (added <date>)
**Carried over (N):**
- #<id> <desc>  (added <date>, open <X>d)

## Highlights & notes
<Bullets from important.md / remember.md dated in the period. Omit the section
if there are none.>

## By the numbers
- Journal entries: <n> across <m> day(s)
- Tasks: <c> completed · <a> added · <o> still open (<k> carried over)
```

Rules that keep this trustworthy:
- **Stay grounded in the notes.** Summarize and connect what's there; never
  invent accomplishments, dates, or tasks.
- If a section has no data, say so briefly (e.g. "No journal entries this
  period") rather than padding.
- If the whole period is empty, say the period has no notie activity and
  suggest a wider range.

## Step 5 — Task edits (only when the user asks)

After seeing the review the user may want to reconcile their list — "mark #12
and #14 done", "drop that stale one", "add a follow-up". Apply these through
**native notie commands only**. Never hand-edit `task.md` or any `.md`: notie
owns the ids, the `.task_seq` counter, and the done/added stamps, and direct
edits desync them.

| Intent | Command |
|--------|---------|
| Mark done | `notie task done <id>` |
| Reopen | `notie task open <id>` |
| Delete | `notie task del <id>` |
| Add | `notie task "<text>"` |

Honor `NOTIE_DIR` on every call (e.g. `NOTIE_DIR="$DIR" notie task done 12`) so
edits land in the same store you reviewed.

Before running anything:
- **Confirm the exact change list with the user first.** Restate each id and
  what will happen.
- Note that `done` ↔ `open` is fully reversible, but **`del` permanently
  removes** the task — call that out for any deletion.

After applying, run `notie task list` (with the right `NOTIE_DIR`) and re-show
the updated **Completed / Still open** grouping so the user sees the result.

If `command -v notie` fails, the binary isn't on `PATH`: reviews still work
(they only read files), but edits need the binary — point the user to the repo's
`setup.sh` (or `go build -o notie .`) to install it.

## Edge notes

- Everything uses local dates, matching how notie itself timestamps entries.
- Don't run `notie cache` implicitly. For a large or mostly-past period you may
  *offer* to run it first to build `datecache.md` for cheaper reads, but only
  with the user's OK (it can invoke the `claude` CLI and writes a file).
- Shell audit (`shell.md`) is excluded by default; only fold it in if the user
  explicitly wants command activity in the review.
