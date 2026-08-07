package main

import (
	"strings"
	"testing"
)

// TestTaskDeleteUndo deletes a task (with a description) and confirms u revives
// both the task line and its detail file.
func TestTaskDeleteUndo(t *testing.T) {
	t.Setenv("NOTIE_DIR", t.TempDir())
	writeLines(taskPath(), []string{
		"# Tasks", "",
		"- [ ] #1 !2 alpha (added 2026-08-07)",
		"- [ ] #2 !2 beta (added 2026-08-07)",
	})
	writeTaskDetail("1", "important detail")

	tt := &taskTUI{showDone: true}
	tt.reload()
	tt.cursor = 0 // #1
	tt.delete()
	tt.reload()

	if hasTaskDetail("1") {
		t.Fatal("detail should be gone right after delete")
	}
	if strings.Contains(strings.Join(tt.lines, "\n"), "alpha") {
		t.Fatal("task line should be gone right after delete")
	}

	tt.popUndo()
	if !hasTaskDetail("1") {
		t.Fatal("undo should restore the deleted task's detail")
	}
	if readTaskDetail("1") != "important detail" {
		t.Fatalf("restored detail = %q", readTaskDetail("1"))
	}
	if !strings.Contains(strings.Join(tt.lines, "\n"), "alpha") {
		t.Fatal("undo should restore the deleted task line")
	}
}

// TestNotesDeleteUndo deletes a note and confirms u restores it.
func TestNotesDeleteUndo(t *testing.T) {
	t.Setenv("NOTIE_DIR", t.TempDir())
	tt := &notesTUI{cfg: rememberCfg}
	writeLines(tt.path(), []string{
		"# Remember", "",
		"- 2026-08-07 09:00 — buy milk",
		"- 2026-08-07 10:00 — call alice",
	})
	tt.reload()
	tt.cursor = 0
	tt.deleteNote()
	tt.reload()
	if strings.Contains(strings.Join(tt.lines, "\n"), "buy milk") {
		t.Fatal("note should be gone after delete")
	}
	tt.popUndo()
	if !strings.Contains(strings.Join(tt.lines, "\n"), "buy milk") {
		t.Fatal("undo should restore the deleted note")
	}
}
