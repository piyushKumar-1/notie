// notie — tiny local notes CLI. Everything lives in ~/.notie as plain markdown.
// Standalone binary: Go stdlib only, no external commands required.
// (Exception: `notie cache` will use the `claude` CLI for nicer one-line
// summaries if it is installed, and falls back to joining entries otherwise.)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
var timeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
var dcLineRe = regexp.MustCompile(`^- (\d{4}-\d{2}-\d{2}): (.*)$`)

func notieDir() string {
	if d := os.Getenv("NOTIE_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fatal("cannot resolve home directory: %v", err)
	}
	return filepath.Join(home, ".notie")
}

func fatal(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "notie: "+format+"\n", a...)
	os.Exit(1)
}

func today() string { return time.Now().Format("2006-01-02") }
func clock() string { return time.Now().Format("15:04") }

func journalPath(d string) string { return filepath.Join(notieDir(), d, "journal.md") }
func datecachePath() string       { return filepath.Join(notieDir(), "datecache.md") }

// cachedDate reports whether datecache.md already summarizes d — a retroactive
// write to such a day leaves its one-liner stale until `notie cache <d>`.
func cachedDate(d string) bool {
	for _, l := range readLines(datecachePath()) {
		if m := dcLineRe.FindStringSubmatch(l); m != nil && m[1] == d {
			return true
		}
	}
	return false
}

func readLines(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func writeLines(path string, lines []string) {
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		fatal("writing %s: %v", path, err)
	}
}

// appendLine appends a line to path, creating it with "# header" if missing.
func appendLine(path, header, line string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fatal("creating %s: %v", filepath.Dir(path), err)
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte("# "+header+"\n\n"), 0o644); err != nil {
			fatal("creating %s: %v", path, err)
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fatal("opening %s: %v", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		fatal("writing %s: %v", path, err)
	}
}

func usage() {
	fmt.Print(`notie — local notes in ~/.notie

  notie add "text"        append to today's journal (~/.notie/<date>/journal.md)
  notie add <date> [HH:MM] "text"
                          add to an older day, in chronological position
  notie did <date> "text" record a task that was already done on that day
  notie log "cmd"         append a shell command to today's audit trail
                          (~/.notie/<date>/shell.md — wired to a zsh preexec hook)
  notie shell-init        print the zsh audit hook — source it with
                          eval "$(notie shell-init)" (works under nix/home-manager)
  notie setup-claude-hook register a Claude Code hook so its shell commands are logged
  notie addi "text"       append to important.md
  notie remember "text"   append to remember.md
  notie task [0|1|2] "text"  add a task (0 high · 1 normal · 2 low; default 2)
  notie radd              record voice, transcribe, append to today's journal
                          (also: rjournal · raddi/rimportant · rremember · rtask)
  notie task              interactive task list (done tasks hidden — . shows them)
  notie task list         plain list of last 100 tasks, grouped by priority
  notie task done <id>    mark task done
  notie task open <id>    reopen a task
  notie task del <id>     delete a task
  notie journal           interactive journal browser (dates sidebar + / search)
  notie shell             interactive shell-audit browser (same day-level UI)
  notie important         interactive important-notes browser (day-level UI)
  notie remember          interactive remember-notes list
  notie settings          interactive settings (accent, task priority, voice, hooks)
  notie cache             build datecache.md entries for past days (journal + tasks done)
  notie cache <date>      re-summarize one day (after a retroactive edit)
  notie show [what]       print a file (journal|shell|remember|important|task|datecache|YYYY-MM-DD)
  notie show shell [date] print a day's shell audit trail (default today)
  notie upgrade           clone the public repo, rebuild, and replace this binary
                          (--check reports what's available, installs nothing)
  notie version           print the commit this binary was built from

  TUI keys: ↑/↓ (j/k) move · ←/→ (h/l) switch pane in the day browsers
            gg/G top/bottom · x toggle (tasks) · 0/1/2 priority · . show/hide done
            a add · e edit · dd delete · / search · n/N next/prev · t today
            q/:q quit · :ff <pat> find date files · :fg <pat> find mentions
`)
}

