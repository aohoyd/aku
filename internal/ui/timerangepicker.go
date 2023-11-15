package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

type timePreset struct {
	Label   string
	Seconds int64 // 0 = "all", -1 = "tail 200" (default)
}

var timePresets = []timePreset{
	{"tail 200", -1},
	{"1m", 60},
	{"5m", 300},
	{"15m", 900},
	{"30m", 1800},
	{"1h", 3600},
	{"4h", 14400},
	{"12h", 43200},
	{"24h", 86400},
	{"all", 0},
}

// TimeRangePicker is an overlay for selecting a log time range.
type TimeRangePicker struct {
	Picker[timePreset]
}

// NewTimeRangePicker creates a new time range picker with the given dimensions.
func NewTimeRangePicker(width, height int) TimeRangePicker {
	return TimeRangePicker{Picker: NewPicker(PickerConfig[timePreset]{
		Title:      "Log Time Range",
		NoItemsMsg: "(no matches)",
		MaxVisible: maxDropdownItems,
		Display:    func(p timePreset) string { return p.Label },
		Filter: func(query string, items []timePreset) []timePreset {
			if query == "" {
				return items
			}
			lower := strings.ToLower(query)
			var out []timePreset
			for _, p := range items {
				if strings.Contains(strings.ToLower(p.Label), lower) {
					out = append(out, p)
				}
			}
			return out
		},
		OnSelect: func(p timePreset) tea.Cmd {
			var sinceSeconds *int64
			if p.Seconds > 0 {
				s := p.Seconds
				sinceSeconds = &s
			}
			// Seconds == -1 means default tail 200
			// Seconds == 0 means "all" — no tail limit, no since filter
			return func() tea.Msg {
				return msgs.LogTimeRangeSelectedMsg{
					SinceSeconds: sinceSeconds,
					Label:        p.Label,
				}
			}
		},
	}, width, height)}
}

// OpenPresets opens the picker with the predefined time range presets.
func (t *TimeRangePicker) OpenPresets() {
	t.SetItems(timePresets)
	t.Open()
}

// Update handles key messages for the time range picker.
func (t TimeRangePicker) Update(msg tea.Msg) (TimeRangePicker, tea.Cmd) {
	p, cmd := t.Picker.Update(msg)
	t.Picker = p
	return t, cmd
}
