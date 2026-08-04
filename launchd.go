// macOS scheduled jobs via launchd LaunchAgents. Backs the settings toggles for
// daily summaries (notie cache) and self-update (notie upgrade): each writes a
// per-job plist to ~/Library/LaunchAgents and (un)loads it with launchctl.
//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func launchAgentsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents")
}

func launchAgentPath(label string) string {
	return filepath.Join(launchAgentsDir(), label+".plist")
}

func launchAgentInstalled(label string) bool {
	_, err := os.Stat(launchAgentPath(label))
	return err == nil
}

// notieBinary is the absolute path a scheduled job should run (symlinks
// resolved), falling back to bare "notie" on PATH.
func notieBinary() string {
	if p, err := os.Executable(); err == nil {
		if rp, err := filepath.EvalSymlinks(p); err == nil {
			return rp
		}
		return p
	}
	return "notie"
}

// syncLaunchAgent reconciles one job's LaunchAgent: when enabled it (re)writes
// the plist to run notie args at hh:mm daily and loads it; when disabled it
// unloads and removes it. Reloading on every change keeps launchd in sync with
// a new time.
func syncLaunchAgent(label string, args []string, enabled bool, hhmm string) error {
	path := launchAgentPath(label)
	if !enabled {
		exec.Command("launchctl", "unload", "-w", path).Run()
		os.Remove(path)
		return nil
	}
	hh, mm := 9, 0
	if p := strings.SplitN(hhmm, ":", 2); len(p) == 2 {
		hh, _ = strconv.Atoi(p[0])
		mm, _ = strconv.Atoi(p[1])
	}
	if err := os.MkdirAll(launchAgentsDir(), 0o755); err != nil {
		return err
	}
	exec.Command("launchctl", "unload", "-w", path).Run()
	if err := os.WriteFile(path, []byte(renderLaunchPlist(label, args, hh, mm)), 0o644); err != nil {
		return err
	}
	return exec.Command("launchctl", "load", "-w", path).Run()
}

// renderLaunchPlist builds the LaunchAgent plist XML for a daily job: notie
// (plus args) run at hh:mm, with stderr captured under the temp dir.
func renderLaunchPlist(label string, args []string, hh, mm int) string {
	var prog strings.Builder
	for _, a := range append([]string{notieBinary()}, args...) {
		prog.WriteString("    <string>" + a + "</string>\n")
	}
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
%s  </array>
  <key>StartCalendarInterval</key>
  <dict><key>Hour</key><integer>%d</integer><key>Minute</key><integer>%d</integer></dict>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, label, prog.String(), hh, mm, filepath.Join(os.TempDir(), label+".log"))
}

func syncCacheAgent() error {
	return syncLaunchAgent("com.notie.cache", []string{"cache"},
		configVal("cache.schedule", "off") == "on", configVal("cache.time", "09:00"))
}

func syncUpgradeAgent() error {
	return syncLaunchAgent("com.notie.upgrade", []string{"upgrade"},
		configVal("upgrade.schedule", "off") == "on", configVal("upgrade.time", "04:00"))
}