// ---- add / addi / remember ----

// addJournal writes a journal entry to date's journal, inserted after the last
// entry with an earlier timestamp so the day stays in chronological order —
// retroactive entries land in the right place instead of at the end. Entries
// already in the file are never reordered. Silent, so the TUI can call it
// without corrupting its raw-mode screen.
func addJournal(date, hhmm, text string) {
	path := journalPath(date)
	line := fmt.Sprintf("- %s — %s", hhmm, text)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		appendLine(path, "Journal — "+date, line)
	} else {
		lines := readLines(path)
		pos := len(lines)
		for i, l := range lines {
			m := entryRe.FindStringSubmatch(l)
			if m == nil {
				continue
			}
			if m[1] > hhmm {
				pos = i
				break
			}
			pos = i + 1
		}
		out := make([]string, 0, len(lines)+1)
		out = append(out, lines[:pos]...)
		out = append(out, line)
		out = append(out, lines[pos:]...)
		writeLines(path, out)
	}
}

func cmdAdd(date, hhmm, text string) {
	addJournal(date, hhmm, text)
	fmt.Println("journal: " + text)
	staleCacheHint(date)
}

// staleCacheHint nudges the user to refresh a day's summary after a retroactive
// write. Silent for today, which is never cached.
func staleCacheHint(date string) {
	if warnStale() && date != today() && cachedDate(date) {
		fmt.Printf("note: %s is already summarized — run 'notie cache %s' to refresh\n", date, date)
	}
}

func cmdDated(file, header, label, text string) {
	path := filepath.Join(notieDir(), file)
	appendLine(path, header, fmt.Sprintf("- %s %s — %s", today(), clock(), text))
	fmt.Println(label + ": " + text)
}

// ---- shell audit trail ----

// logShell appends a shell command to today's shell.md, tagged with marker
// ("$" for an interactive command, iAgent for one run by an agent). Silent on
// success and tolerant of empty input: it is invoked by hooks on every command,
// so it must never clutter the prompt.
func logShell(text, cwd, marker string) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return
	}
	loc := ""
	if cwd != "" {
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(cwd, home) {
			cwd = "~" + strings.TrimPrefix(cwd, home)
		}
		loc = " (" + cwd + ")"
	}
	path := filepath.Join(notieDir(), today(), "shell.md")
	appendLine(path, "Shell — "+today(), fmt.Sprintf("- %s%s %s %s", clock(), loc, marker, text))
}

// cmdLog records an interactive shell command (the zsh preexec hook path).
func cmdLog(text string) {
	cwd, _ := os.Getwd()
	logShell(text, cwd, "$")
}

// cmdLogHook records a command run by Claude Code. It reads the tool's
// PreToolUse JSON ({"cwd":…,"tool_input":{"command":…}}) from stdin — the shape
// Claude Code pipes to a PreToolUse hook — and tags it as agent-run. Always
// exits cleanly so it can never block the tool.
func cmdLogHook() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return
	}
	var in struct {
		Cwd       string `json:"cwd"`
		ToolInput struct {
			Command string `json:"command"`
		} `json:"tool_input"`
	}
	if json.Unmarshal(data, &in) != nil {
		return
	}
	logShell(in.ToolInput.Command, in.Cwd, iAgent)
}

// cmdShellInit prints the zsh preexec hook. Sourcing it (eval "$(notie
// shell-init)") keeps the hook in the binary, so it survives on nix / home-
// manager setups where ~/.zshrc is read-only or regenerated.
func cmdShellInit() {
	fmt.Print(`# notie shell audit trail — source via: eval "$(notie shell-init)"
_notie_log() { command notie log "$1" >/dev/null 2>&1 }
autoload -Uz add-zsh-hook
add-zsh-hook preexec _notie_log
`)
}

