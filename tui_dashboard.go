// The landing dashboard: a home screen that lists every part of notie with a
// one-line description and the command behind it, so the whole product is
// discoverable and navigable from one place. Selecting a row opens that screen
// (reusing the existing task / journal / notes / settings TUIs) and returns
// here when it closes. `notie` with no arguments opens this in a terminal.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// dashItem is one navigable area of the app.
type dashItem struct {
	icon, label, hint, cmd string
	open                   func()
}

func dashItems() []dashItem {
	return []dashItem{
		{iTask, "Tasks", "add, prioritize and check off to-dos", "notie task", runTaskTUI},
		{iJournal, "Journal", "browse daily entries by date · / to search", "notie journal", func() { runBrowser(journalBrowser()) }},
		{iStar, "Important", "notes you flagged as worth keeping in view", "notie important", func() { runBrowser(importantBrowser()) }},
		{iDiamond, "Remember", "a running list of things to remember", "notie remember", func() { runNotesTUI(rememberCfg) }},
		{iAgent, "Shell audit", "commands you and your agents have run", "notie shell", func() { runBrowser(shellBrowser()) }},
		{iSettings, "Settings", "accent, icons, defaults, schedules, keys", "notie settings", runSettingsTUI},
	}
}

type dashboardTUI struct {
	items  []dashItem
	cursor int
}

const dashLabelW = 12

func (d *dashboardTUI) render() {
	var b strings.Builder
	b.WriteString("\x1b[H\x1b[2J")
	rows, cols := termSize()
	b.WriteString(titleBar(cols, iJournal+" notie", "dashboard") + "\r\n\r\n")
	b.WriteString("  " + cGrey + cItalic + "your notes, journal & tasks — everything lives in ~/.notie" + cReset + "\r\n\r\n")

	for i, it := range d.items {
		marker := "  "
		if i == d.cursor {
			marker = cAccent + cBold + iCursor + cReset + " "
		}
		content := " " + marker + cAccent + it.icon + cReset + "  " +
			cBold + padTo(it.label, dashLabelW) + cReset + "  " + cGrey + it.hint + cReset
		if i == d.cursor {
			plainW := 1 + 2 + runeLen(it.icon) + 2 + dashLabelW + 2 + runeLen(it.hint)
			b.WriteString(cursorRow(content, plainW, cols))
		} else {
			b.WriteString(content)
		}
		b.WriteString("\r\n")
	}

	b.WriteString(fmt.Sprintf("\x1b[%d;1H", rows))
	b.WriteString(cGrey + " j/k move · ↵ open · q quit    " +
		cItalic + "cli: " + d.items[d.cursor].cmd + cReset)
	os.Stdout.WriteString(b.String())
}

// runDashboard drives the landing menu. Each selected screen runs in its own
// alternate-screen session (sequentially, never nested) and control returns
// here afterward; q or ^C exits notie.
func runDashboard() {
	if !isTTY() {
		usage()
		return
	}
	d := &dashboardTUI{items: dashItems()}
	for {
		open := -1
		runScreen(usage, func(r *bufio.Reader) {
			for {
				d.render()
				c, err := readKey(r)
				if err != nil {
					return
				}
				switch c {
				case 'q', 3:
					return
				case 'j':
					if d.cursor < len(d.items)-1 {
						d.cursor++
					}
				case 'k':
					if d.cursor > 0 {
						d.cursor--
					}
				case 'g':
					d.cursor = 0
				case 'G':
					d.cursor = len(d.items) - 1
				case 13, 10, ' ', 'l':
					open = d.cursor
					return
				}
			}
		})
		if open < 0 {
			return
		}
		d.items[open].open()
	}
}
