// Shared TUI primitives: the raw-screen lifecycle, the footer line-editor, and
// the wrap-around search stepper used by the browser, task and notes TUIs.
package main

import (
	"bufio"
	"os"
	"os/exec"
	"strings"
)

// savedCooked holds the pre-raw terminal state captured by the active
// runScreen, so editFile can hand a cooked terminal to an external editor and
// restore raw mode afterward. Set only while a TUI screen is up; the TUIs are
// never nested, so a single global is enough.
var savedCooked termios

// runScreen runs loop inside the alternate screen with raw mode and a hidden
// cursor, restoring the terminal afterward. When raw mode is unavailable (no
// TTY) it calls fallback — the plain, non-interactive output path — instead.
func runScreen(fallback func(), loop func(*bufio.Reader)) {
	old, err := enterRaw()
	if err != nil {
		fallback()
		return
	}
	savedCooked = old
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

// editFile suspends the alternate-screen TUI — dropping back to the cooked
// terminal and the primary screen — runs $VISUAL/$EDITOR (falling back to vim)
// on path, then restores raw mode and the alternate screen when the editor
// exits. Meant to be called from inside a runScreen loop.
func editFile(path string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vim"
	}
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		fields = []string{"vim"}
	}
	os.Stdout.WriteString("\x1b[?1049l\x1b[?25h")
	restoreTerm(savedCooked)
	cmd := exec.Command(fields[0], append(fields[1:], path)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := cmd.Run()
	enterRaw()
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l")
	return err
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