// claudeSettingsPath returns the ~/.claude dir and its settings.json path.
func claudeSettingsPath() (dir, path string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	dir = filepath.Join(home, ".claude")
	return dir, filepath.Join(dir, "settings.json"), nil
}

// loadClaudeSettings reads settings.json into a map; a missing or empty file
// yields an empty map, but malformed JSON is an error (we must not clobber it).
func loadClaudeSettings(path string) (map[string]any, error) {
	s := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return s, nil
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return s, nil
}

// claudeHookIndex returns the PreToolUse slice and the index of the group
// holding notie's log-hook (or -1 if absent).
func claudeHookIndex(s map[string]any) ([]any, int) {
	hooks, _ := s["hooks"].(map[string]any)
	pre, _ := hooks["PreToolUse"].([]any)
	for i, g := range pre {
		gm, _ := g.(map[string]any)
		hl, _ := gm["hooks"].([]any)
		for _, h := range hl {
			hm, _ := h.(map[string]any)
			if cmd, _ := hm["command"].(string); strings.Contains(cmd, "notie log-hook") {
				return pre, i
			}
		}
	}
	return pre, -1
}

// claudeHookInstalled reports whether the log-hook is registered.
func claudeHookInstalled() bool {
	_, path, err := claudeSettingsPath()
	if err != nil {
		return false
	}
	s, err := loadClaudeSettings(path)
	if err != nil {
		return false
	}
	_, i := claudeHookIndex(s)
	return i >= 0
}

// setClaudeHook installs (on) or removes (off) notie's Bash PreToolUse hook,
// preserving all other settings. Returns the resulting state.
func setClaudeHook(on bool) (bool, error) {
	dir, path, err := claudeSettingsPath()
	if err != nil {
		return false, err
	}
	s, err := loadClaudeSettings(path)
	if err != nil {
		return false, err
	}
	pre, idx := claudeHookIndex(s)
	switch {
	case on && idx >= 0:
		return true, nil // already installed
	case !on && idx < 0:
		return false, nil // already absent
	case on:
		pre = append(pre, map[string]any{
			"matcher": "Bash",
			"hooks":   []any{map[string]any{"type": "command", "command": "notie log-hook"}},
		})
	default:
		pre = append(pre[:idx], pre[idx+1:]...)
	}
	hooks, _ := s["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	if len(pre) == 0 {
		delete(hooks, "PreToolUse")
	} else {
		hooks["PreToolUse"] = pre
	}
	if len(hooks) == 0 {
		delete(s, "hooks")
	} else {
		s["hooks"] = hooks
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return idx >= 0, err
	}
	out, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return idx >= 0, err
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		return idx >= 0, err
	}
	return on, nil
}

// cmdSetupClaudeHook registers the Bash PreToolUse hook so Claude Code's shell
// commands land in notie's audit trail. Idempotent.
func cmdSetupClaudeHook() {
	_, path, _ := claudeSettingsPath()
	if claudeHookInstalled() {
		fmt.Println("notie: Claude Code hook already installed in " + path)
		return
	}
	if _, err := setClaudeHook(true); err != nil {
		fatal("updating %s: %v", path, err)
	}
	fmt.Println("notie: added a Bash PreToolUse hook to " + path)
	fmt.Println("      Claude Code will now log its shell commands (restart Claude Code to apply)")
}

// ---- tasks ----

// taskPath returns ~/.notie/task.md, migrating a legacy todo.md in place.
func taskPath() string {
	p := filepath.Join(notieDir(), "task.md")
	if _, err := os.Stat(p); os.IsNotExist(err) {
		old := filepath.Join(notieDir(), "todo.md")
		if _, err := os.Stat(old); err == nil {
			os.Rename(old, p)
		}
	}
	return p
}

