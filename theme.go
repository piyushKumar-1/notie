// Theme: 256-color ANSI palette, icons, and small text helpers shared by
// all the TUIs. Icons are single-cell unicode glyphs (no nerd font needed).
package main

import (
	"regexp"
	"strings"
)

const (
	cReset  = "\x1b[0m"
	cBold   = "\x1b[1m"
	cItalic = "\x1b[3m"
	cStrike = "\x1b[9m"

	cAccent  = "\x1b[38;5;110m" // soft blue
	cGreen   = "\x1b[38;5;114m"
	cYellow  = "\x1b[38;5;179m"
	cMagenta = "\x1b[38;5;176m"
	cRed     = "\x1b[38;5;174m"
	cGrey    = "\x1b[38;5;243m"

	cTitle  = "\x1b[48;5;237;38;5;223;1m" // title bar: warm text on grey
	cCursor = "\x1b[48;5;236m"            // selected-row background
	cHit    = "\x1b[48;5;58;38;5;229m"    // search-match highlight

	iTask     = "○"
	iTaskDone = "✔"
	iJournal  = "◉"
	iStar     = "★"
	iDiamond  = "◆"
	iCursor   = "❯"
	iDate     = "▸"
	iBullet   = "•"
)

func runeLen(s string) int { return len([]rune(s)) }

func truncRunes(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return string(r[:w])
	}
	return string(r[:w-1]) + "…"
}

func padTo(s string, w int) string {
	if n := w - runeLen(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// wrapRunes soft-wraps s into chunks of at most w runes, breaking on spaces
// where possible.
func wrapRunes(s string, w int) []string {
	if w < 4 {
		w = 4
	}
	var out []string
	r := []rune(s)
	for len(r) > w {
		cut := w
		for i := w; i > w/2; i-- {
			if r[i-1] == ' ' {
				cut = i
				break
			}
		}
		out = append(out, strings.TrimRight(string(r[:cut]), " "))
		r = r[cut:]
	}
	return append(out, string(r))
}

// highlight wraps case-insensitive occurrences of query in the cHit style,
// restoring `restore` afterwards so surrounding styling survives.
func highlight(s, query, restore string) string {
	if query == "" {
		return s
	}
	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if err != nil {
		return s
	}
	return re.ReplaceAllStringFunc(s, func(m string) string {
		return cHit + m + cReset + restore
	})
}

// splitCmd splits a ":" command line into its name and argument.
func splitCmd(val string) (string, string) {
	if i := strings.IndexByte(val, ' '); i >= 0 {
		return val[:i], strings.TrimSpace(val[i+1:])
	}
	return val, ""
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

// titleBar renders a full-width colored bar: "left ... right".
func titleBar(cols int, left, right string) string {
	plain := " " + left
	pad := cols - runeLen(plain) - runeLen(right) - 1
	if pad < 1 {
		return cTitle + padTo(truncRunes(plain, cols), cols) + cReset
	}
	return cTitle + plain + strings.Repeat(" ", pad) + right + " " + cReset
}

// cursorRow re-bases a styled string onto the selected-row background and
// pads it to cols. plainWidth must be the escape-free width of `styled`.
func cursorRow(styled string, plainWidth, cols int) string {
	s := cCursor + strings.ReplaceAll(styled, cReset, cReset+cCursor)
	if n := cols - plainWidth; n > 0 {
		s += strings.Repeat(" ", n)
	}
	return s + cReset
}
