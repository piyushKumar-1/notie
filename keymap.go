// Vim-style modal editing shared by the two text editors (the full-screen
// detail editor in tui_editor.go and the one-line footer input in tui.go).
// Both open in insert mode; Esc drops to normal mode where these single-key
// commands apply. The most-used bindings are remappable from the settings page
// (the keys.* config keys), so a user can pick their own layout.
package main

// vimKeys is the resolved normal-mode keymap. Bytes, since normal-mode commands
// are single printable ASCII keys.
type vimKeys struct {
	insert, appendCh, appendEol, openBelow, openAbove byte
	left, down, up, right                             byte
	lineStart, lineEnd, delChar, delLine, undo        byte
	gotoTop, gotoBottom                               byte
}

// defaultVimKeys is the standard vi layout.
func defaultVimKeys() vimKeys {
	return vimKeys{
		insert: 'i', appendCh: 'a', appendEol: 'A', openBelow: 'o', openAbove: 'O',
		left: 'h', down: 'j', up: 'k', right: 'l',
		lineStart: '0', lineEnd: '$', delChar: 'x', delLine: 'd', undo: 'u',
		gotoTop: 'g', gotoBottom: 'G',
	}
}

// firstByte returns the first byte of s, or def when s is empty.
func firstByte(s string, def byte) byte {
	if s == "" {
		return def
	}
	return s[0]
}

// loadVimKeys resolves the keymap, letting config override the handful of
// bindings the settings page exposes (undo, insert, append, open-line).
func loadVimKeys() vimKeys {
	k := defaultVimKeys()
	k.undo = firstByte(configVal("keys.undo", ""), k.undo)
	k.insert = firstByte(configVal("keys.insert", ""), k.insert)
	k.appendCh = firstByte(configVal("keys.append", ""), k.appendCh)
	k.openBelow = firstByte(configVal("keys.open_line", ""), k.openBelow)
	return k
}

// vimEnabled reports whether modal editing is on (ui.vim_mode, default on).
// When off, both editors behave as plain always-insert boxes where Esc cancels.
func vimEnabled() bool { return configVal("ui.vim_mode", "on") == "on" }
