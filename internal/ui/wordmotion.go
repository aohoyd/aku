package ui

import (
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// Sub-word cursor motion and deletion for dialog text inputs.
//
// The bubbles textinput binds alt+f/alt+b/alt+left/alt+right (and the ctrl+
// variants) to word motion, but its boundary rule only breaks on whitespace.
// Dialog values are space-free structured tokens (image refs like nginx:1.25,
// host:port, paths), so the library motion jumps the entire value. We override
// those keys and reposition using punctuation/digit-aware boundaries instead.

// charClass buckets a rune into one of three motion classes. Letters are not
// split by case, so camelCase stays a single word.
type charClass int

const (
	classOther charClass = iota // punctuation, space, symbols
	classLetter
	classDigit
)

func classOf(r rune) charClass {
	switch {
	case unicode.IsLetter(r):
		return classLetter
	case unicode.IsDigit(r):
		return classDigit
	default:
		return classOther
	}
}

// nextWordBoundary returns the index of the next sub-word boundary at or after
// pos, moving right. A boundary sits between two runes of different classes.
func nextWordBoundary(runes []rune, pos int) int {
	n := len(runes)
	if pos >= n {
		return n
	}
	pos++
	for pos < n && classOf(runes[pos-1]) == classOf(runes[pos]) {
		pos++
	}
	return pos
}

// prevWordBoundary returns the index of the previous sub-word boundary at or
// before pos, moving left. Mirror of nextWordBoundary.
func prevWordBoundary(runes []rune, pos int) int {
	if pos <= 0 {
		return 0
	}
	if pos > len(runes) {
		pos = len(runes)
	}
	pos--
	for pos > 0 && classOf(runes[pos-1]) == classOf(runes[pos]) {
		pos--
	}
	return pos
}

// wordMotionDir reports the direction of a word-motion key: +1 forward, -1
// backward, 0 if the key is not a word-motion key. Mirrors the textinput
// WordForward ("alt+right", "ctrl+right", "alt+f") and WordBackward
// ("alt+left", "ctrl+left", "alt+b") bindings.
func wordMotionDir(km tea.KeyPressMsg) int {
	alt := km.Mod.Contains(tea.ModAlt)
	ctrl := km.Mod.Contains(tea.ModCtrl)
	switch km.Code {
	case tea.KeyRight:
		if alt || ctrl {
			return 1
		}
	case tea.KeyLeft:
		if alt || ctrl {
			return -1
		}
	case 'f':
		if alt {
			return 1
		}
	case 'b':
		if alt {
			return -1
		}
	}
	return 0
}

// wordDeleteDir reports the direction of a word-delete key: +1 forward, -1
// backward, 0 if not a word-delete key. Mirrors the textinput
// DeleteWordBackward ("alt+backspace", "ctrl+w") and DeleteWordForward
// ("alt+delete", "alt+d") bindings.
func wordDeleteDir(km tea.KeyPressMsg) int {
	alt := km.Mod.Contains(tea.ModAlt)
	ctrl := km.Mod.Contains(tea.ModCtrl)
	switch km.Code {
	case tea.KeyBackspace:
		if alt {
			return -1
		}
	case tea.KeyDelete:
		if alt {
			return 1
		}
	case 'w':
		if ctrl {
			return -1
		}
	case 'd':
		if alt {
			return 1
		}
	}
	return 0
}
