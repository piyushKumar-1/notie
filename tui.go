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
// search, command or edit. kind identifies the caller's mode ('a','/',':','e');
// seed buf to pre-fill it (an edit starts from the existing text).
type lineInput struct {
	kind byte
	buf  string
}

// key applies one keypress, returning "submit" on Enter, "cancel" on Esc, or
// "" while still editing (printable keys and backspace mutate buf in place).
func (li *lineInput) key(c byte) string {
	switch {
	case c == 13 || c == 10:
		return "submit"
	case c == 27:
		return "cancel"
	case c == 127 || c == 8:
		if len(li.buf) > 0 {
			li.buf = li.buf[:len(li.buf)-1]
		}
	case c >= 32 && c < 127:
		li.buf += string(c)
	}
	return ""
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
// block cursor.
func inputBar(prefix, buf string) string {
	return prefix + buf + cCursor + " " + cReset
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
