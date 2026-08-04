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
	in      *lineInput
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
	case t.in != nil && t.in.kind == 'a':
		b.WriteString(inputBar(t.cfg.iconColor+" + "+cReset, t.in.buf))
	case t.in != nil && t.in.kind == 'e':
		b.WriteString(inputBar(cYellow+" "+iEdit+" "+cReset, t.in.buf))
	case t.in != nil && t.in.kind == '/':
		b.WriteString(inputBar(cAccent+" /"+cReset, t.in.buf))
	case t.in != nil && t.in.kind == ':':
		b.WriteString(inputBar(cYellow+" :"+cReset, t.in.buf))
	case t.pending == 'd':
		b.WriteString(cRed + " d… delete? press d again" + cReset)
	case t.status != "":
		b.WriteString(" " + t.status)
	case t.search != "":
		b.WriteString(cAccent + " /" + t.search + cReset + cGrey + "  n/N next/prev · esc clear" + cReset)
	default:
		b.WriteString(cGrey + " j/k move · a add · e edit · dd del · yy copy · / search · q/:q quit" + cReset)
	}
	os.Stdout.WriteString(b.String())
}

func (t *notesTUI) findNext(dir int) {
	if t.search == "" || len(t.notes) == 0 {
		return
	}
	if i, ok := cycle(len(t.notes), t.cursor, dir, func(i int) bool {
		return containsFold(t.lines[t.notes[i]], t.search)
	}); ok {
		t.cursor = i
		return
	}
	t.status = cGrey + "no match for /" + t.search + cReset
}

// deleteNote removes the selected note.
func (t *notesTUI) deleteNote() {
	if len(t.notes) == 0 {
		return
	}
	i := t.notes[t.cursor]
	t.lines = append(t.lines[:i], t.lines[i+1:]...)
	writeLines(t.path(), t.lines)
	t.reload()
	t.status = cRed + "deleted" + cReset
}

// yank copies the selected note's text to the clipboard.
func (t *notesTUI) yank() {
	if len(t.notes) == 0 {
		return
	}
	l := t.lines[t.notes[t.cursor]]
	if m := noteLineRe.FindStringSubmatch(l); m != nil {
		l = m[3]
	}
	yank(&t.status, l)
}

// editNote rewrites the selected note's text, preserving its date and time.
func (t *notesTUI) editNote(text string) {
	if len(t.notes) == 0 || text == "" {
		return
	}
	i := t.notes[t.cursor]
	m := noteLineRe.FindStringSubmatch(t.lines[i])
	if m == nil {
		return
	}
	t.lines[i] = "- " + m[1] + " " + m[2] + " — " + text
	writeLines(t.path(), t.lines)
	t.reload()
	t.status = cGreen + "updated" + cReset
}

func runNotesTUI(cfg notesCfg) {
	t := &notesTUI{cfg: cfg}
	t.reload()
	runScreen(func() { printFile(notieDir()+"/"+cfg.file, "empty") }, func(r *bufio.Reader) {
		for {
			t.render()
			c, err := readKey(r)
			if err != nil {
				return
			}

			if t.in != nil {
				switch t.in.key(c) {
				case "cancel":
					t.in = nil
				case "submit":
					kind, val := t.in.kind, strings.TrimSpace(t.in.buf)
					t.in = nil
					switch kind {
					case 'a':
						if val != "" {
							appendLine(t.path(), t.cfg.header,
								fmt.Sprintf("- %s %s — %s", today(), clock(), val))
							t.reload()
							t.cursor = len(t.notes) - 1
							t.status = cGreen + "added" + cReset
						}
					case 'e':
						t.editNote(val)
					case '/':
						t.search = val
						t.findNext(1)
					case ':':
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
				}
				continue
			}

			t.status = ""
			if t.pending == 'd' {
				t.pending = 0
				if c == 'd' {
					t.deleteNote()
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
				if confirmDelete() {
					t.pending = 'd'
				} else {
					t.deleteNote()
				}
			case 'y':
				t.pending = 'y'
			case 'e':
				if len(t.notes) > 0 {
					if m := noteLineRe.FindStringSubmatch(t.lines[t.notes[t.cursor]]); m != nil {
						t.in = &lineInput{kind: 'e', buf: m[3]}
					}
				}
			case 'a', 'o':
				t.in = &lineInput{kind: 'a'}
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
