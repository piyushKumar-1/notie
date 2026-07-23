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
	pending  byte    // 'd' or 'g' chord state
	input    *string // non-nil while typing (add or search)
	inputCh  byte    // 'a' or '/'
	search   string
	status   string
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
	// plain layout: "❯ ○ #12 !0 desc · date"
	fixed := 2 + 2 + runeLen("#"+m[2]) + 1 + priW + runeLen(meta)
	desc := truncRunes(m[4], max(4, cols-fixed))
	width := fixed + runeLen(desc)
	styled := marker + icon + " " + cAccent + "#" + m[2] + cReset + " " + pri +
		descStyle + highlight(desc, t.search, descStyle) + cReset +
		cGrey + meta + cReset
	return styled, width
}

func (t *taskTUI) render() {
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
	case t.input != nil && t.inputCh == 'a':
		b.WriteString(cYellow + " + " + cReset + *t.input + cCursor + " " + cReset)
		if *t.input == "" {
			b.WriteString(cGrey + " <0|1|2> text — priority first" + cReset)
		}
	case t.input != nil && t.inputCh == '/':
		b.WriteString(cAccent + " /" + cReset + *t.input + cCursor + " " + cReset)
	case t.input != nil && t.inputCh == ':':
		b.WriteString(cYellow + " :" + cReset + *t.input + cCursor + " " + cReset)
	case t.pending == 'd':
		b.WriteString(cRed + " d… delete? press d again" + cReset)
	case t.status != "":
		b.WriteString(" " + t.status)
	case t.search != "":
		b.WriteString(cAccent + " /" + t.search + cReset + cGrey + "  n/N next/prev · esc clear" + cReset)
	default:
		b.WriteString(cGrey + " j/k move · x toggle · 0-2 pri · . done · dd del · a add · / search · q quit" + cReset)
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
	t.lines = append(t.lines[:i], t.lines[i+1:]...)
	t.save()
	t.status = cRed + "deleted" + cReset
}

// addTask parses "<0|1|2> description" — priority is mandatory.
func (t *taskTUI) addTask(in string) {
	in = strings.TrimSpace(in)
	if in == "" {
		return
	}
	pri, desc := "", ""
	if len(in) > 1 && in[0] >= '0' && in[0] <= '2' && in[1] == ' ' {
		pri, desc = in[:1], strings.TrimSpace(in[1:])
	}
	if pri == "" || desc == "" {
		t.status = cRed + "priority required — type: <0|1|2> text" + cReset
		return
	}
	id := nextID()
	appendLine(taskPath(), "Tasks", fmt.Sprintf("- [ ] #%d !%s %s (added %s)", id, pri, desc, today()))
	t.reload()
	t.cursorTo(fmt.Sprintf("%d", id))
	t.status = cGreen + fmt.Sprintf("added #%d !%s", id, pri) + cReset
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
	t.status = cGrey + "#" + id + " → !" + string(p) + cReset
}

// findNext moves the cursor to the next/prev task matching the search.
func (t *taskTUI) findNext(dir int) {
	if t.search == "" || len(t.tasks) == 0 {
		return
	}
	n := len(t.tasks)
	for step := 1; step <= n; step++ {
		vi := ((t.cursor+dir*step)%n + n) % n
		if containsFold(t.lines[t.tasks[vi]], t.search) {
			t.cursor = vi
			return
		}
	}
	t.status = cGrey + "no match for /" + t.search + cReset
}

func runTaskTUI() {
	old, err := enterRaw()
	if err != nil {
		printTasks() // no terminal control available — plain fallback
		return
	}
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l")
	defer func() {
		os.Stdout.WriteString("\x1b[?1049l\x1b[?25h")
		restoreTerm(old)
	}()

	t := &taskTUI{}
	t.reload()
	r := bufio.NewReader(os.Stdin)
	for {
		t.render()
		c, err := readKey(r)
		if err != nil {
			return
		}

		if t.input != nil {
			switch {
			case c == 13 || c == 10:
				val := strings.TrimSpace(*t.input)
				kind := t.inputCh
				t.input = nil
				if kind == 'a' {
					t.addTask(val)
				} else if kind == '/' {
					t.search = val
					t.findNext(1)
				} else if kind == ':' {
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
			case c == 27:
				t.input = nil
			case c == 127 || c == 8:
				if len(*t.input) > 0 {
					*t.input = (*t.input)[:len(*t.input)-1]
				}
			case c >= 32 && c < 127:
				*t.input += string(c)
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
			t.pending = 'd'
		case 'a', 'o':
			s := ""
			t.input, t.inputCh = &s, 'a'
		case '/':
			s := ""
			t.input, t.inputCh = &s, '/'
		case ':':
			s := ""
			t.input, t.inputCh = &s, ':'
		case 'n':
			t.findNext(1)
		case 'N':
			t.findNext(-1)
		}
	}
}
