// A small in-TUI multi-line text editor. It backs editing a task's details
// without shelling out to $EDITOR: a full-screen modal inside the alternate
// screen, with a real hardware cursor, arrow-key navigation and hard wrapping.
//
// It is modal, vim-style: it opens in insert mode; Esc drops to normal mode
// where hjkl move, i/a/A/o/O start typing, x/dd delete and u undoes; Esc (or
// ^S) from normal mode saves and ^C discards. When ui.vim_mode is off it is a
// plain always-insert box where Esc saves — the original behavior.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type textEditor struct {
	title      string
	lines      []string // logical lines; always at least one
	row, col   int      // cursor position, col in runes
	top        int      // first visible display (wrapped) row
	width      int      // last render's wrap width, for display-aware up/down
	vim        bool     // modal editing on
	insertMode bool     // true: insert mode; false: normal mode
	keys       vimKeys
	pending    byte       // first key of a normal-mode chord (dd, gg)
	undo       []editSnap // undo stack, newest last
}

// editSnap is one point on the undo stack: a full copy of the text and cursor.
type editSnap struct {
	lines    []string
	row, col int
}

// editText runs the editor seeded with text and returns the edited text plus
// whether it was saved (vs discarded). It draws inside the current alternate
// screen and leaves the hardware cursor hidden for the caller.
func editText(r *bufio.Reader, title, text string) (string, bool) {
	e := &textEditor{title: title, lines: strings.Split(text, "\n"), keys: loadVimKeys(), vim: vimEnabled()}
	if len(e.lines) == 0 {
		e.lines = []string{""}
	}
	// Start at the end of the text — most edits append.
	e.row = len(e.lines) - 1
	e.col = runeLen(e.lines[e.row])
	if e.vim {
		e.enterInsert() // open in insert mode (also seeds the undo stack)
	} else {
		e.insertMode = true
	}
	defer os.Stdout.WriteString("\x1b[?25l\x1b[0 q") // re-hide cursor, reset its shape
	for {
		e.render()
		c, err := readEditKey(r)
		if err != nil {
			return text, false
		}
		switch c {
		case 3: // ctrl-c — discard, always
			return text, false
		case 19: // ctrl-s — save, always
			return strings.Join(e.lines, "\n"), true
		}
		if !e.vim {
			if c == 27 { // esc saves in the non-modal editor
				return strings.Join(e.lines, "\n"), true
			}
			e.handleInsert(c)
			continue
		}
		if e.insertMode {
			if c == 27 { // esc drops to normal mode
				e.insertMode = false
				e.col = min(e.col, runeLen(e.lines[e.row]))
				continue
			}
			e.handleInsert(c)
		} else if e.handleNormal(c) {
			return strings.Join(e.lines, "\n"), true
		}
	}
}

// handleInsert applies one key while typing. Esc is handled by the caller.
func (e *textEditor) handleInsert(c int) {
	switch c {
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
			e.insertRune(rune(c))
		}
	}
}

// handleNormal applies one normal-mode key and reports whether to save & exit.
func (e *textEditor) handleNormal(c int) (save bool) {
	k := e.keys
	if e.pending != 0 { // second key of a chord
		p := e.pending
		e.pending = 0
		switch {
		case p == k.delLine && c == int(k.delLine):
			e.pushUndo()
			e.deleteLine()
		case p == k.gotoTop && c == int(k.gotoTop):
			e.row, e.col = 0, 0
		}
		return false
	}
	switch c { // real navigation keys always move
	case 27, 19: // esc / ctrl-s — save
		return true
	case keyLeft:
		e.left()
		return false
	case keyRight:
		e.right()
		return false
	case keyUp:
		e.up()
		return false
	case keyDown:
		e.down()
		return false
	case keyHome:
		e.col = 0
		return false
	case keyEnd:
		e.col = runeLen(e.lines[e.row])
		return false
	case keyPgUp:
		for i := 0; i < 5; i++ {
			e.up()
		}
		return false
	case keyPgDn:
		for i := 0; i < 5; i++ {
			e.down()
		}
		return false
	case keyDel:
		e.pushUndo()
		e.delUnderCursor()
		return false
	}
	switch byte(c) {
	case k.left:
		e.left()
	case k.down:
		e.down()
	case k.up:
		e.up()
	case k.right:
		e.right()
	case k.lineStart:
		e.col = 0
	case k.lineEnd:
		e.col = runeLen(e.lines[e.row])
	case k.insert:
		e.enterInsert()
	case k.appendCh:
		if runeLen(e.lines[e.row]) > 0 {
			e.col = min(e.col+1, runeLen(e.lines[e.row]))
		}
		e.enterInsert()
	case k.appendEol:
		e.col = runeLen(e.lines[e.row])
		e.enterInsert()
	case k.openBelow:
		e.pushUndo()
		e.openLine(false)
		e.insertMode = true
	case k.openAbove:
		e.pushUndo()
		e.openLine(true)
		e.insertMode = true
	case k.delChar:
		e.pushUndo()
		e.delUnderCursor()
	case k.delLine, k.gotoTop:
		e.pending = byte(c)
	case k.gotoBottom:
		e.row, e.col = len(e.lines)-1, 0
	case k.undo:
		e.restore()
	}
	return false
}

