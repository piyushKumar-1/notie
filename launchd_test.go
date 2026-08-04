//go:build darwin

package main

import (
	"encoding/xml"
	"strings"
	"testing"
)

// TestRenderLaunchPlist checks the generated LaunchAgent is well-formed XML and
// carries the label, schedule time and program arguments we asked for.
func TestRenderLaunchPlist(t *testing.T) {
	out := renderLaunchPlist("com.notie.cache", []string{"cache"}, 7, 30)

	if err := xml.Unmarshal([]byte(out), new(interface{})); err != nil {
		t.Fatalf("plist is not well-formed XML: %v\n%s", err, out)
	}
	for _, want := range []string{
		"<string>com.notie.cache</string>",
		"<string>cache</string>",
		"<key>Hour</key><integer>7</integer>",
		"<key>Minute</key><integer>30</integer>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plist missing %q\n%s", want, out)
		}
	}
	if !strings.Contains(out, notieBinary()) {
		t.Errorf("plist missing the notie binary path %q", notieBinary())
	}
}
