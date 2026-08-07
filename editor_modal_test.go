package main

import (
	"bufio"
	"strings"
	"testing"
)

// runEditor feeds a byte sequence to editText and returns the saved text. It
// isolates config so modal editing defaults on.
func runEditor(t *testing.T, seed string, keys ...byte) (string, bool) {
	t.Helper()
	t.Setenv("NOTIE_DIR", t.TempDir())
	muteStdout(t)
	r := bufio.NewReader(strings.NewReader(string(keys)))
	return editText(r, "task", seed)
}

// TestEditorNormalDelete: type, Esc to normal, x deletes the char under the
// cursor, Esc saves.
func TestEditorNormalDelete(t *testing.T) {
	out, ok := runEditor(t, "", 'h', 'e', 'l', 'l', 'o', 27, 'x', 27)
	if !ok || out != "hell" {
		t.Fatalf("got %q (saved=%v), want %q", out, ok, "hell")
	}
}

// TestEditorUndo: an x delete is undone by u, restoring the text.
func TestEditorUndo(t *testing.T) {
	out, ok := runEditor(t, "", 'h', 'e', 'l', 'l', 'o', 27, 'x', 'u', 27)
	if !ok || out != "hello" {
		t.Fatalf("got %q (saved=%v), want %q", out, ok, "hello")
	}
}

// TestEditorOpenLine: o opens a line below and drops into insert mode.
func TestEditorOpenLine(t *testing.T) {
	out, ok := runEditor(t, "a", 27, 'o', 'b', 27, 27)
	if !ok || out != "a\nb" {
		t.Fatalf("got %q (saved=%v), want %q", out, ok, "a\nb")
	}
}

// TestEditorInsertUndoWholeSession: an entire insert session undoes as one step
// back to the pre-insert text.
func TestEditorInsertUndoWholeSession(t *testing.T) {
	// seed "hi"; Esc→normal; A appends at EOL and types " there"; Esc→normal; u
	out, ok := runEditor(t, "hi", 27, 'A', ' ', 't', 'h', 'e', 'r', 'e', 27, 'u', 27)
	if !ok || out != "hi" {
		t.Fatalf("got %q (saved=%v), want %q", out, ok, "hi")
	}
}

// TestLineInputModalEditing exercises the footer input's normal-mode motions,
// delete and undo.
func TestLineInputModalEditing(t *testing.T) {
	t.Setenv("NOTIE_DIR", t.TempDir())

	li := newLineInput('e', "hello")
	li.key(27)       // → normal mode
	li.key(int('0')) // to line start
	li.key(int('x')) // delete 'h'
	if li.buf != "ello" {
		t.Fatalf("after x: buf=%q, want %q", li.buf, "ello")
	}
	li.key(int('u')) // undo
	if li.buf != "hello" {
		t.Fatalf("after u: buf=%q, want %q", li.buf, "hello")
	}
}

// TestLineInputArrowMoves confirms a real Left arrow (keyLeft) moves the cursor
// instead of typing a letter — the bug where arrows inserted h/j/k/l.
func TestLineInputArrowMoves(t *testing.T) {
	t.Setenv("NOTIE_DIR", t.TempDir())

	li := newLineInput('a', "ab") // insert mode, cursor at end
	li.key(keyLeft)               // move between a and b
	li.key(int('Z'))              // insert here
	if li.buf != "aZb" {
		t.Fatalf("arrow+insert: buf=%q, want %q", li.buf, "aZb")
	}
}
