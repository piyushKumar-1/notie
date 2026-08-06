// A small in-TUI multi-line text editor. It backs editing a task's details
// without shelling out to $EDITOR: a full-screen modal inside the alternate
// screen, with a real hardware cursor, arrow-key navigation and hard wrapping.
// Esc (or ^S) saves; ^C discards. Input is ASCII-printable, matching the rest
// of notie's raw-mode editors.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type textEditor struct {
	title    string
	lines    []string // logical lines; always at least one
	row, col int      // cursor position, col in runes
	top      int      // first visible display (wrapped) row
}

// editText runs the editor seeded with text and returns the edited text plus
// whether it was saved (vs discarded). It draws inside the current alternate
// screen and leaves the hardware cursor hidden for the caller.
func editText(r *bufio.Reader, title, text string) (string, bool) {
	e := &textEditor{title: title, lines: strings.Split(text, "\n")}
	if len(e.lines) == 0 {
		e.lines = []string{""}
	}
	// Start at the end of the text — most edits append.
	e.row = len(e.lines) - 1
	e.col = runeLen(e.lines[e.row])
	defer os.Stdout.WriteString("\x1b[?25l") // re-hide the cursor on the way out
	for {
		e.render()
		c, err := readEditKey(r)
		if err != nil {
			return text, false
		}
		switch c {
		case 3: // ctrl-c — discard
			return text, false
		case 27, 19: // esc / ctrl-s — save
			return strings.Join(e.lines, "\n"), true
		case 13, 10:
			e.newline()
		case 127, 8:
			e.backspace()
		case keyDel:
			e.del()
		case keyLeft:
			e.left()
		case keyRight:
			e.right()
		case keyUp:
			e.up()
		case keyDown:
			e.down()
		case keyHome:
			e.col = 0
		case keyEnd:
			e.col = runeLen(e.lines[e.row])
		case keyPgUp:
			for i := 0; i < 5; i++ {
				e.up()
			}
		case keyPgDn:
			for i := 0; i < 5; i++ {
				e.down()
			}
		default:
			if c >= 32 && c < 127 {
				e.insert(rune(c))
			}
		}
	}
}

func (e *textEditor) insert(ch rune) {
	r := []rune(e.lines[e.row])
	r = append(r[:e.col], append([]rune{ch}, r[e.col:]...)...)
	e.lines[e.row] = string(r)
	e.col++
}

// newline splits the current line at the cursor.
func (e *textEditor) newline() {
	r := []rune(e.lines[e.row])
	before, after := string(r[:e.col]), string(r[e.col:])
	e.lines[e.row] = before
	e.lines = append(e.lines[:e.row+1], append([]string{after}, e.lines[e.row+1:]...)...)
	e.row++
	e.col = 0
}

// backspace deletes the char before the cursor, joining lines at column 0.
func (e *textEditor) backspace() {
	if e.col > 0 {
		r := []rune(e.lines[e.row])
		e.lines[e.row] = string(append(r[:e.col-1], r[e.col:]...))
		e.col--
		return
	}
	if e.row == 0 {
		return
	}
	e.col = runeLen(e.lines[e.row-1])
	e.lines[e.row-1] += e.lines[e.row]
	e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
	e.row--
}

// del deletes the char under the cursor, joining the next line at line end.
func (e *textEditor) del() {
	r := []rune(e.lines[e.row])
	if e.col < len(r) {
		e.lines[e.row] = string(append(r[:e.col], r[e.col+1:]...))
		return
	}
	if e.row < len(e.lines)-1 {
		e.lines[e.row] += e.lines[e.row+1]
		e.lines = append(e.lines[:e.row+1], e.lines[e.row+2:]...)
	}
}

func (e *textEditor) left() {
	if e.col > 0 {
		e.col--
	} else if e.row > 0 {
		e.row--
		e.col = runeLen(e.lines[e.row])
	}
}

func (e *textEditor) right() {
	if e.col < runeLen(e.lines[e.row]) {
		e.col++
	} else if e.row < len(e.lines)-1 {
		e.row++
		e.col = 0
	}
}

func (e *textEditor) up() {
	if e.row > 0 {
		e.row--
		e.col = min(e.col, runeLen(e.lines[e.row]))
	}
}

func (e *textEditor) down() {
	if e.row < len(e.lines)-1 {
		e.row++
		e.col = min(e.col, runeLen(e.lines[e.row]))
	}
}

// seg is one hard-wrapped display row: a rune slice [start:end) of logical line li.
type seg struct{ li, start, end int }

// wrap hard-wraps every logical line into width-wide display rows (each line
// yields at least one row, so a blank line still shows).
func (e *textEditor) wrap(width int) []seg {
	var segs []seg
	for li, ln := range e.lines {
		r := []rune(ln)
		if len(r) == 0 {
			segs = append(segs, seg{li, 0, 0})
			continue
		}
		for start := 0; start < len(r); start += width {
			segs = append(segs, seg{li, start, min(start+width, len(r))})
		}
	}
	return segs
}

// cursorAt returns the display row and column of the logical cursor.
func (e *textEditor) cursorAt(segs []seg) (int, int) {
	lineLen := runeLen(e.lines[e.row])
	for i, s := range segs {
		if s.li != e.row {
			continue
		}
		last := i+1 >= len(segs) || segs[i+1].li != e.row
		if e.col >= s.start && (e.col < s.end || (last && e.col <= lineLen)) {
			return i, e.col - s.start
		}
	}
	return 0, 0
}

func (e *textEditor) render() {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	rows, cols := termSize()
	width := max(8, cols-1) // one-column left pad
	b.WriteString(titleBar(cols, iEdit+" "+e.title, "edit") + "\r\n")

	segs := e.wrap(width)
	curRow, curCol := e.cursorAt(segs)

	avail := max(1, rows-2)
	if curRow < e.top {
		e.top = curRow
	}
	if curRow >= e.top+avail {
		e.top = curRow - avail + 1
	}
	if e.top < 0 {
		e.top = 0
	}
	for i := e.top; i < len(segs) && i < e.top+avail; i++ {
		s := segs[i]
		b.WriteString(" " + string([]rune(e.lines[s.li])[s.start:s.end]) + "\r\n")
	}

	b.WriteString(fmt.Sprintf("\x1b[%d;1H", rows))
	b.WriteString(cGrey + " ↵ newline · ←↑↓→ move · esc save · ^c discard" + cReset)

	// Place and reveal the real cursor last so it lands in the text.
	b.WriteString(fmt.Sprintf("\x1b[%d;%dH\x1b[?25h", 2+curRow-e.top, 2+curCol))
	os.Stdout.WriteString(b.String())
}
