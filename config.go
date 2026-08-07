// Persistent settings: a tiny key/value store at ~/.notie/config, one
// "key: value" line per setting. Backs `notie settings` and its TUI. Keys are
// namespaced (task.priority, ui.accent, voice.transcriber, voice.whisper_model).
package main

import (
	"sort"
	"strings"
)

func configPath() string { return notieDir() + "/config" }

// loadConfig reads config into a map; missing file yields an empty map.
func loadConfig() map[string]string {
	m := map[string]string{}
	for _, l := range readLines(configPath()) {
		if strings.HasPrefix(l, "#") || strings.TrimSpace(l) == "" {
			continue
		}
		if i := strings.IndexByte(l, ':'); i > 0 {
			m[strings.TrimSpace(l[:i])] = strings.TrimSpace(l[i+1:])
		}
	}
	return m
}

// configVal returns key's value, or def when it is unset or empty.
func configVal(key, def string) string {
	if v, ok := loadConfig()[key]; ok && v != "" {
		return v
	}
	return def
}

// configSet writes key=val (deleting the key when val is empty), keeping the
// file sorted and stable.
func configSet(key, val string) {
	m := loadConfig()
	if val == "" {
		delete(m, key)
	} else {
		m[key] = val
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := []string{"# notie settings"}
	for _, k := range keys {
		lines = append(lines, k+": "+m[k])
	}
	writeLines(configPath(), lines)
}

// applyConfig loads config-driven runtime state (currently the UI accent).
// Called once at startup, before any command runs. Read-only: never writes.
func applyConfig() {
	for _, s := range iconSets {
		if s.name == configVal("ui.icons", "classic") {
			applyIcons(s)
			break
		}
	}
	name := configVal("ui.accent", "blue")
	for _, a := range accentPalette {
		if a.name == name {
			cAccent = a.code
			return
		}
	}
}

// setAccent persists the named accent and updates the live cAccent color so a
// change inside the settings TUI is visible immediately.
func setAccent(name string) {
	for _, a := range accentPalette {
		if a.name == name {
			cAccent = a.code
			configSet("ui.accent", name)
			return
		}
	}
}

// defaultPri is the priority a task gets when none is given (task.priority,
// default 2 = low).
func defaultPri() string { return configVal("task.priority", "2") }

// confirmDelete reports whether dd needs a confirming second keystroke before
// it removes a row (ui.confirm_delete, default on).
func confirmDelete() bool { return configVal("ui.confirm_delete", "on") == "on" }

// warnStale reports whether backdated writes warn to re-run `notie cache`
// (summary.warn_stale, default on).
func warnStale() bool { return configVal("summary.warn_stale", "on") == "on" }

// collapseDone reports whether the task TUI hides done tasks on open
// (task.collapse_done, default on).
func collapseDone() bool { return configVal("task.collapse_done", "on") == "on" }
