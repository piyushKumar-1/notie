package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestEditFileInvokesEditor checks editFile resolves $EDITOR and passes the
// target path as its final argument. A stub editor writes to that path so we
// can confirm it ran against the right file.
func TestEditFileInvokesEditor(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "stub-editor")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf 'edited by stub' > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "note.md")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", stub)

	// editFile writes screen-control escapes to stdout; silence them.
	old := os.Stdout
	os.Stdout, _ = os.Open(os.DevNull)
	err := editFile(target)
	os.Stdout = old
	if err != nil {
		t.Fatalf("editFile returned %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "edited by stub" {
		t.Fatalf("editor did not edit target, got %q", got)
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