var taskRe = regexp.MustCompile(`^- \[[ x]\] #(\d+) `)
var taskPriRe = regexp.MustCompile(`^- \[[ x]\] #\d+ !([0-2]) `)

// taskPri returns a task's priority (0 high · 1 normal · 2 low). Lines
// without a marker (pre-priority tasks) sort after every prioritized group.
func taskPri(l string) int {
	if m := taskPriRe.FindStringSubmatch(l); m != nil {
		return int(m[1][0] - '0')
	}
	return 3
}

func taskLines() []string {
	var out []string
	for _, l := range readLines(taskPath()) {
		if taskRe.MatchString(l) {
			out = append(out, l)
		}
	}
	return out
}

func printTasks() {
	tasks := taskLines()
	if len(tasks) == 0 {
		fmt.Println("no tasks yet")
		return
	}
	if len(tasks) > 100 {
		tasks = tasks[len(tasks)-100:]
	}
	sort.SliceStable(tasks, func(i, j int) bool { return taskPri(tasks[i]) < taskPri(tasks[j]) })
	for _, t := range tasks {
		fmt.Println(t)
	}
}

// nextID reads .task_seq, self-healing against the max id present in task.md.
func nextID() int {
	seqPath := filepath.Join(notieDir(), ".task_seq")
	id := 0
	if data, err := os.ReadFile(seqPath); err == nil {
		if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			id = n
		}
	}
	for _, l := range taskLines() {
		if n, err := strconv.Atoi(taskRe.FindStringSubmatch(l)[1]); err == nil && n > id {
			id = n
		}
	}
	id++
	if err := os.MkdirAll(notieDir(), 0o755); err != nil {
		fatal("creating %s: %v", notieDir(), err)
	}
	if err := os.WriteFile(seqPath, []byte(strconv.Itoa(id)+"\n"), 0o644); err != nil {
		fatal("writing %s: %v", seqPath, err)
	}
	return id
}

var doneStampRe = regexp.MustCompile(` \(done \d{4}-\d{2}-\d{2}\)$`)

func taskEdit(idStr, action string) {
	if _, err := strconv.Atoi(idStr); err != nil {
		fatal("usage: notie task %s <id>", action)
	}
	lines := readLines(taskPath())
	if lines == nil {
		fatal("no tasks yet")
	}
	prefix := regexp.MustCompile(`^- \[[ x]\] #` + idStr + ` `)
	found := false
	var out []string
	for _, l := range lines {
		if !prefix.MatchString(l) {
			out = append(out, l)
			continue
		}
		found = true
		switch action {
		case "del":
			continue
		case "done":
			l = strings.Replace(l, "- [ ]", "- [x]", 1)
			if !doneStampRe.MatchString(l) {
				l += " (done " + today() + ")"
			}
		case "open":
			l = strings.Replace(l, "- [x]", "- [ ]", 1)
			l = doneStampRe.ReplaceAllString(l, "")
		}
		out = append(out, l)
	}
	if !found {
		fatal("no task #%s", idStr)
	}
	writeLines(taskPath(), out)
	fmt.Printf("task #%s: %s\n", idStr, action)
}

// cmdDid records a task that was already finished — added and done on the same
// past date. Priority is fixed at normal: a completed task's priority is moot,
// but the marker keeps the line matching taskFullRe. Field order matters —
// (added …) must precede (done …), or doneTasksByDate silently drops it.
func cmdDid(date, desc string) {
	id := nextID()
	appendLine(taskPath(), "Tasks",
		fmt.Sprintf("- [x] #%d !1 %s (added %s) (done %s)", id, desc, date, date))
	fmt.Printf("recorded #%d as done on %s\n", id, date)
	staleCacheHint(date)
}

