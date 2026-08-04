// Day browser: dates sidebar on the left, the selected day's entries on the
// right. Backs `notie journal`, `notie shell` and `notie important`.
//
// Two panes with a focus model: ↑/↓ move within the focused pane, ←/→ switch
// focus between the date menu and that day's entries. In the entries pane you
// can edit (e) and delete (dd) — journal and important are editable; shell is a
// read-only audit trail. / searches dates AND content · :ff finds dates (live) ·
// :fg greps content · n/N jump between matching days · q or :q quits.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const sideW = 14 // " ▸ 2026-07-14 "

// browserEntryRe matches a generic "- HH:MM rest" line (shell audit entries).
var browserEntryRe = regexp.MustCompile(`^- (\d{2}:\d{2}) (.*)$`)

// dayEntry is one selectable row in the content pane: a timestamp and its text.
type dayEntry struct{ ts, text string }

// browserCfg adapts the day browser to one data source.
type browserCfg struct {
	label, icon string
	dates       func() []string           // days that have content (any order)
	entries     func(d string) []dayEntry // that day's selectable entries, in file order
	// add writes to the selected day and returns the date it actually wrote to,
	// which is not always d — see importantBrowser. nil disables 'a'.
	add func(d, text string) string
	// edit rewrites entry i's text; del removes it. Both nil ⇒ a read-only
	// content pane (shell). i indexes into entries(d).
	edit      func(d string, i int, text string)
	del       func(d string, i int)
	summaries func() map[string]string // nil: no per-day summary line
	fallback  func()                   // plain output when raw mode fails
	empty     string                   // message when there are no days
}

// datesWithFile lists the date dirs that contain the given file.
func datesWithFile(name string) func() []string {
	return func() []string {
		var out []string
		dirents, _ := os.ReadDir(notieDir())
		for _, de := range dirents {
			d := de.Name()
			if de.IsDir() && dateRe.MatchString(d) {
				if _, err := os.Stat(filepath.Join(notieDir(), d, name)); err == nil {
					out = append(out, d)
				}
			}
		}
		return out
	}
}

func journalBrowser() browserCfg {
	return browserCfg{
		label: "journal", icon: iJournal,
		dates: datesWithFile("journal.md"),
		entries: func(d string) []dayEntry {
			var out []dayEntry
			for _, l := range readLines(journalPath(d)) {
				if m := entryRe.FindStringSubmatch(l); m != nil {
					out = append(out, dayEntry{m[1], entryRe.ReplaceAllString(l, "")})
				}
			}
			return out
		},
		add: func(d, text string) string { addJournal(d, clock(), text); return d },
		edit: func(d string, i int, text string) {
			path := journalPath(d)
			lines := readLines(path)
			n := 0
			for li, l := range lines {
				m := entryRe.FindStringSubmatch(l)
				if m == nil {
					continue
				}
				if n == i {
					lines[li] = "- " + m[1] + " — " + text
					writeLines(path, lines)
					return
				}
				n++
			}
		},
		del: func(d string, i int) {
			path := journalPath(d)
			lines := readLines(path)
			n := 0
			for li, l := range lines {
				if !entryRe.MatchString(l) {
					continue
				}
				if n == i {
					writeLines(path, append(lines[:li], lines[li+1:]...))
					return
				}
				n++
			}
		},
		summaries: func() map[string]string {
			m := map[string]string{}
			for _, l := range readLines(datecachePath()) {
				if s := dcLineRe.FindStringSubmatch(l); s != nil {
					m[s[1]] = s[2]
				}
			}
			return m
		},
		fallback: func() { cmdShow("journal") },
		empty:    "no journal entries yet — press a to write one",
	}
}

func shellBrowser() browserCfg {
	return browserCfg{
		label: "shell", icon: iCursor,
		dates: datesWithFile("shell.md"),
		entries: func(d string) []dayEntry {
			var out []dayEntry
			for _, l := range readLines(filepath.Join(notieDir(), d, "shell.md")) {
				if m := browserEntryRe.FindStringSubmatch(l); m != nil {
					out = append(out, dayEntry{m[1], m[2]})
				}
			}
			return out
		},
		fallback: func() { cmdShowShell("") },
		empty:    "no shell history yet — run some commands in a terminal",
	}
}

