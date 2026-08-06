package main

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// muteStdout redirects os.Stdout to /dev/null for the rest of the test, so the
// editor's raw screen escapes don't pollute `go test` output.
func muteStdout(t *testing.T) {
	t.Helper()
	old := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	t.Cleanup(func() { os.Stdout = old; devnull.Close() })
}

// TestTaskDetailStorage covers the per-id description file: write, read, the
// has-detail check, blank-write removal, and explicit removal.
func TestTaskDetailStorage(t *testing.T) {
	t.Setenv("NOTIE_DIR", t.TempDir())

	if hasTaskDetail("7") {
		t.Fatal("fresh task should have no detail")
	}
	writeTaskDetail("7", "pointers:\n- a\n- b")
	if got := readTaskDetail("7"); got != "pointers:\n- a\n- b" {
		t.Fatalf("readTaskDetail = %q", got)
	}
	if !hasTaskDetail("7") {
		t.Fatal("hasTaskDetail should be true after write")
	}
	// A blank write removes the description rather than leaving an empty file.
	writeTaskDetail("7", "   \n  ")
	if hasTaskDetail("7") {
		t.Fatal("blank write should remove the detail")
	}
	writeTaskDetail("7", "back again")
	removeTaskDetail("7")
	if readTaskDetail("7") != "" {
		t.Fatal("removeTaskDetail left content behind")
	}
}

// TestSplitDetailFlag checks the "-d" tail is peeled off the add args.
func TestSplitDetailFlag(t *testing.T) {
	text, detail, has := splitDetailFlag([]string{"buy", "milk", "-d", "2%", "organic"})
	if text != "buy milk" || detail != "2% organic" || !has {
		t.Fatalf("split = %q / %q / %v", text, detail, has)
	}
	text, detail, has = splitDetailFlag([]string{"buy", "milk"})
	if text != "buy milk" || detail != "" || has {
		t.Fatalf("no-flag split = %q / %q / %v", text, detail, has)
	}
}

// TestRowDetailMarker checks a task with a description gets the ⋮ marker in the
// list row, and one without does not.
func TestRowDetailMarker(t *testing.T) {
	t.Setenv("NOTIE_DIR", t.TempDir())
	writeTaskDetail("12", "some details")

	tt := &taskTUI{}
	with, _ := tt.row("- [ ] #12 !1 buy milk (added 2026-08-06)", false, 80)
	if !strings.Contains(with, iNote) {
		t.Fatalf("row with detail missing %q marker: %q", iNote, with)
	}
	without, _ := tt.row("- [ ] #13 !1 no notes (added 2026-08-06)", false, 80)
	if strings.Contains(without, iNote) {
		t.Fatalf("row without detail should not show %q: %q", iNote, without)
	}
}

// TestEditTextTypingAndSave drives the in-TUI editor through readEditKey:
// typing, a newline, a backspace, then Esc to save. It confirms editText
// returns the assembled multi-line text.
func TestEditTextTypingAndSave(t *testing.T) {
	// "ab", Enter, "c", Backspace, "de", Esc  ->  "ab\nde"
	muteStdout(t)
	keys := []byte{'a', 'b', '\r', 'c', 127, 'd', 'e', 27}
	r := bufio.NewReader(strings.NewReader(string(keys)))

	out, ok := editText(r, "task", "")
	if !ok {
		t.Fatal("editText reported discard, expected save")
	}
	if out != "ab\nde" {
		t.Fatalf("editText = %q, want %q", out, "ab\nde")
	}
}

// TestEditTextDiscard checks ^C returns the original seed unchanged.
func TestEditTextDiscard(t *testing.T) {
	muteStdout(t)
	r := bufio.NewReader(strings.NewReader(string([]byte{'x', 'y', 3})))
	out, ok := editText(r, "task", "seed")
	if ok {
		t.Fatal("editText reported save, expected discard")
	}
	if out != "seed" {
		t.Fatalf("discard returned %q, want original %q", out, "seed")
	}
}

// TestOpenDetail checks Enter targets the selected task's id and summary.
func TestOpenDetail(t *testing.T) {
	t.Setenv("NOTIE_DIR", t.TempDir())
	tt := &taskTUI{
		lines: []string{"# Tasks", "", "- [ ] #12 !1 buy milk (added 2026-08-06)"},
		tasks: []int{2},
	}
	tt.openDetail()
	if !tt.detail || tt.detailID != "12" || tt.detailSum != "buy milk" {
		t.Fatalf("openDetail state: detail=%v id=%q sum=%q", tt.detail, tt.detailID, tt.detailSum)
	}
}