func cmdTask(args []string) {
	if len(args) == 0 {
		if isTTY() {
			runTaskTUI()
		} else {
			printTasks()
		}
		return
	}
	switch args[0] {
	case "list":
		printTasks()
	case "done", "open", "del":
		if len(args) < 2 {
			fatal("usage: notie task %s <id>", args[0])
		}
		taskEdit(args[1], args[0])
	default:
		// A leading 0|1|2 sets the priority; without one the task takes the
		// default, so "notie task \"buy milk\"" works.
		pri, rest := defaultPri(), args
		if args[0] == "0" || args[0] == "1" || args[0] == "2" {
			pri, rest = args[0], args[1:]
		}
		desc := strings.TrimSpace(strings.Join(rest, " "))
		if desc == "" {
			fatal("missing text — notie task [0|1|2] \"text\"")
		}
		id := nextID()
		appendLine(taskPath(), "Tasks", fmt.Sprintf("- [ ] #%d !%s %s (added %s)", id, pri, desc, today()))
		fmt.Printf("added task #%d\n---\n", id)
		printTasks()
	}
}

// ---- datecache ----

// entryRe matches a journal entry's timestamp prefix. The capture group is used
// to order retroactive inserts; ReplaceAllString with "" strips the prefix.
var entryRe = regexp.MustCompile(`^- (\d{2}:\d{2}) — `)

// journalEntries returns the entry texts of a journal file, timestamps stripped.
func journalEntries(path string) []string {
	var out []string
	for _, l := range readLines(path) {
		if strings.HasPrefix(l, "- ") {
			out = append(out, entryRe.ReplaceAllString(l, ""))
		}
	}
	return out
}

// doneTasksByDate maps a completion date to the descriptions of the tasks
// closed that day. Completed tasks with no (done …) stamp have no known date
// and are skipped.
func doneTasksByDate() map[string][]string {
	out := map[string][]string{}
	for _, l := range readLines(taskPath()) {
		m := taskFullRe.FindStringSubmatch(l)
		if m == nil || m[1] != "x" || m[6] == "" {
			continue
		}
		out[m[6]] = append(out[m[6]], m[4])
	}
	return out
}

