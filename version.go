// Build identity: the commit a notie binary was built from. Go stamps it into
// the binary whenever the build ran inside a git worktree, so there is no
// version constant to bump and nothing to keep in sync. Backs `notie version`
// and the up-to-date check in `notie upgrade`.
package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"time"
)

// selfRevision returns the commit this binary was built from, and whether the
// worktree had uncommitted changes at the time. Both are zero for a binary
// built outside a git worktree, so callers must treat "" as "don't know"
// rather than "no changes".
func selfRevision() (rev string, dirty bool) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	return rev, dirty
}

func shortRev(rev string) string {
	if rev == "" {
		return "unknown"
	}
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}

// cmdVersion prints the commit, its date and the Go toolchain behind this
// binary — enough to tell two builds apart when reporting a bug.
func cmdVersion() {
	rev, dirty := selfRevision()
	when, goVer := "", runtime.Version()
	if bi, ok := debug.ReadBuildInfo(); ok {
		goVer = bi.GoVersion
		for _, s := range bi.Settings {
			if s.Key == "vcs.time" {
				when = s.Value
			}
		}
	}
	if rev == "" {
		fmt.Printf("notie (unknown revision) %s\n", goVer)
		return
	}
	out := "notie " + shortRev(rev)
	if t, err := time.Parse(time.RFC3339, when); err == nil {
		out += " (" + t.Local().Format("2006-01-02") + ")"
	}
	if dirty {
		out += " (modified)"
	}
	fmt.Println(out + " " + goVer)
}