// importantBrowser groups important.md's dated lines into per-day views.
func importantBrowser() browserCfg {
	path := filepath.Join(notieDir(), "important.md")
	// nth walks important.md's lines for day d, calling fn with the file index
	// and match of the i-th entry, then returns.
	nth := func(d string, i int, fn func(li int, m []string, lines []string)) {
		lines := readLines(path)
		n := 0
		for li, l := range lines {
			if m := noteLineRe.FindStringSubmatch(l); m != nil && m[1] == d {
				if n == i {
					fn(li, m, lines)
					return
				}
				n++
			}
		}
	}
	return browserCfg{
		label: "important", icon: iStar,
		dates: func() []string {
			seen := map[string]bool{}
			var out []string
			for _, l := range readLines(path) {
				if m := noteLineRe.FindStringSubmatch(l); m != nil && !seen[m[1]] {
					seen[m[1]] = true
					out = append(out, m[1])
				}
			}
			return out
		},
		entries: func(d string) []dayEntry {
			var out []dayEntry
			for _, l := range readLines(path) {
				if m := noteLineRe.FindStringSubmatch(l); m != nil && m[1] == d {
					out = append(out, dayEntry{m[2], m[3]})
				}
			}
			return out
		},
		// important.md is a flat, append-only file rendered in raw order, so it
		// always takes today's date — the selected day is deliberately ignored.
		add: func(_, text string) string {
			appendLine(path, "Important", fmt.Sprintf("- %s %s — %s", today(), clock(), text))
			return today()
		},
		edit: func(d string, i int, text string) {
			nth(d, i, func(li int, m, lines []string) {
				lines[li] = "- " + m[1] + " " + m[2] + " — " + text
				writeLines(path, lines)
			})
		},
		del: func(d string, i int) {
			nth(d, i, func(li int, _, lines []string) {
				writeLines(path, append(lines[:li], lines[li+1:]...))
			})
		},
		fallback: func() { cmdShow("important") },
		empty:    "nothing important yet — press a to add",
	}
}

// contentRow is one rendered line of the content pane; entry is the index of
// the dayEntry it belongs to, or -1 for the header, summary and blank rows.
type contentRow struct {
	text  string
	entry int
}

type browserTUI struct {
	cfg        browserCfg
	dates      []string   // newest first
	entries    []dayEntry // the selected day's entries (loaded each render)
	summaries  map[string]string
	focus      byte // 0: dates sidebar · 1: content
	cursor     int  // selected date
	entryCur   int  // selected entry in the content pane
	sideOff    int
	contentOff int
	pending    byte
	in         *lineInput
	search     string
	searchMode byte // 0: dates+content ('/') · 'd': dates (:ff) · 'c': content (:fg)
	status     string
	quit       bool
}

// selectDate moves to date i and resets the content pane to its top.
func (t *browserTUI) selectDate(i int) {
	t.cursor, t.entryCur, t.contentOff = i, 0, 0
}

// addTarget is the day 'a' writes to: whichever date is selected, falling back
// to today when there are no days yet (the empty browser invites 'a').
func (t *browserTUI) addTarget() string {
	if t.cursor < 0 || t.cursor >= len(t.dates) {
		return today()
	}
	return t.dates[t.cursor]
}