// summarize produces the one-liner for a day from its journal and the tasks
// closed that day. Prefers Claude Code (headless, haiku); falls back to joining
// the raw entries if claude is unavailable, errors out, or takes >2 minutes.
func summarize(journalPath string, done []string) string {
	entries := journalEntries(journalPath)
	if len(entries) == 0 && len(done) == 0 {
		return ""
	}
	if bin, err := exec.LookPath("claude"); err == nil {
		var b strings.Builder
		if raw, err := os.ReadFile(journalPath); err == nil {
			b.Write(raw)
		}
		if len(done) > 0 {
			b.WriteString("\n## Tasks completed\n")
			for _, t := range done {
				b.WriteString("- " + t + "\n")
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "-p", "--model", "haiku",
			"Below is one day's work journal, followed by the tasks closed that day (either section may be absent). Reply with EXACTLY one line (no preamble, no markdown) summarizing what was done that day, compressing related entries together and folding the completed tasks in without repeating what the journal already covers.")
		cmd.Stdin = strings.NewReader(b.String())
		if out, err := cmd.Output(); err == nil {
			s := strings.Join(strings.Fields(string(out)), " ")
			if s != "" {
				return s
			}
		}
	}
	s := strings.Join(entries, "; ")
	if len(done) > 0 {
		if s != "" {
			s += "; "
		}
		s += "Completed: " + strings.Join(done, "; ")
	}
	return s
}

// cmdCache summarizes each past day — its journal plus the tasks closed that
// day — into one line in datecache.md. Idempotent: skips dates already cached.
// Catch-up safe: processes every past date, so missed runs (laptop asleep) are
// filled in on the next run. A non-empty force re-summarizes just that date,
// which is how a retroactively edited day gets a fresh one-liner.
func cmdCache(force string) {
	dir := notieDir()
	cachePath := datecachePath()
	if force != "" && force >= today() {
		fatal("cannot cache %s — only past days are summarized", force)
	}
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		appendLine(cachePath, "Date cache", "")
	}
	cached := map[string]bool{}
	dropped := false
	var header, entries []string
	for _, l := range readLines(cachePath) {
		if m := dcLineRe.FindStringSubmatch(l); m != nil {
			if m[1] == force {
				dropped = true // omit, so the date is treated as uncached below
				continue
			}
			cached[m[1]] = true
			entries = append(entries, l)
		} else if strings.TrimSpace(l) != "" || len(entries) == 0 {
			header = append(header, l)
		}
	}
	// A day is worth summarizing if it has a journal directory or if any task
	// was closed on it — a day spent only ticking tasks off still counts.
	done := doneTasksByDate()
	candidates := map[string]bool{}
	dirents, _ := os.ReadDir(dir)
	for _, de := range dirents {
		if de.IsDir() && dateRe.MatchString(de.Name()) {
			candidates[de.Name()] = true
		}
	}
	for d := range done {
		candidates[d] = true
	}
	var dates []string
	for d := range candidates {
		if d < today() && !cached[d] && (force == "" || d == force) {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates) // chronological, and deterministic across runs
	added := 0
	for _, d := range dates {
		if s := summarize(journalPath(d), done[d]); s != "" {
			entries = append(entries, fmt.Sprintf("- %s: %s", d, s))
			added++
		}
	}
	// dropped alone still needs a write: a forced date whose content has since
	// been emptied must lose its stale line.
	if added > 0 || dropped {
		sort.Strings(entries)
		if len(header) == 0 || header[len(header)-1] != "" {
			header = append(header, "")
		}
		writeLines(cachePath, append(header, entries...))
	}
	if force != "" {
		if added > 0 {
			fmt.Printf("datecache: %s refreshed\n", force)
		} else {
			fmt.Printf("datecache: nothing to summarize for %s\n", force)
		}
		return
	}
	fmt.Printf("datecache: %d day(s) added\n", added)
}

// ---- show ----

func printFile(path, emptyMsg string) {
	data, err := os.ReadFile(path)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		fmt.Println(emptyMsg)
		return
	}
	fmt.Print(string(data))
}

// latestDateWith returns the most recent date dir containing the given file.
func latestDateWith(name string) string {
	dirents, _ := os.ReadDir(notieDir())
	latest := ""
	for _, de := range dirents {
		d := de.Name()
		if de.IsDir() && dateRe.MatchString(d) && d > latest {
			if _, err := os.Stat(filepath.Join(notieDir(), d, name)); err == nil {
				latest = d
			}
		}
	}
	return latest
}

// cmdShowShell prints a day's shell audit trail; empty date means today,
// falling back to the most recent day that has one.
func cmdShowShell(d string) {
	dir := notieDir()
	if d != "" {
		printFile(filepath.Join(dir, d, "shell.md"), "no shell log for "+d)
		return
	}
	d = today()
	if _, err := os.Stat(filepath.Join(dir, d, "shell.md")); err != nil {
		if l := latestDateWith("shell.md"); l != "" {
			fmt.Printf("no shell log for today; latest is %s:\n\n", l)
			d = l
		} else {
			fmt.Println("no shell log yet — commands are recorded by the zsh hook")
			return
		}
	}
	printFile(filepath.Join(dir, d, "shell.md"), "empty")
}

func cmdShow(what string) {
	dir := notieDir()
	switch what {
	case "journal":
		path := journalPath(today())
		if _, err := os.Stat(path); err != nil {
			if d := latestDateWith("journal.md"); d != "" {
				fmt.Printf("no journal for today; latest is %s:\n\n", d)
				printFile(journalPath(d), "empty")
				return
			}
			fmt.Println("no journal entries yet — try: notie add \"did something\"")
			return
		}
		printFile(path, "empty")
	case "remember", "important", "datecache":
		printFile(filepath.Join(dir, what+".md"), "empty")
	case "task", "todo":
		printTasks()
	default:
		if dateRe.MatchString(what) {
			printFile(journalPath(what), "no journal for "+what)
			return
		}
		fatal("unknown file %q (journal|remember|important|task|datecache|YYYY-MM-DD)", what)
	}
}

