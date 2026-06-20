package ui

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/theme"
)

func TestTableHealthStyles(t *testing.T) {
	warnFg := TableHealthWarnStyle.GetForeground()
	errorFg := TableHealthErrorStyle.GetForeground()

	if warnFg != theme.StatusPending {
		t.Errorf("TableHealthWarnStyle foreground = %v, want %v", warnFg, theme.StatusPending)
	}
	if errorFg != theme.StatusFailed {
		t.Errorf("TableHealthErrorStyle foreground = %v, want %v", errorFg, theme.StatusFailed)
	}

	// Warn and error tints must be distinguishable from each other.
	if warnFg == errorFg {
		t.Errorf("warn and error foregrounds must differ, both = %v", warnFg)
	}

	// The health tint is foreground-only: no background, no bold.
	for _, tc := range []struct {
		name  string
		style lipgloss.Style
	}{
		{"TableHealthWarnStyle", TableHealthWarnStyle},
		{"TableHealthErrorStyle", TableHealthErrorStyle},
	} {
		if bg := tc.style.GetBackground(); bg != (lipgloss.NoColor{}) {
			t.Errorf("%s background = %v, want unset (NoColor)", tc.name, bg)
		}
		if tc.style.GetBold() {
			t.Errorf("%s is bold, want not bold", tc.name)
		}
	}
}
