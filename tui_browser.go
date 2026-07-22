// Day browser: dates sidebar on the left, the selected day's entries on the
// right. Backs `notie journal`, `notie shell` and `notie important`.
// / searches dates AND content · :ff finds dates (live) · :fg greps content ·
// n/N jump between matching days · q or :q quits.
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

var browserEntryRe = regexp.MustCompile(`^- (\d{2}:\d{2}) (.*)$`)

// browserCfg adapts the day browser to one data source.
type browserCfg struct {
	label, icon string
	dates       func() []string          // days that have content (any order)
	dayLines    func(d string) []string  // that day's raw "- HH:MM ..." lines
	add         func(text string)        // nil disables 'a'
	summaries   func() map[string]string // nil: no per-day summary line
	fallback    func()                   // plain output when raw mode fails
	empty       string                   // message when there are no days
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
		dayLines: func(d string) []string {
			return readLines(filepath.Join(notieDir(), d, "journal.md"))
		},
		add: func(text string) {
			appendLine(filepath.Join(notieDir(), today(), "journal.md"),
				"Journal — "+today(), fmt.Sprintf("- %s — %s", clock(), text))
		},
		summaries: func() map[string]string {
			m := map[string]string{}
			for _, l := range readLines(filepath.Join(notieDir(), "datecache.md")) {
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
		dayLines: func(d string) []string {
			return readLines(filepath.Join(notieDir(), d, "shell.md"))
		},
		fallback: func() { cmdShowShell("") },
		empty:    "no shell history yet — run some commands in a terminal",
	}
}

// importantBrowser groups important.md's dated lines into per-day views.
func importantBrowser() browserCfg {
	path := filepath.Join(notieDir(), "important.md")
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
		dayLines: func(d string) []string {
			var out []string
			for _, l := range readLines(path) {
				if m := noteLineRe.FindStringSubmatch(l); m != nil && m[1] == d {
					out = append(out, "- "+m[2]+" — "+m[3])
				}
			}
			return out
		},
		add: func(text string) {
			appendLine(path, "Important", fmt.Sprintf("- %s %s — %s", today(), clock(), text))
		},
		fallback: func() { cmdShow("important") },
		empty:    "nothing important yet — press a to add",
	}
}

type browserTUI struct {
	cfg        browserCfg
	dates      []string // newest first
	summaries  map[string]string
	cursor     int
	sideOff    int
	contentOff int
	pending    byte
	input      *string
	inputCh    byte // 'a', '/' or ':'
	search     string
	searchMode byte // 0: dates+content ('/') · 'd': dates (:ff) · 'c': content (:fg)
	status     string
	quit       bool
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
}

