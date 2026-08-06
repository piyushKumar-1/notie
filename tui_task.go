// Interactive task list: vim keys, themed rendering.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

var taskFullRe = regexp.MustCompile(
	`^- \[([ x])\] #(\d+)(?: !([0-2]))? (.*?)(?: \(added (\d{4}-\d{2}-\d{2})\))?(?: \(done (\d{4}-\d{2}-\d{2})\))?$`)

type taskTUI struct {
	lines    []string // full file content
	tasks    []int    // indices into lines that are task lines
	showDone bool     // '.' toggles; done tasks hidden by default
	cursor   int
	offset   int
	pending  byte       // 'd' or 'g' chord state
	in       *lineInput // non-nil while typing (add, search, command or edit)
	search   string
	status   string

	detail    bool   // detail pane open (Enter on a task)
	detailID  string // id of the task shown in the detail pane
	detailSum string // that task's one-line summary (for the pane header)
	detailOff int    // scroll offset within the detail pane
}

// reload rebuilds the visible task list: grouped by priority (0·1·2, unmarked
// legacy tasks last), oldest first within a group, done tasks filtered out
// unless showDone.
func (t *taskTUI) reload() {
	t.lines = readLines(taskPath())
	t.tasks = t.tasks[:0]
	for i, l := range t.lines {
		if !taskRe.MatchString(l) {
			continue
		}
		if !t.showDone && strings.HasPrefix(l, "- [x]") {
			continue
		}
		t.tasks = append(t.tasks, i)
	}
	sort.SliceStable(t.tasks, func(i, j int) bool {
		return taskPri(t.lines[t.tasks[i]]) < taskPri(t.lines[t.tasks[j]])
	})
	if t.cursor >= len(t.tasks) {
		t.cursor = len(t.tasks) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
	}
}

// cursorTo moves the cursor to task #id's row, if visible.
func (t *taskTUI) cursorTo(id string) {
	for vi, li := range t.tasks {
		if m := taskRe.FindStringSubmatch(t.lines[li]); m != nil && m[1] == id {
			t.cursor = vi
			return
		}
	}
}

func (t *taskTUI) save() {
	writeLines(taskPath(), t.lines)
	t.reload()
}

// row renders one task line; returns styled text and its escape-free width.
func (t *taskTUI) row(l string, selected bool, cols int) (string, int) {
	m := taskFullRe.FindStringSubmatch(l)
	if m == nil {
		return truncRunes(l, cols), runeLen(truncRunes(l, cols))
	}
	done := m[1] == "x"
	icon, descStyle := cYellow+iTask+cReset, ""
	if done {
		icon, descStyle = cGreen+iTaskDone+cReset, cGrey+cStrike
	}
	meta := ""
	if m[5] != "" {
		meta = " · " + m[5]
	}
	if done && m[6] != "" {
		meta += " " + iTaskDone + " " + m[6]
	}
	// a trailing ⋮ marks tasks that carry a description (press ↵ to read it).
	if m[2] != "" && hasTaskDetail(m[2]) {
		meta += " " + iNote
	}
	pri, priW := "", 0
	if m[3] != "" {
		pc := cGrey
		switch {
		case done:
		case m[3] == "0":
			pc = cRed
		case m[3] == "1":
			pc = cYellow
		}
		pri, priW = pc+"!"+m[3]+cReset+" ", 3
	}
	marker := "  "
	if selected {
		marker = cAccent + cBold + iCursor + cReset + " "
	}
	// plain layout: "❯ ○ !0 desc · date" — ids stay in the file (and in
	// `notie task done <id>`), but are noise in the list.
	fixed := 2 + 2 + priW + runeLen(meta)
	desc := truncRunes(m[4], max(4, cols-fixed))
	width := fixed + runeLen(desc)
	styled := marker + icon + " " + pri +
		descStyle + highlight(desc, t.search, descStyle) + cReset +
		cGrey + meta + cReset
	return styled, width
}

