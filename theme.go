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

	cGreen   = "\x1b[38;5;114m"
	cYellow  = "\x1b[38;5;179m"
	cMagenta = "\x1b[38;5;176m"
	cRed     = "\x1b[38;5;174m"
	cGrey    = "\x1b[38;5;243m"

	cTitle  = "\x1b[48;5;237;38;5;223;1m" // title bar: warm text on grey
	cCursor = "\x1b[48;5;236m"            // selected-row background
	cHit    = "\x1b[48;5;58;38;5;229m"    // search-match highlight
)

// Icons are single-cell unicode glyphs (no nerd font needed). They are vars,
// not consts, so the settings page can switch icon packs live — applyIcons
// repoints them all at once and applyConfig loads the chosen pack at startup.
var (
	iTask      = "○"
	iTaskDone  = "✔"
	iJournal   = "◉"
	iStar      = "★"
	iDiamond   = "◆"
	iCursor    = "❯"
	iDate      = "▸"
	iBullet    = "•"
	iEdit      = "✎"
	iNote      = "⋮" // task has a description / details
	iBar       = "▎" // focused-entry left marker
	iAgent     = "»" // shell command run by an agent (e.g. Claude Code)
	iToggleOn  = "◉"
	iToggleOff = "○"
	iSettings  = "≡"
)

// iconSet is a named pack of every glyph the UI draws.
type iconSet struct {
	name                                                 string
	task, taskDone, journal, star, diamond, cursor, date string
	bullet, edit, note, bar, agent                       string
	toggleOn, toggleOff, settings                        string
}

// iconSets are the selectable packs (ui.icons). All glyphs are single-cell BMP
// characters so columns stay aligned on any terminal without a nerd font.
var iconSets = []iconSet{
	{
		name: "classic", task: "○", taskDone: "✔", journal: "◉", star: "★", diamond: "◆",
		cursor: "❯", date: "▸", bullet: "•", edit: "✎", note: "⋮", bar: "▎", agent: "»",
		toggleOn: "◉", toggleOff: "○", settings: "≡",
	},
	{
		name: "minimal", task: "◦", taskDone: "✓", journal: "▪", star: "✧", diamond: "◇",
		cursor: "›", date: "·", bullet: "·", edit: "✎", note: "⋯", bar: "▏", agent: "›",
		toggleOn: "▣", toggleOff: "▢", settings: "≡",
	},
	{
		name: "bold", task: "●", taskDone: "✔", journal: "◉", star: "★", diamond: "◆",
		cursor: "➤", date: "▹", bullet: "◦", edit: "✐", note: "⁝", bar: "▌", agent: "»",
		toggleOn: "●", toggleOff: "○", settings: "≣",
	},
	{
		name: "arrows", task: "▹", taskDone: "▸", journal: "❖", star: "✦", diamond: "❖",
		cursor: "→", date: "»", bullet: "‣", edit: "✏", note: "⁞", bar: "▍", agent: "↳",
		toggleOn: "◆", toggleOff: "◇", settings: "❖",
	},
}

// applyIcons repoints every icon var at the given pack.
func applyIcons(s iconSet) {
	iTask, iTaskDone, iJournal, iStar, iDiamond = s.task, s.taskDone, s.journal, s.star, s.diamond
	iCursor, iDate, iBullet, iEdit, iNote = s.cursor, s.date, s.bullet, s.edit, s.note
	iBar, iAgent, iToggleOn, iToggleOff, iSettings = s.bar, s.agent, s.toggleOn, s.toggleOff, s.settings
}

// iconNames lists the selectable packs by name, for the settings cycler.
func iconNames() []string {
	out := make([]string, len(iconSets))
	for i, s := range iconSets {
		out[i] = s.name
	}
	return out
}

// setIcons persists the named pack and repoints the icons live so a change in
// the settings TUI shows immediately. Unknown names are ignored.
func setIcons(name string) {
	for _, s := range iconSets {
		if s.name == name {
			applyIcons(s)
			configSet("ui.icons", name)
			return
		}
	}
}

// cAccent is the UI accent color. It is a var, not a const, so the settings TUI
// can recolor the interface live; applyConfig sets it from ui.accent at startup.
var cAccent = "\x1b[38;5;110m" // soft blue

// accentPalette lists the selectable UI accents by friendly name.
var accentPalette = []struct{ name, code string }{
	{"blue", "\x1b[38;5;110m"},
	{"green", "\x1b[38;5;114m"},
	{"magenta", "\x1b[38;5;176m"},
	{"yellow", "\x1b[38;5;179m"},
	{"red", "\x1b[38;5;174m"},
	{"cyan", "\x1b[38;5;80m"},
	{"orange", "\x1b[38;5;173m"},
}

// caretStyle is the text-cursor block used by the footer line editor: the
// accent color as a background with dark text, so the caret is clearly visible
// while typing (brighter than the dim selected-row background).
func caretStyle() string {
	return strings.Replace(cAccent, "38;5;", "48;5;", 1) + "\x1b[38;5;235m"
}

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
