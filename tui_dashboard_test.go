package main

import (
	"os"
	"strings"
	"testing"
)

// TestDashboardRenderSmoke renders the landing dashboard and confirms it lists
// the product areas without panicking.
func TestDashboardRenderSmoke(t *testing.T) {
	t.Setenv("NOTIE_DIR", t.TempDir())

	old := os.Stdout
	f, err := os.CreateTemp(t.TempDir(), "dash")
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = f
	d := &dashboardTUI{items: dashItems()}
	d.cursor = 2 // exercise the selected-row highlight path
	d.render()
	os.Stdout = old
	f.Close()

	data, _ := os.ReadFile(f.Name())
	for _, want := range []string{"dashboard", "Tasks", "Journal", "Remember", "Settings", "notie important"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("dashboard render missing %q", want)
		}
	}
}