func (t *browserTUI) reload() {
	t.dates = t.cfg.dates()
	sort.Sort(sort.Reverse(sort.StringSlice(t.dates)))
	if t.cursor >= len(t.dates) {
		t.cursor = len(t.dates) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
	t.summaries = map[string]string{}
	if t.cfg.summaries != nil {
		t.summaries = t.cfg.summaries()
	}
	t.loadDay()
}

// loadDay refreshes the selected day's entries and clamps the entry cursor;
// focus falls back to the sidebar when the day has nothing to select.
func (t *browserTUI) loadDay() {
	t.entries = nil
	if t.cursor >= 0 && t.cursor < len(t.dates) {
		t.entries = t.cfg.entries(t.dates[t.cursor])
	}
	if t.entryCur >= len(t.entries) {
		t.entryCur = len(t.entries) - 1
	}
	if t.entryCur < 0 {
		t.entryCur = 0
	}
	if len(t.entries) == 0 {
		t.focus = 0
	}
}

// content builds the right pane for the selected date.
func (t *browserTUI) content(width int) []contentRow {
	if len(t.dates) == 0 {
		return nil
	}
	var out []contentRow
	push := func(s string) { out = append(out, contentRow{s, -1}) }
	d := t.dates[t.cursor]
	if day, err := time.Parse("2006-01-02", d); err == nil {
		push(cBold + cAccent + day.Format("Monday, 02 January 2006") + cReset)
	} else {
		push(cBold + cAccent + d + cReset)
	}
	if s, ok := t.summaries[d]; ok {
		for _, w := range wrapRunes(s, width) {
			push(cGrey + cItalic + highlight(w, t.search, cGrey+cItalic) + cReset)
		}
	}
	push("")
	for ei, e := range t.entries {
		indent := runeLen(e.ts) + 3
		for i, w := range wrapRunes(e.text, max(4, width-indent)) {
			if i == 0 {
				out = append(out, contentRow{
					cAccent + iBullet + cReset + " " + cGrey + e.ts + cReset + " " +
						highlight(w, t.search, ""), ei})
			} else {
				out = append(out, contentRow{strings.Repeat(" ", indent) + highlight(w, t.search, ""), ei})
			}
		}
	}
	return out
}

// searchLabel is the prompt shown for the active search mode.
func (t *browserTUI) searchLabel() string {
	switch t.searchMode {
	case 'd':
		return ":ff "
	case 'c':
		return ":fg "
	}
	return "/"
}

func (t *browserTUI) render() {
	t.loadDay()
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	rows, cols := termSize()
	b.WriteString(titleBar(cols, t.cfg.icon+" notie · "+t.cfg.label,
		fmt.Sprintf("%d days", len(t.dates))) + "\r\n")

	avail := max(1, rows-2)
	if t.cursor < t.sideOff {
		t.sideOff = t.cursor
	}
	if t.cursor >= t.sideOff+avail {
		t.sideOff = t.cursor - avail + 1
	}
	paneW := max(10, cols-sideW-2)
	content := t.content(paneW - 2) // leave 2 cols for the focus gutter

	// keep the focused entry in view
	if t.focus == 1 {
		first, last := -1, -1
		for i, r := range content {
			if r.entry == t.entryCur {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first >= 0 {
			if first < t.contentOff {
				t.contentOff = first
			}
			if last >= t.contentOff+avail {
				t.contentOff = last - avail + 1
			}
		}
	}
	if maxOff := max(0, len(content)-avail); t.contentOff > maxOff {
		t.contentOff = maxOff
	}
	if t.contentOff < 0 {
		t.contentOff = 0
	}

	if len(t.dates) == 0 {
		b.WriteString("\r\n  " + cGrey + t.cfg.empty + cReset + "\r\n")
	}
	for i := 0; i < avail; i++ {
		// sidebar cell
		var left string
		if di := t.sideOff + i; di < len(t.dates) {
			d := t.dates[di]
			mark := " "
			if d == today() {
				mark = cGreen + iBullet + cReset
			}
			switch {
			case di == t.cursor && t.focus == 0:
				cell := " " + iDate + " " + d + " "
				left = cCursor + cAccent + cBold + mark + strings.ReplaceAll(cell, cReset, cReset+cCursor) + cReset
			case di == t.cursor:
				left = mark + cAccent + " " + iDate + " " + d + " " + cReset
			default:
				left = mark + "   " + cGrey + d + cReset + " "
			}
		} else {
			left = strings.Repeat(" ", sideW)
		}
		// content cell, with a focus gutter marking the selected entry
		right, gutter := "", "  "
		if ci := t.contentOff + i; ci < len(content) {
			right = content[ci].text
			if t.focus == 1 && content[ci].entry >= 0 && content[ci].entry == t.entryCur {
				gutter = cAccent + iBar + " " + cReset
			}
		}
		b.WriteString(left + cGrey + "│" + cReset + " " + gutter + right + "\r\n")
	}

	b.WriteString(fmt.Sprintf("\x1b[%d;1H", rows))
	switch {
	case t.in != nil && t.in.kind == 'a':
		b.WriteString(inputBar(cGreen+" + "+t.addTarget()+": "+cReset, t.in.buf))
	case t.in != nil && t.in.kind == 'e':
		b.WriteString(inputBar(cYellow+" "+iEdit+" "+cReset, t.in.buf))
	case t.in != nil && t.in.kind == '/':
		b.WriteString(inputBar(cAccent+" /"+cReset, t.in.buf))
	case t.in != nil && t.in.kind == ':':
		b.WriteString(inputBar(cYellow+" :"+cReset, t.in.buf))
	case t.pending == 'd':
		b.WriteString(cRed + " d… delete entry? press d again" + cReset)
	case t.status != "":
		b.WriteString(" " + t.status)
	case t.search != "":
		b.WriteString(cAccent + " " + t.searchLabel() + t.search + cReset +
			cGrey + "  n/N matching day · esc clear" + cReset)
	case t.focus == 1:
		help := " ↑↓ entry · ← dates"
		if t.cfg.edit != nil {
			help += " · e edit · dd del"
		}
		if t.cfg.add != nil {
			help += " · a add"
		}
		b.WriteString(cGrey + help + " · yy copy · / search · q quit" + cReset)
	default:
		help := " ↑↓ date · → open"
		if t.cfg.add != nil {
			help += " · a add"
		}
		b.WriteString(cGrey + help + " · yy copy · / search · t today · q quit" + cReset)
	}
	os.Stdout.WriteString(b.String())
}

// dayMatches reports whether a date matches the query under the current
// search mode: date name + summary for '/' and :ff, content for '/' and :fg.
func (t *browserTUI) dayMatches(d, q string) bool {
	if t.searchMode != 'c' && (containsFold(d, q) || containsFold(t.summaries[d], q)) {
		return true
	}
	if t.searchMode == 'd' {
		return false
	}
	for _, e := range t.cfg.entries(d) {
		if containsFold(e.ts+" "+e.text, q) {
			return true
		}
	}
	return false
}

// doDelete removes the focused content entry, if the source allows it.
func (t *browserTUI) doDelete() {
	if t.focus == 1 && t.cfg.del != nil && t.entryCur < len(t.entries) {
		t.cfg.del(t.dates[t.cursor], t.entryCur)
		t.reload()
		t.status = cRed + "deleted" + cReset
	}
}

// yank copies the focused row: the entry text in the content pane, or the date
// in the sidebar.
func (t *browserTUI) yank() {
	var s string
	if t.focus == 1 && t.entryCur < len(t.entries) {
		s = t.entries[t.entryCur].text
	} else if t.cursor < len(t.dates) {
		s = t.dates[t.cursor]
	}
	yank(&t.status, s)
}

func (t *browserTUI) findNext(dir int) {
	if t.search == "" || len(t.dates) == 0 {
		return
	}
	if i, ok := cycle(len(t.dates), t.cursor, dir, func(i int) bool {
		return t.dayMatches(t.dates[i], t.search)
	}); ok {
		t.selectDate(i)
		return
	}
	if t.dayMatches(t.dates[t.cursor], t.search) {
		return // only the current day matches
	}
	t.status = cGrey + "no day matches " + t.searchLabel() + t.search + cReset
}

// runCommand executes a ":" command: q quits, ff finds dates, fg greps.
func (t *browserTUI) runCommand(val string) {
	cmd, arg := splitCmd(val)
	switch cmd {
	case "q", "quit":
		t.quit = true
	case "ff", "fg":
		if arg == "" {
			t.search, t.searchMode = "", 0
			return
		}
		t.search, t.searchMode = arg, 'd'
		if cmd == "fg" {
			t.searchMode = 'c'
		}
		if len(t.dates) > 0 && !t.dayMatches(t.dates[t.cursor], arg) {
			t.findNext(1)
		}
	case "":
	default:
		t.status = cRed + "unknown command :" + cmd + cReset
	}
}

// liveFind jumps to the first matching date while a ":ff pat" is being typed.
func (t *browserTUI) liveFind() {
	if t.in == nil || !strings.HasPrefix(t.in.buf, "ff ") {
		return
	}
	pat := strings.TrimSpace(t.in.buf[3:])
	if pat == "" {
		return
	}
	for i, d := range t.dates {
		if containsFold(d, pat) {
			t.selectDate(i)
			return
		}
	}
}

// submit applies a finished line-editor entry.
func (t *browserTUI) submit(kind byte, val string) {
	switch kind {
	case 'a':
		if val == "" {
			return
		}
		wrote := t.cfg.add(t.addTarget(), val) // may differ from the selected day
		t.reload()
		for i, d := range t.dates {
			if d == wrote {
				t.selectDate(i)
			}
		}
		t.status = cGreen + "added to " + wrote + cReset
		if warnStale() && t.cfg.summaries != nil && cachedDate(wrote) {
			t.status += cGrey + " · stale summary — notie cache " + wrote + cReset
		}
	case 'e':
		if val == "" || t.cfg.edit == nil || t.entryCur >= len(t.entries) {
			return
		}
		t.cfg.edit(t.dates[t.cursor], t.entryCur, val)
		t.reload()
		t.status = cGreen + "updated" + cReset
	case '/':
		t.search, t.searchMode = val, 0
		if val != "" && len(t.dates) > 0 && !t.dayMatches(t.dates[t.cursor], val) {
			t.findNext(1)
		}
	case ':':
		t.runCommand(val)
	}
}

func runBrowser(cfg browserCfg) {
	t := &browserTUI{cfg: cfg}
	t.reload()
	runScreen(cfg.fallback, func(r *bufio.Reader) {
		for {
			t.render()
			c, err := readKey(r)
			if err != nil {
				return
			}

			if t.in != nil {
				switch t.in.key(c) {
				case "":
					if t.in.kind == ':' {
						t.liveFind()
					}
				case "cancel":
					t.in = nil
				case "submit":
					kind, val := t.in.kind, strings.TrimSpace(t.in.buf)
					t.in = nil
					t.submit(kind, val)
					if t.quit {
						return
					}
				}
				continue
			}

			t.status = ""
			if t.pending == 'g' {
				t.pending = 0
				if c == 'g' {
					if t.focus == 0 {
						t.selectDate(0)
					} else {
						t.entryCur, t.contentOff = 0, 0
					}
				}
				continue
			}
			if t.pending == 'd' {
				t.pending = 0
				if c == 'd' {
					t.doDelete()
				}
				continue
			}
			if t.pending == 'y' {
				t.pending = 0
				if c == 'y' {
					t.yank()
				}
				continue
			}

			switch c {
			case 'q', 3:
				return
			case 27:
				t.search, t.searchMode = "", 0
			case 'h':
				t.focus = 0
			case 'l':
				if len(t.entries) > 0 {
					t.focus = 1
				}
			case 'j':
				if t.focus == 0 {
					if t.cursor < len(t.dates)-1 {
						t.selectDate(t.cursor + 1)
					}
				} else if t.entryCur < len(t.entries)-1 {
					t.entryCur++
				}
			case 'k':
				if t.focus == 0 {
					if t.cursor > 0 {
						t.selectDate(t.cursor - 1)
					}
				} else if t.entryCur > 0 {
					t.entryCur--
				}
			case 'g':
				t.pending = 'g'
			case 'G':
				if t.focus == 0 {
					if len(t.dates) > 0 {
						t.selectDate(len(t.dates) - 1)
					}
				} else {
					t.entryCur = len(t.entries) - 1
				}
			case 'J':
				t.contentOff++ // clamped in render
			case 'K':
				if t.contentOff > 0 {
					t.contentOff--
				}
			case 4: // ctrl-d / PgDn
				rows, _ := termSize()
				t.contentOff += max(1, (rows-2)/2)
			case 21: // ctrl-u / PgUp
				rows, _ := termSize()
				t.contentOff = max(0, t.contentOff-max(1, (rows-2)/2))
			case 't':
				for i, d := range t.dates {
					if d == today() {
						t.selectDate(i)
					}
				}
			case 'a', 'o':
				if t.cfg.add != nil {
					t.in = &lineInput{kind: 'a'}
				}
			case 'e':
				if t.focus == 1 && t.cfg.edit != nil && t.entryCur < len(t.entries) {
					t.in = &lineInput{kind: 'e', buf: t.entries[t.entryCur].text}
				}
			case 'd':
				if t.focus == 1 && t.cfg.del != nil {
					if confirmDelete() {
						t.pending = 'd'
					} else {
						t.doDelete()
					}
				}
			case 'y':
				t.pending = 'y'
			case '/':
				t.in = &lineInput{kind: '/'}
			case ':':
				t.in = &lineInput{kind: ':'}
			case 'n':
				t.findNext(1)
			case 'N':
				t.findNext(-1)
			}
		}
	})
}
