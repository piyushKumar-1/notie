// Interactive task list: vim keys, themed rendering.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var taskFullRe = regexp.MustCompile(
	`^- \[([ x])\] #(\d+) (.*?)(?: \(added (\d{4}-\d{2}-\d{2})\))?(?: \(done (\d{4}-\d{2}-\d{2})\))?$`)

type taskTUI struct {
	lines   []string // full file content
	tasks   []int    // indices into lines that are task lines
	cursor  int
	offset  int
	pending byte    // 'd' or 'g' chord state
	input   *string // non-nil while typing (add or search)
	inputCh byte    // 'a' or '/'
	search  string
	status  string
}

func (t *taskTUI) reload() {
	t.lines = readLines(taskPath())
	t.tasks = t.tasks[:0]
	for i, l := range t.lines {
		if taskRe.MatchString(l) {
			t.tasks = append(t.tasks, i)
		}
	}
	if t.cursor >= len(t.tasks) {
		t.cursor = len(t.tasks) - 1
	}
	if t.cursor < 0 {
		t.cursor = 0
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
	if m[4] != "" {
		meta = " · " + m[4]
	}
	if done && m[5] != "" {
		meta += " " + iTaskDone + " " + m[5]
	}
	marker := "  "
	if selected {
		marker = cAccent + cBold + iCursor + cReset + " "
	}
	// plain layout: "❯ ○ #12 desc · date"
	fixed := 2 + 2 + runeLen("#"+m[2]) + 1 + runeLen(meta)
	desc := truncRunes(m[3], max(4, cols-fixed))
	width := fixed + runeLen(desc)
	styled := marker + icon + " " + cAccent + "#" + m[2] + cReset + " " +
		descStyle + highlight(desc, t.search, descStyle) + cReset +
		cGrey + meta + cReset
	return styled, width
}

func (t *taskTUI) render() {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	rows, cols := termSize()

	open := 0
	for _, i := range t.tasks {
		if strings.HasPrefix(t.lines[i], "- [ ]") {
			open++
		}
	}
	b.WriteString(titleBar(cols, iTaskDone+" notie · tasks",
		fmt.Sprintf("%d open · %d done", open, len(t.tasks)-open)) + "\r\n")

	avail := max(1, rows-2)
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+avail {
		t.offset = t.cursor - avail + 1
	}
	if len(t.tasks) == 0 {
		b.WriteString("\r\n  " + cGrey + "no tasks — press " + cReset + cYellow + "a" + cReset + cGrey + " to add one" + cReset + "\r\n")
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
	case t.input != nil && t.inputCh == '/':
		b.WriteString(cAccent + " /" + cReset + *t.input + cCursor + " " + cReset)
	case t.input != nil && t.inputCh == ':':
		b.WriteString(cYellow + " :" + cReset + *t.input + cCursor + " " + cReset)
	case t.pending == 'd':
		b.WriteString(cRed + " d… delete? press d again" + cReset)
	case t.status != "":
		b.WriteString(" " + t.status)
	case t.search != "":
		b.WriteString(cAccent+" /"+t.search+cReset+cGrey+"  n/N next/prev · esc clear"+cReset)
	default:
		b.WriteString(cGrey + " j/k move · x toggle · dd delete · a add · / search · :fg grep · q/:q quit" + cReset)
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

func (t *taskTUI) addTask(desc string) {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return
	}
	id := nextID()
	appendLine(taskPath(), "Tasks", fmt.Sprintf("- [ ] #%d %s (added %s)", id, desc, today()))
	t.reload()
	t.cursor = len(t.tasks) - 1
	t.status = cGreen + fmt.Sprintf("added #%d", id) + cReset
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