func (t *taskTUI) render() {
	if t.detail {
		t.renderDetail()
		return
	}
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	rows, cols := termSize()

	open, done := 0, 0
	for _, l := range t.lines {
		if !taskRe.MatchString(l) {
			continue
		}
		if strings.HasPrefix(l, "- [x]") {
			done++
		} else {
			open++
		}
	}
	right := fmt.Sprintf("%d open · %d done", open, done)
	if !t.showDone && done > 0 {
		right += " (hidden)"
	}
	b.WriteString(titleBar(cols, iTaskDone+" notie · tasks", right) + "\r\n")

	avail := max(1, rows-2)
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+avail {
		t.offset = t.cursor - avail + 1
	}
	if len(t.tasks) == 0 {
		if !t.showDone && done > 0 {
			b.WriteString("\r\n  " + cGrey + "all tasks done — press " + cReset + cGreen + "." + cReset + cGrey + " to show them" + cReset + "\r\n")
		} else {
			b.WriteString("\r\n  " + cGrey + "no tasks — press " + cReset + cYellow + "a" + cReset + cGrey + " to add one" + cReset + "\r\n")
		}
	}
	for vi := t.offset; vi < len(t.tasks) && vi < t.offset+avail; vi++ {
		styled, w := t.row(t.lines[t.tasks[vi]], vi == t.cursor, cols)
		if vi == t.cursor {
			b.WriteString(cursorRow(styled, w, cols) + "\r\n")
		} else {
			b.WriteString(styled + "\r\n")
		}
	}

	b.WriteString(fmt.Sprintf("\x1b[%d;1H", rows))
	switch {
	case t.in != nil && t.in.kind == 'a':
		b.WriteString(inputBar(cYellow+" + "+cReset, t.in.buf))
		if t.in.buf == "" {
			b.WriteString(cGrey + " text — prefix 0|1|2 to set priority (default " + defaultPri() + ")" + cReset)
		}
	case t.in != nil && t.in.kind == 'e':
		b.WriteString(inputBar(cYellow+" "+iEdit+" "+cReset, t.in.buf))
	case t.in != nil && t.in.kind == '/':
		b.WriteString(inputBar(cAccent+" /"+cReset, t.in.buf))
	case t.in != nil && t.in.kind == ':':
		b.WriteString(inputBar(cYellow+" :"+cReset, t.in.buf))
	case t.pending == 'd':
		b.WriteString(cRed + " d… delete? press d again" + cReset)
	case t.status != "":
		b.WriteString(" " + t.status)
	case t.search != "":
		b.WriteString(cAccent + " /" + t.search + cReset + cGrey + "  n/N next/prev · esc clear" + cReset)
	default:
		b.WriteString(cGrey + " j/k move · x toggle · 0-2 pri · ↵ details · e edit · dd del · yy copy · a add · / search · q quit" + cReset)
	}
	os.Stdout.WriteString(b.String())
}

// openDetail opens the detail pane for the selected task.
func (t *taskTUI) openDetail() {
	if len(t.tasks) == 0 {
		return
	}
	m := taskFullRe.FindStringSubmatch(t.lines[t.tasks[t.cursor]])
	if m == nil {
		return
	}
	t.detail, t.detailID, t.detailSum, t.detailOff = true, m[2], m[4], 0
}

// renderDetail draws the detail pane: the task summary, then its description
// wrapped and vertically scrollable.
func (t *taskTUI) renderDetail() {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	rows, cols := termSize()
	b.WriteString(titleBar(cols, iTaskDone+" notie · task details", "#"+t.detailID) + "\r\n")
	b.WriteString("  " + cBold + truncRunes(t.detailSum, max(4, cols-4)) + cReset + "\r\n")

	var body []string
	if raw := readTaskDetail(t.detailID); strings.TrimSpace(raw) == "" {
		body = []string{cGrey + "no details yet — press e to add" + cReset}
	} else {
		for _, ln := range strings.Split(raw, "\n") {
			if ln == "" {
				body = append(body, "")
				continue
			}
			body = append(body, wrapRunes(ln, max(4, cols-4))...)
		}
	}

	avail := max(1, rows-3) // title + summary + footer
	maxOff := max(0, len(body)-avail)
	if t.detailOff > maxOff {
		t.detailOff = maxOff
	}
	if t.detailOff < 0 {
		t.detailOff = 0
	}
	for i := t.detailOff; i < len(body) && i < t.detailOff+avail; i++ {
		b.WriteString("  " + body[i] + cReset + "\r\n")
	}

	b.WriteString(fmt.Sprintf("\x1b[%d;1H", rows))
	if t.status != "" {
		b.WriteString(" " + t.status)
	} else {
		b.WriteString(cGrey + " j/k scroll · e edit · q back" + cReset)
	}
	os.Stdout.WriteString(b.String())
}

