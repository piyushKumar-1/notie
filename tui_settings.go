// Interactive settings: a single-pane list of preferences persisted to
// ~/.notie/config (see config.go). Same vim keys as the other TUIs; choices
// cycle with ←/→, text values edit inline, toggles/actions fire on enter.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// settingItem is one row. kind: 'c' choice (cycle opts) · 't' text (inline
// edit) · 'a' action/toggle (enter fires act) · 'i' read-only info.
type settingItem struct {
	label, hint string
	kind        byte
	opts        []string            // for 'c'
	get         func() string       // current value (display)
	set         func(string)        // 'c' and 't' apply
	valid       func(string) string // 't' validation; "" ok, else error message
	act         func(*settingsTUI)  // 'a'
}

type settingsTUI struct {
	items   []settingItem
	cursor  int
	pending byte       // 'y' chord state
	in      *lineInput // non-nil while editing a text value
	status  string
}

// statusOnOff renders a themed "enabled/disabled <what>" line.
func statusOnOff(on bool, what string) string {
	if on {
		return cGreen + "enabled " + what + cReset
	}
	return cGrey + "disabled " + what + cReset
}

// hhmmValid rejects anything that is not a 24h HH:MM time.
func hhmmValid(v string) string {
	if timeRe.MatchString(strings.TrimSpace(v)) {
		return ""
	}
	return "use HH:MM (24-hour)"
}

func accentNames() []string {
	out := make([]string, len(accentPalette))
	for i, a := range accentPalette {
		out[i] = a.name
	}
	return out
}

// zshHookStatus reports whether the zsh audit hook is wired into ~/.zshrc.
func zshHookStatus() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "unknown"
	}
	data, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	if strings.Contains(string(data), "notie shell-init") || strings.Contains(string(data), "notie shell audit") {
		return "active"
	}
	return "not in ~/.zshrc"
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func settingsItems() []settingItem {
	return []settingItem{
		{
			label: "Accent color", hint: "the UI highlight color",
			kind: 'c', opts: accentNames(),
			get: func() string { return configVal("ui.accent", "blue") },
			set: setAccent,
		},
		{
			label: "Default task priority", hint: "0 high · 1 normal · 2 low",
			kind: 'c', opts: []string{"0", "1", "2"},
			get: defaultPri,
			set: func(v string) { configSet("task.priority", v) },
		},
		{
			label: "Voice transcriber", hint: "engine for notie radd / rtask",
			kind: 'c', opts: []string{"auto", "hear", "whisper"},
			get: func() string { return configVal("voice.transcriber", "auto") },
			set: func(v string) { configSet("voice.transcriber", v) },
		},
		{
			label: "Whisper model", hint: "path to a ggml model · blank = auto-detect",
			kind: 't',
			get:  func() string { return configVal("voice.whisper_model", "") },
			set:  func(v string) { configSet("voice.whisper_model", strings.TrimSpace(v)) },
		},
		{
			label: "Collapse done tasks", hint: "hide done tasks in the task list by default",
			kind: 'a',
			get:  func() string { return onOff(collapseDone()) },
			act: func(t *settingsTUI) {
				b := !collapseDone()
				configSet("task.collapse_done", onOff(b))
				t.status = statusOnOff(b, "collapse done")
			},
		},
		{
			label: "Confirm before delete", hint: "require a second keystroke before dd deletes",
			kind: 'a',
			get:  func() string { return onOff(confirmDelete()) },
			act: func(t *settingsTUI) {
				b := !confirmDelete()
				configSet("ui.confirm_delete", onOff(b))
				t.status = statusOnOff(b, "delete confirm")
			},
		},
		{
			label: "Stale-summary warnings", hint: "warn to re-run notie cache after backdated edits",
			kind: 'a',
			get:  func() string { return onOff(warnStale()) },
			act: func(t *settingsTUI) {
				b := !warnStale()
				configSet("summary.warn_stale", onOff(b))
				t.status = statusOnOff(b, "stale warnings")
			},
		},
		{
			label: "Daily summaries", hint: "run notie cache on a schedule (macOS launchd)",
			kind: 'a',
			get:  func() string { return onOff(configVal("cache.schedule", "off") == "on") },
			act:  scheduleToggle("cache.schedule", "daily summaries", syncCacheAgent),
		},
		{
			label: "  summary time", hint: "HH:MM the daily-summaries job runs",
			kind: 't', valid: hhmmValid,
			get: func() string { return configVal("cache.time", "09:00") },
			set: func(v string) { configSet("cache.time", strings.TrimSpace(v)); syncCacheAgent() },
		},
		{
			label: "Auto-update", hint: "run notie upgrade on a schedule (macOS launchd)",
			kind: 'a',
			get:  func() string { return onOff(configVal("upgrade.schedule", "off") == "on") },
			act:  scheduleToggle("upgrade.schedule", "auto-update", syncUpgradeAgent),
		},
		{
			label: "  update time", hint: "HH:MM the auto-update job runs",
			kind: 't', valid: hhmmValid,
			get: func() string { return configVal("upgrade.time", "04:00") },
			set: func(v string) { configSet("upgrade.time", strings.TrimSpace(v)); syncUpgradeAgent() },
		},
		{
			label: "Claude Code capture", hint: "log Claude Code's shell commands to the audit trail",
			kind: 'a',
			get:  func() string { return onOff(claudeHookInstalled()) },
			act: func(t *settingsTUI) {
				on, err := setClaudeHook(!claudeHookInstalled())
				switch {
				case err != nil:
					t.status = cRed + "error: " + err.Error() + cReset
				case on:
					t.status = cGreen + "enabled — restart Claude Code to apply" + cReset
				default:
					t.status = cGrey + "disabled" + cReset
				}
			},
		},
		{label: "Notes directory", kind: 'i', get: notieDir},
		{label: "zsh audit hook", kind: 'i', get: zshHookStatus},
		{label: "Version", kind: 'i', get: func() string { rev, _ := selfRevision(); return shortRev(rev) }},
	}
}