// ---- main ----

func main() {
	applyConfig()
	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		return
	}
	text := strings.Join(args[1:], " ")
	requireText := func() {
		if strings.TrimSpace(text) == "" {
			fatal("missing text")
		}
	}
	// pastDate validates an optional retroactive date argument.
	pastDate := func(d string) string {
		if d > today() {
			fatal("cannot write to a future date: %s", d)
		}
		return d
	}
	switch args[0] {
	case "add":
		// optional positional date, then optional HH:MM — same sniffing style as
		// `notie show shell <date>`. The time is only looked for after a date
		// matched, so `notie add "14:05 standup"` stays plain text.
		d, hhmm, i := today(), clock(), 1
		if len(args) > i && dateRe.MatchString(args[i]) {
			d, i = pastDate(args[i]), i+1
			if len(args) > i && timeRe.MatchString(args[i]) {
				hhmm, i = args[i], i+1
			}
		}
		body := strings.TrimSpace(strings.Join(args[i:], " "))
		if body == "" {
			fatal("missing text")
		}
		cmdAdd(d, hhmm, body)
	case "did":
		if len(args) < 2 || !dateRe.MatchString(args[1]) {
			fatal("usage: notie did <YYYY-MM-DD> \"text\"")
		}
		desc := strings.TrimSpace(strings.Join(args[2:], " "))
		if desc == "" {
			fatal("missing text — notie did %s \"text\"", args[1])
		}
		cmdDid(pastDate(args[1]), desc)
	case "log":
		cmdLog(text)
	case "log-hook":
		cmdLogHook()
	case "shell-init":
		cmdShellInit()
	case "setup-claude-hook":
		cmdSetupClaudeHook()
	case "radd", "rjournal":
		cmdRecord("journal")
	case "raddi", "rimportant":
		cmdRecord("important")
	case "rremember":
		cmdRecord("remember")
	case "rtask":
		cmdRecord("task")
	case "addi":
		requireText()
		cmdDated("important.md", "Important", "important", text)
	case "remember":
		if strings.TrimSpace(text) == "" {
			if isTTY() {
				runNotesTUI(rememberCfg)
			} else {
				cmdShow("remember")
			}
			return
		}
		cmdDated("remember.md", "Remember", "remember", text)
	case "journal":
		if isTTY() {
			runBrowser(journalBrowser())
		} else {
			cmdShow("journal")
		}
	case "shell":
		if isTTY() {
			runBrowser(shellBrowser())
		} else {
			cmdShowShell("")
		}
	case "important":
		if isTTY() {
			runBrowser(importantBrowser())
		} else {
			cmdShow("important")
		}
	case "task":
		cmdTask(args[1:])
	case "settings", "config":
		if isTTY() {
			runSettingsTUI()
		} else {
			cmdShowSettings()
		}
	case "cache":
		force := ""
		if len(args) > 1 {
			if !dateRe.MatchString(args[1]) {
				fatal("usage: notie cache [YYYY-MM-DD]")
			}
			force = args[1]
		}
		cmdCache(force)
	case "show":
		what := "journal"
		if len(args) > 1 {
			what = args[1]
		}
		if what == "shell" {
			d := ""
			if len(args) > 2 && dateRe.MatchString(args[2]) {
				d = args[2]
			}
			cmdShowShell(d)
			return
		}
		cmdShow(what)
	case "upgrade":
		cmdUpgrade(args[1:])
	case "version", "--version":
		cmdVersion()
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "notie: unknown command %q\n", args[0])
		usage()
		os.Exit(1)
	}
}
