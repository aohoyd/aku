package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNextWordBoundary(t *testing.T) {
	tests := []struct {
		name  string
		value string
		from  int
		want  int
	}{
		{"image ref segments", "nginx:1.25", 0, 5},   // -> after "nginx"
		{"image ref colon", "nginx:1.25", 5, 6},      // ":" -> after ":"
		{"image ref digit", "nginx:1.25", 6, 7},      // "1" -> after "1"
		{"image ref dot", "nginx:1.25", 7, 8},        // "." -> after "."
		{"image ref last digits", "nginx:1.25", 8, 10}, // "25" -> end
		{"host port", "localhost:8080", 0, 9},        // -> after "localhost"
		{"host port digits", "localhost:8080", 10, 14}, // "8080" -> end
		{"path letter", "a/b-c_d", 0, 1},
		{"path sep", "a/b-c_d", 1, 2},
		{"at end", "abc", 3, 3},
		{"empty", "", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextWordBoundary([]rune(tt.value), tt.from); got != tt.want {
				t.Errorf("nextWordBoundary(%q, %d) = %d, want %d", tt.value, tt.from, got, tt.want)
			}
		})
	}
}

func TestPrevWordBoundary(t *testing.T) {
	tests := []struct {
		name  string
		value string
		from  int
		want  int
	}{
		{"image ref from end", "nginx:1.25", 10, 8}, // -> before "25"
		{"image ref dot", "nginx:1.25", 8, 7},
		{"image ref digit", "nginx:1.25", 7, 6},
		{"image ref colon", "nginx:1.25", 6, 5},
		{"image ref letters", "nginx:1.25", 5, 0},
		{"host port from end", "localhost:8080", 14, 10},
		{"host port colon", "localhost:8080", 10, 9},
		{"at start", "abc", 0, 0},
		{"empty", "", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prevWordBoundary([]rune(tt.value), tt.from); got != tt.want {
				t.Errorf("prevWordBoundary(%q, %d) = %d, want %d", tt.value, tt.from, got, tt.want)
			}
		})
	}
}

func TestCamelCaseStaysWhole(t *testing.T) {
	// Letters are not split by case: "myAppName" is one word.
	if got := nextWordBoundary([]rune("myAppName"), 0); got != 9 {
		t.Errorf("expected camelCase to stay whole (9), got %d", got)
	}
	if got := prevWordBoundary([]rune("myAppName"), 9); got != 0 {
		t.Errorf("expected camelCase to stay whole (0), got %d", got)
	}
}

func TestWordMotionDir(t *testing.T) {
	tests := []struct {
		name string
		km   tea.KeyPressMsg
		want int
	}{
		{"alt+f", tea.KeyPressMsg{Code: 'f', Mod: tea.ModAlt}, 1},
		{"alt+b", tea.KeyPressMsg{Code: 'b', Mod: tea.ModAlt}, -1},
		{"alt+right", tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt}, 1},
		{"alt+left", tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt}, -1},
		{"ctrl+right", tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModCtrl}, 1},
		{"ctrl+left", tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl}, -1},
		{"plain right", tea.KeyPressMsg{Code: tea.KeyRight}, 0},
		{"plain left", tea.KeyPressMsg{Code: tea.KeyLeft}, 0},
		{"plain f", tea.KeyPressMsg{Code: 'f'}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wordMotionDir(tt.km); got != tt.want {
				t.Errorf("wordMotionDir(%s) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

func TestWordDeleteDir(t *testing.T) {
	tests := []struct {
		name string
		km   tea.KeyPressMsg
		want int
	}{
		{"alt+backspace", tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModAlt}, -1},
		{"ctrl+w", tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl}, -1},
		{"alt+delete", tea.KeyPressMsg{Code: tea.KeyDelete, Mod: tea.ModAlt}, 1},
		{"alt+d", tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt}, 1},
		{"plain backspace", tea.KeyPressMsg{Code: tea.KeyBackspace}, 0},
		{"plain d", tea.KeyPressMsg{Code: 'd'}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wordDeleteDir(tt.km); got != tt.want {
				t.Errorf("wordDeleteDir(%s) = %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}
