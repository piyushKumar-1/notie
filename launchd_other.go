// Scheduled jobs are macOS-only (launchd). On other platforms these are no-ops
// so the settings TUI still compiles and reports the jobs as unavailable.
//go:build !darwin

package main

import "errors"

var errLaunchdUnsupported = errors.New("scheduled jobs need macOS (launchd)")

func launchAgentInstalled(string) bool { return false }
func syncCacheAgent() error            { return errLaunchdUnsupported }
func syncUpgradeAgent() error          { return errLaunchdUnsupported }