// scheduleToggle builds the action for a launchd job toggle: flip the config
// flag, reconcile the LaunchAgent, and report the result.
func scheduleToggle(key, what string, sync func() error) func(*settingsTUI) {
	return func(t *settingsTUI) {
		on := configVal(key, "off") != "on"
		configSet(key, onOff(on))
		if err := sync(); err != nil {
			configSet(key, onOff(!on)) // roll back on failure
			t.status = cRed + err.Error() + cReset
			return
		}
		t.status = statusOnOff(on, what)
	}
}

const settingsLabelW = 22

// value renders the right-hand side of a row for its kind.
func (t *settingsTUI) value(it settingItem) string {
	switch it.kind {
	case 'c':
		cur := it.get()
		parts := make([]string, len(it.opts))
		for i, o := range it.opts {
			if o == cur {
				parts[i] = cAccent + cBold + o + cReset
			} else {
				parts[i] = cGrey + o + cReset
			}
		}
		return strings.Join(parts, "  ")
	case 't':
		if v := it.get(); v != "" {
			return v
		}
		return cGrey + "(auto-detect)" + cReset
	case 'a':
		if it.get() == "on" {
			return cGreen + iToggleOn + " on" + cReset
		}
		return cGrey + iToggleOff + " off" + cReset
	default: // 'i'
		return cGrey + it.get() + cReset
	}
}

func (t *settingsTUI) render() {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	rows, cols := termSize()
	b.WriteString(titleBar(cols, iSettings+" notie · settings", "") + "\r\n\r\n")

	for i, it := range t.items {
		marker, lab := "  ", padTo(it.label, settingsLabelW)
		switch {
		case i == t.cursor:
			marker = cAccent + cBold + iCursor + cReset + " "
			lab = cBold + lab + cReset
		case it.kind == 'i':
			lab = cGrey + lab + cReset
		}
		b.WriteString("  " + marker + lab + t.value(it) + "\r\n")
	}

	b.WriteString(fmt.Sprintf("\x1b[%d;1H", rows))
	switch {
	case t.in != nil:
		b.WriteString(inputBar(cYellow+" "+iEdit+" "+cReset, t.in.buf))
	case t.status != "":
		b.WriteString(" " + t.status)
	default:
		hint := t.items[t.cursor].hint
		help := " j/k move · ←/→ change · enter edit/toggle · yy copy · q quit"
		if hint != "" {
			help = " " + cItalic + hint + cReset + cGrey + "   " + strings.TrimSpace(help)
		}
		b.WriteString(cGrey + help + cReset)
	}
	os.Stdout.WriteString(b.String())
}

// cycleOpt advances a choice row to its next/prev option and persists it.
func (t *settingsTUI) cycleOpt(dir int) {
	it := t.items[t.cursor]
	if it.kind != 'c' {
		return
	}
	idx := 0
	for i, o := range it.opts {
		if o == it.get() {
			idx = i
		}
	}
	n := len(it.opts)
	it.set(it.opts[((idx+dir)%n+n)%n])
	t.status = ""
}

func (t *settingsTUI) activate() {
	it := t.items[t.cursor]
	switch it.kind {
	case 'c':
		t.cycleOpt(1)
	case 't':
		t.in = &lineInput{kind: 'e', buf: it.get()}
	case 'a':
		if it.act != nil {
			it.act(t)
		}
	}
}

func runSettingsTUI() {
	t := &settingsTUI{items: settingsItems()}
	runScreen(cmdShowSettings, func(r *bufio.Reader) {
		for {
			t.render()
			c, err := readKey(r)
			if err != nil {
				return
			}
			if t.in != nil {
				switch t.in.key(c) {
				case "cancel":
					t.in = nil
				case "submit":
					val := t.in.buf
					it := t.items[t.cursor]
					t.in = nil
					if it.valid != nil {
						if msg := it.valid(val); msg != "" {
							t.status = cRed + msg + cReset
							break
						}
					}
					it.set(val)
					t.status = cGreen + "saved" + cReset
				}
				continue
			}
			if t.pending == 'y' {
				t.pending = 0
				if c == 'y' {
					yank(&t.status, t.items[t.cursor].get())
				}
				continue
			}
			switch c {
			case 'q', 3:
				return
			case 'y':
				t.pending = 'y'
			case 'j':
				if t.cursor < len(t.items)-1 {
					t.cursor, t.status = t.cursor+1, ""
				}
			case 'k':
				if t.cursor > 0 {
					t.cursor, t.status = t.cursor-1, ""
				}
			case 'g':
				t.cursor, t.status = 0, ""
			case 'G':
				t.cursor, t.status = len(t.items)-1, ""
			case 'h':
				t.cycleOpt(-1)
			case 'l':
				t.cycleOpt(1)
			case 13, 10, ' ':
				t.activate()
			}
		}
	})
}

// cmdShowSettings prints the current settings — the non-interactive fallback.
func cmdShowSettings() {
	for _, it := range settingsItems() {
		v := it.get()
		if it.kind == 't' && v == "" {
			v = "(auto-detect)"
		}
		fmt.Printf("%-*s %s\n", settingsLabelW, it.label, v)
	}
}
