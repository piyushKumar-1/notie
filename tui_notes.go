// Interactive list for important.md / remember.md: same vim keys as tasks.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var noteLineRe = regexp.MustCompile(`^- (\d{4}-\d{2}-\d{2}) (\d{2}:\d{2}) — (.*)$`)

type notesCfg struct {
	file, header, label, icon, iconColor string
}

var rememberCfg = notesCfg{"remember.md", "Remember", "remember", iDiamond, cMagenta}

type notesTUI struct {
	cfg     notesCfg
	lines   []string
	notes   []int // indices into lines that are note lines
	cursor  int
	offset  int
	pending byte
	input   *string
	inputCh byte
	search  string
	status  string
}

func (t *notesTUI) path() string { return notieDir() + "/" + t.cfg.file }

func (t *notesTUI) reload() {
	t.lines = readLines(t.path())
	t.notes = t.notes[:0]
	for i, l := range t.lines {
		if strings.HasPrefix(l, "- ") {
			t.notes = append(t.notes, i)
		}
	}
	if t.cursor >= len(t.notes) {
		t.cursor = len(t.notes) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
}

func (t *notesTUI) row(l string, selected bool, cols int) (string, int) {
	icon := t.cfg.iconColor + t.cfg.icon + cReset
	marker := "  "
	if selected {
		marker = cAccent + cBold + iCursor + cReset + " "
	}
	m := noteLineRe.FindStringSubmatch(l)
	if m == nil {
		text := truncRunes(strings.TrimPrefix(l, "- "), max(4, cols-4))
		return marker + icon + " " + highlight(text, t.search, ""), 4 + runeLen(text)
	}
	meta := m[1] + " " + m[2]
	fixed := 2 + 2 + runeLen(meta) + 3
	text := truncRunes(m[3], max(4, cols-fixed))
	styled := marker + icon + " " + cGrey + meta + cReset + " · " +
		highlight(text, t.search, "")
	return styled, fixed + runeLen(text)
}

func (t *notesTUI) render() {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	rows, cols := termSize()
	b.WriteString(titleBar(cols, t.cfg.icon+" notie · "+t.cfg.label,
		fmt.Sprintf("%d notes", len(t.notes))) + "\r\n")

	avail := max(1, rows-2)
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+avail {
		t.offset = t.cursor - avail + 1
	}
	if len(t.notes) == 0 {
		b.WriteString("\r\n  " + cGrey + "nothing here yet — press " + cReset +
			t.cfg.iconColor + "a" + cReset + cGrey + " to add" + cReset + "\r\n")
	}
	for vi := t.offset; vi < len(t.notes) && vi < t.offset+avail; vi++ {
		styled, w := t.row(t.lines[t.notes[vi]], vi == t.cursor, cols)
		if vi == t.cursor {
			b.WriteString(cursorRow(styled, w, cols) + "\r\n")
		} else {
			b.WriteString(styled + "\r\n")
		}
	}

	b.WriteString(fmt.Sprintf("\x1b[%d;1H", rows))
	switch {
	case t.input != nil && t.inputCh == 'a':
		b.WriteString(t.cfg.iconColor + " + " + cReset + *t.input + cCursor + " " + cReset)
	case t.input != nil && t.inputCh == '/':
		b.WriteString(cAccent + " /" + cReset + *t.input + cCursor + " " + cReset)
	case t.input != nil && t.inputCh == ':':
		b.WriteString(cYellow + " :" + cReset + *t.input + cCursor + " " + cReset)
	case t.pending == 'd':
		b.WriteString(cRed + " d… delete? press d again" + cReset)
	case t.status != "":
		b.WriteString(" " + t.status)
	case t.search != "":
		b.WriteString(cAccent + " /" + t.search + cReset + cGrey + "  n/N next/prev · esc clear" + cReset)
	default:
		b.WriteString(cGrey + " j/k move · a add · dd delete · / search · :fg grep · q/:q quit" + cReset)
	}
	os.Stdout.WriteString(b.String())
}

func (t *notesTUI) findNext(dir int) {
	if t.search == "" || len(t.notes) == 0 {
		return
	}
	n := len(t.notes)
	for step := 1; step <= n; step++ {
		vi := ((t.cursor+dir*step)%n + n) % n
		if containsFold(t.lines[t.notes[vi]], t.search) {
			t.cursor = vi
			return
		}
	}
	t.status = cGrey + "no match for /" + t.search + cReset
}

func runNotesTUI(cfg notesCfg) {
	old, err := enterRaw()
	if err != nil {
		printFile(notieDir()+"/"+cfg.file, "empty")
		return
	}
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l")
	defer func() {
		os.Stdout.WriteString("\x1b[?1049l\x1b[?25h")
		restoreTerm(old)
	}()

	t := &notesTUI{cfg: cfg}
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
				if kind == 'a' && val != "" {
					appendLine(t.path(), t.cfg.header,
						fmt.Sprintf("- %s %s — %s", today(), clock(), val))
					t.reload()
					t.cursor = len(t.notes) - 1
					t.status = cGreen + "added" + cReset
				} else if kind == '/' {
					t.search = val
					t.findNext(1)
				} else if kind == ':' {
					switch cmd, arg := splitCmd(val); cmd {
					case "q", "quit":
						return
					case "ff", "fg":
						t.search = arg
						t.findNext(1)
					case "":
					default:
						t.status = cRed + "unknown command :" + cmd + cReset
					}
				}
			case c == 27:
				t.input = nil
			case c == 127 || c == 8:
				if len(*t.input) > 0 {
					*t.input = (*t.input)[:len(*t.input)-1]
				}
			case c >= 32 && c < 127:
				*t.input += string(c)
			}
			continue
		}

		t.status = ""
		if t.pending == 'd' {
			t.pending = 0
			if c == 'd' && len(t.notes) > 0 {
				i := t.notes[t.cursor]
				t.lines = append(t.lines[:i], t.lines[i+1:]...)
				writeLines(t.path(), t.lines)
				t.reload()
				t.status = cRed + "deleted" + cReset
			}
			continue
		}
		if t.pending == 'g' {
			t.pending = 0
			if c == 'g' {
				t.cursor = 0
			}
			continue
		}

		switch c {
		case 'q', 3:
			return
		case 27:
			t.search = ""
		case 'j':
			if t.cursor < len(t.notes)-1 {
				t.cursor++
			}
		case 'k':
			if t.cursor > 0 {
				t.cursor--
			}
		case 'g':
			t.pending = 'g'
		case 'G':
			if len(t.notes) > 0 {
				t.cursor = len(t.notes) - 1
			}
		case 'd':
			t.pending = 'd'
		case 'a', 'o':
			s := ""
			t.input, t.inputCh = &s, 'a'
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
