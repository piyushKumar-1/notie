// Shared TUI primitives: the raw-screen lifecycle, the footer line-editor, and
// the wrap-around search stepper used by the browser, task and notes TUIs.
package main

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
)

// runScreen runs loop inside the alternate screen with raw mode and a hidden
// cursor, restoring the terminal afterward. When raw mode is unavailable (no
// TTY) it calls fallback — the plain, non-interactive output path — instead.
func runScreen(fallback func(), loop func(*bufio.Reader)) {
	old, err := enterRaw()
	if err != nil {
		fallback()
		return
	}
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l")
	defer func() {
		os.Stdout.WriteString("\x1b[?1049l\x1b[?25h")
		restoreTerm(old)
	}()
	loop(bufio.NewReader(os.Stdin))
}

// lineInput is the one-line editor shown in the footer while typing an add,
// search, command or edit. kind identifies the caller's mode ('a','/',':','e').
// It is modal, matching the multi-line editor: it opens in insert mode; Esc
// drops to normal mode (hjkl move, i/a start typing, x deletes, u undoes) and a
// second Esc cancels. When ui.vim_mode is off it is a plain box where Esc
// cancels. Callers read it back through buf; feed it keys via readEditKey so
// real arrow keys move the cursor instead of being typed as hjkl.
type lineInput struct {
	kind       byte
	buf        string
	pos        int // rune cursor position within buf
	vim        bool
	insertMode bool
	pending    byte
	keys       vimKeys
	undo       []string // undo stack of prior buffers, newest last
}

// newLineInput builds a footer editor of the given kind, pre-filled with buf
// and ready to type (insert mode, cursor at the end).
func newLineInput(kind byte, buf string) *lineInput {
	li := &lineInput{kind: kind, buf: buf, pos: runeLen(buf), keys: loadVimKeys(), vim: vimEnabled(), insertMode: true}
	li.undo = append(li.undo, buf) // seed so u can restore the starting text
	return li
}

// key applies one keypress (from readEditKey, so nav keys arrive as key* codes),
// returning "submit" on Enter, "cancel" on Esc (from normal mode, or any mode
// when vim is off), or "" while still editing.
func (li *lineInput) key(c int) string {
	switch c { // real navigation keys always move, in either mode
	case keyLeft:
		if li.pos > 0 {
			li.pos--
		}
		return ""
	case keyRight:
		if li.pos < runeLen(li.buf) {
			li.pos++
		}
		return ""
	case keyHome:
		li.pos = 0
		return ""
	case keyEnd:
		li.pos = runeLen(li.buf)
		return ""
	case keyDel:
		li.delUnder()
		return ""
	case keyUp, keyDown, keyPgUp, keyPgDn:
		return "" // single line — nothing to move to
	}
	if !li.vim || li.insertMode {
		return li.insertKey(c)
	}
	return li.normalKey(byte(c))
}

// insertKey handles a key while typing.
func (li *lineInput) insertKey(c int) string {
	switch {
	case c == 13 || c == 10:
		return "submit"
	case c == 27:
		if li.vim { // esc drops to normal mode
			li.insertMode = false
			li.pos = min(li.pos, runeLen(li.buf))
			return ""
		}
		return "cancel"
	case c == 127 || c == 8:
		li.backspace()
	case c >= 32 && c < 127:
		li.insertRune(rune(c))
	}
	return ""
}

// normalKey handles a key in normal mode.
func (li *lineInput) normalKey(c byte) string {
	if c == 13 || c == 10 {
		return "submit"
	}
	if c == 27 {
		return "cancel"
	}
	k := li.keys
	switch c {
	case k.left:
		if li.pos > 0 {
			li.pos--
		}
	case k.right:
		if li.pos < runeLen(li.buf) {
			li.pos++
		}
	case k.lineStart:
		li.pos = 0
	case k.lineEnd:
		li.pos = runeLen(li.buf)
	case k.insert:
		li.enterInsert()
	case k.appendCh:
		if li.pos < runeLen(li.buf) {
			li.pos++
		}
		li.enterInsert()
	case k.appendEol:
		li.pos = runeLen(li.buf)
		li.enterInsert()
	case k.delChar:
		li.pushUndo()
		li.delUnder()
	case k.undo:
		li.restore()
	}
	return ""
}

func (li *lineInput) enterInsert() {
	li.pushUndo()
	li.insertMode = true
}

func (li *lineInput) pushUndo() {
	li.undo = append(li.undo, li.buf)
	if len(li.undo) > 100 {
		li.undo = li.undo[len(li.undo)-100:]
	}
}

func (li *lineInput) restore() {
	if len(li.undo) == 0 {
		return
	}
	li.buf = li.undo[len(li.undo)-1]
	li.undo = li.undo[:len(li.undo)-1]
	li.pos = min(li.pos, runeLen(li.buf))
}

func (li *lineInput) insertRune(ch rune) {
	r := []rune(li.buf)
	r = append(r[:li.pos], append([]rune{ch}, r[li.pos:]...)...)
	li.buf = string(r)
	li.pos++
}

func (li *lineInput) backspace() {
	if li.pos == 0 {
		return
	}
	r := []rune(li.buf)
	li.buf = string(append(r[:li.pos-1], r[li.pos:]...))
	li.pos--
}

func (li *lineInput) delUnder() {
	r := []rune(li.buf)
	if li.pos >= len(r) {
		return
	}
	li.buf = string(append(r[:li.pos], r[li.pos+1:]...))
}

// cycle steps from cur through all n positions in direction dir (+1 / -1),
// returning the first index whose match is true. ok is false if none match.
func cycle(n, cur, dir int, match func(int) bool) (int, bool) {
	for step := 1; step <= n; step++ {
		i := ((cur+dir*step)%n + n) % n
		if match(i) {
			return i, true
		}
	}
	return cur, false
}

// inputBar renders the footer editor: a styled prefix, the typed buffer, and a
// block cursor sitting at the input's cursor position.
func inputBar(prefix string, li *lineInput) string {
	r := []rune(li.buf)
	p := li.pos
	if p > len(r) {
		p = len(r)
	}
	under, after := " ", ""
	if p < len(r) {
		under, after = string(r[p]), string(r[p+1:])
	}
	return prefix + string(r[:p]) + caretStyle() + under + cReset + after
}

// copyClipboard puts s on the macOS clipboard via pbcopy.
func copyClipboard(s string) error {
	cmd := exec.Command("pbcopy")
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

// yank copies s to the clipboard and writes a themed result into status. The
// shared behavior behind `yy` in every TUI.
func yank(status *string, s string) {
	if s == "" {
		return
	}
	if err := copyClipboard(s); err != nil {
		*status = cRed + "copy failed" + cReset
	} else {
		*status = cGreen + "copied" + cReset
	}
}
