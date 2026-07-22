// notie — tiny local notes CLI. Everything lives in ~/.notie as plain markdown.
// Standalone binary: Go stdlib only, no external commands required.
// (Exception: `notie cache` will use the `claude` CLI for nicer one-line
// summaries if it is installed, and falls back to joining entries otherwise.)
package main

import (
	"context"
	"fmt"
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
  notie log "cmd"         append a shell command to today's audit trail
                          (~/.notie/<date>/shell.md — wired to a zsh preexec hook)
  notie addi "text"       append to important.md
  notie remember "text"   append to remember.md
  notie task "text"       add a task to task.md, then show tasks
  notie radd              record voice, transcribe, append to today's journal
                          (also: rjournal · raddi/rimportant · rremember · rtask)
  notie task              interactive task list (j/k move, x toggle, dd delete, a add, q quit)
  notie task list         plain list of last 100 tasks
  notie task done <id>    mark task done
  notie task open <id>    reopen a task
  notie task del <id>     delete a task
  notie journal           interactive journal browser (dates sidebar + / search)
  notie shell             interactive shell-audit browser (same day-level UI)
  notie important         interactive important-notes browser (day-level UI)
  notie remember          interactive remember-notes list
  notie cache             build datecache.md entries for past journal days
  notie show [what]       print a file (journal|shell|remember|important|task|datecache|YYYY-MM-DD)
  notie show shell [date] print a day's shell audit trail (default today)

  TUI keys: j/k move · gg/G top/bottom · x toggle (tasks) · dd delete
            a add · / search · n/N next/prev · q quit
            :q quit · :ff <pat> find date files · :fg <pat> find mentions
`)
}

// ---- add / addi / remember ----

func cmdAdd(text string) {
	path := filepath.Join(notieDir(), today(), "journal.md")
	appendLine(path, "Journal — "+today(), fmt.Sprintf("- %s — %s", clock(), text))
	fmt.Println("journal: " + text)
}

func cmdDated(file, header, label, text string) {
	path := filepath.Join(notieDir(), file)
	appendLine(path, header, fmt.Sprintf("- %s %s — %s", today(), clock(), text))
	fmt.Println(label + ": " + text)
}

// ---- shell audit trail ----

// cmdLog appends a shell command to today's shell.md. Silent on success and
// tolerant of empty input: it is invoked by a zsh preexec hook on every
// interactive command, so it must never clutter the prompt.
func cmdLog(text string) {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return
	}
	loc := ""
	if wd, err := os.Getwd(); err == nil {
		if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(wd, home) {
			wd = "~" + strings.TrimPrefix(wd, home)
		}
		loc = " (" + wd + ")"
	}
	path := filepath.Join(notieDir(), today(), "shell.md")
	appendLine(path, "Shell — "+today(), fmt.Sprintf("- %s%s $ %s", clock(), loc, text))
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

func cmdTask(args []string) {
	if len(args) == 0 {
		if isTTY() {
			runTaskTUI()
		} else {
			printTasks()
		}
		return
	}
	if args[0] == "list" {
		printTasks()
		return
	}
	switch args[0] {
	case "done", "open", "del":
		if len(args) < 2 {
			fatal("usage: notie task %s <id>", args[0])
		}
		taskEdit(args[1], args[0])
	default:
		desc := strings.Join(args, " ")
		id := nextID()
		appendLine(taskPath(), "Tasks", fmt.Sprintf("- [ ] #%d %s (added %s)", id, desc, today()))
		fmt.Printf("added task #%d\n---\n", id)
		printTasks()
	}
}

// ---- datecache ----

var entryRe = regexp.MustCompile(`^- \d{2}:\d{2} — `)

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

// summarize produces the one-liner for a day. Prefers Claude Code (headless,
// haiku); falls back to joining the raw entries if claude is unavailable,
// errors out, or takes longer than 2 minutes.
func summarize(journalPath string) string {
	if entries := journalEntries(journalPath); len(entries) == 0 {
		return ""
	}
	if bin, err := exec.LookPath("claude"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, bin, "-p", "--model", "haiku",
			"Below is one day's work journal. Reply with EXACTLY one line (no preamble, no markdown) summarizing what was done that day, compressing related entries together.")
		if f, err := os.Open(journalPath); err == nil {
			defer f.Close()
			cmd.Stdin = f
			if out, err := cmd.Output(); err == nil {
				s := strings.Join(strings.Fields(string(out)), " ")
				if s != "" {
					return s
				}
			}
		}
	}
	return strings.Join(journalEntries(journalPath), "; ")
}

// cmdCache summarizes each past day's journal into one line in datecache.md.
// Idempotent: skips dates already cached. Catch-up safe: processes every past
// date, so missed runs (laptop asleep) are filled in on the next run.
func cmdCache() {
	dir := notieDir()
	cachePath := filepath.Join(dir, "datecache.md")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		appendLine(cachePath, "Date cache", "")
	}
	cached := map[string]bool{}
	var header, entries []string
	for _, l := range readLines(cachePath) {
		if m := dcLineRe.FindStringSubmatch(l); m != nil {
			cached[m[1]] = true
			entries = append(entries, l)
		} else if strings.TrimSpace(l) != "" || len(entries) == 0 {
			header = append(header, l)
		}
	}
	dirents, _ := os.ReadDir(dir)
	added := 0
	for _, de := range dirents {
		d := de.Name()
		if !de.IsDir() || !dateRe.MatchString(d) || d >= today() || cached[d] {
			continue
		}
		journal := filepath.Join(dir, d, "journal.md")
		if _, err := os.Stat(journal); err != nil {
			continue
		}
		if s := summarize(journal); s != "" {
			entries = append(entries, fmt.Sprintf("- %s: %s", d, s))
			added++
		}
	}
	if added > 0 {
		sort.Strings(entries)
		if len(header) == 0 || header[len(header)-1] != "" {
			header = append(header, "")
		}
		writeLines(cachePath, append(header, entries...))
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
		path := filepath.Join(dir, today(), "journal.md")
		if _, err := os.Stat(path); err != nil {
			if d := latestDateWith("journal.md"); d != "" {
				fmt.Printf("no journal for today; latest is %s:\n\n", d)
				printFile(filepath.Join(dir, d, "journal.md"), "empty")
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
			printFile(filepath.Join(dir, what, "journal.md"), "no journal for "+what)
			return
		}
		fatal("unknown file %q (journal|remember|important|task|datecache|YYYY-MM-DD)", what)
	}
}

// ---- main ----

func main() {
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
	switch args[0] {
	case "add":
		requireText()
		cmdAdd(text)
	case "log":
		cmdLog(text)
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
	case "cache":
		cmdCache()
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
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "notie: unknown command %q\n", args[0])
		usage()
		os.Exit(1)
	}
}