// --- undo ---

// snap captures the current text and cursor.
func (e *textEditor) snap() editSnap {
	cp := make([]string, len(e.lines))
	copy(cp, e.lines)
	return editSnap{cp, e.row, e.col}
}

// pushUndo records the current state as an undo point, capped to a sane depth.
func (e *textEditor) pushUndo() {
	e.undo = append(e.undo, e.snap())
	if len(e.undo) > 200 {
		e.undo = e.undo[len(e.undo)-200:]
	}
}

// enterInsert snapshots the pre-edit state (so the whole insert session undoes
// as one step) and switches to insert mode.
func (e *textEditor) enterInsert() {
	e.pushUndo()
	e.insertMode = true
}

// restore pops the newest undo point back into the editor.
func (e *textEditor) restore() {
	if len(e.undo) == 0 {
		return
	}
	s := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.lines = s.lines
	e.row = min(s.row, len(e.lines)-1)
	e.col = min(s.col, runeLen(e.lines[e.row]))
}

// --- text mutation ---

func (e *textEditor) insertRune(ch rune) {
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

// openLine inserts a fresh blank line above or below the cursor and moves onto
// it — backing normal-mode O and o.
func (e *textEditor) openLine(above bool) {
	at := e.row
	if !above {
		at = e.row + 1
	}
	e.lines = append(e.lines[:at], append([]string{""}, e.lines[at:]...)...)
	e.row, e.col = at, 0
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

// delUnderCursor removes the char under the cursor without joining lines —
// normal-mode x. On an empty line it is a no-op.
func (e *textEditor) delUnderCursor() {
	r := []rune(e.lines[e.row])
	if len(r) == 0 {
		return
	}
	if e.col >= len(r) {
		e.col = len(r) - 1
	}
	e.lines[e.row] = string(append(r[:e.col], r[e.col+1:]...))
	e.col = min(e.col, max(0, runeLen(e.lines[e.row])-1))
}

// deleteLine removes the current logical line — normal-mode dd. The last
// remaining line is emptied rather than removed, so there is always one line.
func (e *textEditor) deleteLine() {
	if len(e.lines) == 1 {
		e.lines[0], e.col = "", 0
		return
	}
	e.lines = append(e.lines[:e.row], e.lines[e.row+1:]...)
	if e.row >= len(e.lines) {
		e.row = len(e.lines) - 1
	}
	e.col = min(e.col, runeLen(e.lines[e.row]))
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

// up and down move by one *display* (wrapped) row rather than a whole logical
// line, so the cursor glides smoothly through long, wrapped paragraphs and
// keeps its visual column across the move.
func (e *textEditor) up()   { e.moveVert(-1) }
func (e *textEditor) down() { e.moveVert(1) }

// editWidth is the wrap width in effect; it mirrors render's computation so a
// motion between renders uses the same wrapping.
func (e *textEditor) editWidth() int {
	if e.width >= 8 {
		return e.width
	}
	_, cols := termSize()
	return max(8, cols-1)
}

func (e *textEditor) moveVert(dir int) {
	segs := e.wrap(e.editWidth())
	cr, cc := e.cursorAt(segs)
	nr := cr + dir
	if nr < 0 || nr >= len(segs) {
		return
	}
	s := segs[nr]
	col := s.start + cc // keep the same visual column, clamped to the new row
	if col > s.end {
		col = s.end
	}
	e.row, e.col = s.li, col
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

// modeTag is the right-side title label and cursor shape for the current mode.
func (e *textEditor) modeTag() (label, shape string) {
	switch {
	case !e.vim:
		return "edit", "\x1b[6 q"
	case e.insertMode:
		return "insert", "\x1b[6 q" // bar cursor
	default:
		return "normal", "\x1b[2 q" // block cursor
	}
}

func (e *textEditor) render() {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	rows, cols := termSize()
	width := max(8, cols-1) // one-column left pad
	e.width = width         // remember it for display-aware up/down
	label, shape := e.modeTag()
	b.WriteString(titleBar(cols, iEdit+" "+e.title, label) + "\r\n")

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
	b.WriteString(cGrey + " " + e.helpLine() + cReset)

	// Place and reveal the real cursor last so it lands in the text, and give it
	// a shape that reflects the mode (bar in insert, block in normal).
	b.WriteString(fmt.Sprintf("\x1b[%d;%dH%s\x1b[?25h", 2+curRow-e.top, 2+curCol, shape))
	os.Stdout.WriteString(b.String())
}

// helpLine is the mode-aware footer legend.
func (e *textEditor) helpLine() string {
	if !e.vim {
		return "↵ newline · ←↑↓→ move · esc save · ^c discard"
	}
	if e.insertMode {
		return "typing · esc normal mode · ^s save · ^c discard"
	}
	return "i insert · a append · o open · x del · dd cut · u undo · esc save · ^c discard"
}