// content builds the right pane for the selected date.
func (t *browserTUI) content(width int) []string {
	if len(t.dates) == 0 {
		return nil
	}
	d := t.dates[t.cursor]
	var out []string
	if day, err := time.Parse("2006-01-02", d); err == nil {
		out = append(out, cBold+cAccent+day.Format("Monday, 02 January 2006")+cReset)
	} else {
		out = append(out, cBold+cAccent+d+cReset)
	}
	if s, ok := t.summaries[d]; ok {
		for _, w := range wrapRunes(s, width) {
			out = append(out, cGrey+cItalic+highlight(w, t.search, cGrey+cItalic)+cReset)
		}
	}
	out = append(out, "")
	for _, l := range t.cfg.dayLines(d) {
		if strings.HasPrefix(l, "# ") || strings.TrimSpace(l) == "" {
			continue
		}
		if m := browserEntryRe.FindStringSubmatch(l); m != nil {
			ts, text := m[1], strings.TrimPrefix(m[2], "— ")
			indent := runeLen(ts) + 3
			for i, w := range wrapRunes(text, max(4, width-indent)) {
				if i == 0 {
					out = append(out, cAccent+iBullet+cReset+" "+cGrey+ts+cReset+" "+
						highlight(w, t.search, ""))
				} else {
					out = append(out, strings.Repeat(" ", indent)+highlight(w, t.search, ""))
				}
			}
		} else {
			for _, w := range wrapRunes(l, width) {
				out = append(out, cGrey+highlight(w, t.search, cGrey)+cReset)
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
	content := t.content(paneW)
	maxOff := max(0, len(content)-avail)
	if t.contentOff > maxOff {
		t.contentOff = maxOff
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
			if di == t.cursor {
				cell := " " + iDate + " " + d + " "
				left = cCursor + cAccent + cBold + mark + strings.ReplaceAll(cell, cReset, cReset+cCursor) + cReset
			} else {
				left = mark + "   " + cGrey + d + cReset + " "
			}
		} else {
			left = strings.Repeat(" ", sideW)
		}
		// content cell
		right := ""
		if ci := t.contentOff + i; ci < len(content) {
			right = content[ci]
		}
		b.WriteString(left + cGrey + "│" + cReset + " " + right + "\r\n")
	}

	b.WriteString(fmt.Sprintf("\x1b[%d;1H", rows))
	switch {
	case t.input != nil && t.inputCh == 'a':
		b.WriteString(cGreen + " + today: " + cReset + *t.input + cCursor + " " + cReset)
	case t.input != nil && t.inputCh == '/':
		b.WriteString(cAccent + " /" + cReset + *t.input + cCursor + " " + cReset)
	case t.input != nil && t.inputCh == ':':
		b.WriteString(cYellow + " :" + cReset + *t.input + cCursor + " " + cReset)
	case t.status != "":
		b.WriteString(" " + t.status)
	case t.search != "":
		b.WriteString(cAccent + " " + t.searchLabel() + t.search + cReset +
			cGrey + "  n/N matching day · esc clear" + cReset)
	default:
		help := " j/k days · J/K scroll · / search · :ff date · :fg grep"
		if t.cfg.add != nil {
			help += " · a add"
		}
		help += " · t today · q/:q quit"
		b.WriteString(cGrey + help + cReset)
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
	for _, l := range t.cfg.dayLines(d) {
		if containsFold(l, q) {
			return true
		}
	}
	return false
}

func (t *browserTUI) findNext(dir int) {
	if t.search == "" || len(t.dates) == 0 {
		return
	}
	n := len(t.dates)
	for step := 1; step <= n; step++ {
		di := ((t.cursor+dir*step)%n + n) % n
		if t.dayMatches(t.dates[di], t.search) {
			t.cursor, t.contentOff = di, 0
			return
		}
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
	if !strings.HasPrefix(*t.input, "ff ") {
		return
	}
	pat := strings.TrimSpace((*t.input)[3:])
	if pat == "" {
		return
	}
	for i, d := range t.dates {
		if containsFold(d, pat) {
			t.cursor, t.contentOff = i, 0
			return
		}
	}
}

func runBrowser(cfg browserCfg) {
	old, err := enterRaw()
	if err != nil {
		cfg.fallback()
		return
	}
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l")
	defer func() {
		os.Stdout.WriteString("\x1b[?1049l\x1b[?25h")
		restoreTerm(old)
	}()

	t := &browserTUI{cfg: cfg}
	t.reload()
	r := bufio.NewReader(os.Stdin)
	for {
		t.render()
		c, err := readKey(r)
		if err != nil {
			return
		}

		if t.input != nil {
			switch {
			case c == 13 || c == 10:
				val := strings.TrimSpace(*t.input)
				kind := t.inputCh
				t.input = nil
				switch {
				case kind == 'a' && val != "":
					t.cfg.add(val)
					t.reload()
					for i, d := range t.dates {
						if d == today() {
							t.cursor, t.contentOff = i, 0
						}
					}
					t.status = cGreen + "added to today" + cReset
				case kind == '/':
					t.search, t.searchMode = val, 0
					if val != "" && len(t.dates) > 0 && !t.dayMatches(t.dates[t.cursor], val) {
						t.findNext(1)
					}
				case kind == ':':
					t.runCommand(val)
					if t.quit {
						return
					}
				}
			case c == 27:
				t.input = nil
			case c == 127 || c == 8:
				if len(*t.input) > 0 {
					*t.input = (*t.input)[:len(*t.input)-1]
					if t.inputCh == ':' {
						t.liveFind()
					}
				}
			case c >= 32 && c < 127:
				*t.input += string(c)
				if t.inputCh == ':' {
					t.liveFind()
				}
			}
			continue
		}

		t.status = ""
		if t.pending == 'g' {
			t.pending = 0
			if c == 'g' {
				t.cursor, t.contentOff = 0, 0
			}
			continue
		}

		switch c {
		case 'q', 3:
			return
		case 27:
			t.search, t.searchMode = "", 0
		case 'j':
			if t.cursor < len(t.dates)-1 {
				t.cursor++
				t.contentOff = 0
			}
		case 'k':
			if t.cursor > 0 {
				t.cursor--
				t.contentOff = 0
			}
		case 'g':
			t.pending = 'g'
		case 'G':
			if len(t.dates) > 0 {
				t.cursor, t.contentOff = len(t.dates)-1, 0
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
					t.cursor, t.contentOff = i, 0
				}
			}
		case 'a', 'o':
			if t.cfg.add != nil {
				s := ""
				t.input, t.inputCh = &s, 'a'
			}
		case '/':
			s := ""
			t.input, t.inputCh = &s, '/'
		case ':':
			s := ""
			t.input, t.inputCh = &s, ':'
		case 'n':
			t.findNext(1)
		case 'N':
			t.findNext(-1)
		}
	}
}