func (t *taskTUI) toggle() {
	if len(t.tasks) == 0 {
		return
	}
	i := t.tasks[t.cursor]
	l := t.lines[i]
	if strings.HasPrefix(l, "- [x]") {
		l = strings.Replace(l, "- [x]", "- [ ]", 1)
		l = doneStampRe.ReplaceAllString(l, "")
	} else {
		l = strings.Replace(l, "- [ ]", "- [x]", 1)
		if !doneStampRe.MatchString(l) {
			l += " (done " + today() + ")"
		}
	}
	t.lines[i] = l
	t.save()
}

func (t *taskTUI) delete() {
	if len(t.tasks) == 0 {
		return
	}
	i := t.tasks[t.cursor]
	if m := taskRe.FindStringSubmatch(t.lines[i]); m != nil {
		removeTaskDetail(m[1])
	}
	t.lines = append(t.lines[:i], t.lines[i+1:]...)
	t.save()
	t.status = cRed + "deleted" + cReset
}

// addTask parses "[0|1|2] description" — a leading digit sets the priority,
// otherwise the task takes the default.
func (t *taskTUI) addTask(in string) {
	in = strings.TrimSpace(in)
	if in == "" {
		return
	}
	pri, desc := defaultPri(), in
	if len(in) > 1 && in[0] >= '0' && in[0] <= '2' && in[1] == ' ' {
		pri, desc = in[:1], strings.TrimSpace(in[1:])
	}
	if desc == "" {
		return
	}
	id := nextID()
	appendLine(taskPath(), "Tasks", fmt.Sprintf("- [ ] #%d !%s %s (added %s)", id, pri, desc, today()))
	t.reload()
	t.cursorTo(fmt.Sprintf("%d", id))
	t.status = cGreen + "added !" + pri + cReset
}

// setPri rewrites the selected task's priority marker and follows the task
// to its new group position.
func (t *taskTUI) setPri(p byte) {
	if len(t.tasks) == 0 {
		return
	}
	i := t.tasks[t.cursor]
	head := taskRe.FindString(t.lines[i]) // "- [ ] #12 "
	if head == "" {
		return
	}
	rest := t.lines[i][len(head):]
	if len(rest) > 2 && rest[0] == '!' && rest[1] >= '0' && rest[1] <= '2' && rest[2] == ' ' {
		rest = rest[3:]
	}
	t.lines[i] = head + "!" + string(p) + " " + rest
	id := taskRe.FindStringSubmatch(t.lines[i])[1]
	t.save()
	t.cursorTo(id)
	t.status = cGrey + "priority → !" + string(p) + cReset
}

// editDesc rewrites the selected task's description, preserving its id,
// priority and (added/done) stamps.
func (t *taskTUI) editDesc(text string) {
	if len(t.tasks) == 0 || text == "" {
		return
	}
	m := taskFullRe.FindStringSubmatch(t.lines[t.tasks[t.cursor]])
	if m == nil {
		return
	}
	line := fmt.Sprintf("- [%s] #%s", m[1], m[2])
	if m[3] != "" {
		line += " !" + m[3]
	}
	line += " " + text
	if m[5] != "" {
		line += " (added " + m[5] + ")"
	}
	if m[6] != "" {
		line += " (done " + m[6] + ")"
	}
	t.lines[t.tasks[t.cursor]] = line
	t.save()
	t.cursorTo(m[2])
	t.status = cGreen + "updated" + cReset
}

// yank copies the selected task's description to the clipboard.
func (t *taskTUI) yank() {
	if len(t.tasks) == 0 {
		return
	}
	l := t.lines[t.tasks[t.cursor]]
	if m := taskFullRe.FindStringSubmatch(l); m != nil {
		l = m[4]
	}
	yank(&t.status, l)
}

// findNext moves the cursor to the next/prev task matching the search.
func (t *taskTUI) findNext(dir int) {
	if t.search == "" || len(t.tasks) == 0 {
		return
	}
	if i, ok := cycle(len(t.tasks), t.cursor, dir, func(i int) bool {
		return containsFold(t.lines[t.tasks[i]], t.search)
	}); ok {
		t.cursor = i
		return
	}
	t.status = cGrey + "no match for /" + t.search + cReset
}

func runTaskTUI() {
	t := &taskTUI{showDone: !collapseDone()}
	t.reload()
	runScreen(printTasks, func(r *bufio.Reader) {
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
					kind, val := t.in.kind, strings.TrimSpace(t.in.buf)
					t.in = nil
					switch kind {
					case 'a':
						t.addTask(val)
					case 'e':
						t.editDesc(val)
					case '/':
						t.search = val
						t.findNext(1)
					case ':':
						switch cmd, arg := splitCmd(val); cmd {
						case "q", "quit":
							return
						case "ff", "fg":
							t.search = arg
							t.findNext(1)
						case "":
						default:
							t.status = cRed + "unknown command :" + cmd + cReset
						}
					}
				}
				continue
			}

			// Detail pane: scroll, edit in $EDITOR, or back to the list.
			if t.detail {
				t.status = ""
				switch c {
				case 'q', 27, 3, 'h':
					t.detail = false
				case 'j':
					t.detailOff++
				case 'k':
					t.detailOff--
				case 4: // ctrl-d
					t.detailOff += 5
				case 21: // ctrl-u
					t.detailOff -= 5
				case 'g':
					t.detailOff = 0
				case 'G':
					t.detailOff = 1 << 30 // clamped to the bottom in renderDetail
				case 'e', 'i', 13, 10, 'l':
					if out, ok := editText(r, t.detailSum, readTaskDetail(t.detailID)); ok {
						writeTaskDetail(t.detailID, out)
						t.detailOff = 0
						t.status = cGreen + "details saved" + cReset
					}
				}
				continue
			}

			t.status = ""
			if t.pending == 'd' {
				t.pending = 0
				if c == 'd' {
					t.delete()
				}
				continue
			}
			if t.pending == 'g' {
				t.pending = 0
				if c == 'g' {
					t.cursor = 0
				}
				continue
			}
			if t.pending == 'y' {
				t.pending = 0
				if c == 'y' {
					t.yank()
				}
				continue
			}

			switch c {
			case 'q', 3:
				return
			case 27:
				t.search = ""
			case 'j':
				if t.cursor < len(t.tasks)-1 {
					t.cursor++
				}
			case 'k':
				if t.cursor > 0 {
					t.cursor--
				}
			case 'g':
				t.pending = 'g'
			case 'G':
				if len(t.tasks) > 0 {
					t.cursor = len(t.tasks) - 1
				}
			case 'x', ' ':
				t.toggle()
			case '0', '1', '2':
				t.setPri(c)
			case '.':
				t.showDone = !t.showDone
				t.reload()
				if t.showDone {
					t.status = cGrey + "showing done" + cReset
				} else {
					t.status = cGrey + "hiding done" + cReset
				}
			case 'd':
				if confirmDelete() {
					t.pending = 'd'
				} else {
					t.delete()
				}
			case 'y':
				t.pending = 'y'
			case 'e':
				if len(t.tasks) > 0 {
					m := taskFullRe.FindStringSubmatch(t.lines[t.tasks[t.cursor]])
					if m != nil {
						t.in = &lineInput{kind: 'e', buf: m[4]}
					}
				}
			case 13, 10: // Enter — open the task's details
				t.openDetail()
			case 'a', 'o':
				t.in = &lineInput{kind: 'a'}
			case '/':
				t.in = &lineInput{kind: '/'}
			case ':':
				t.in = &lineInput{kind: ':'}
			case 'n':
				t.findNext(1)
			case 'N':
				t.findNext(-1)
			}
		}
	})
}
