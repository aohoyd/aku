package theme

import (
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"gopkg.in/yaml.v3"
)

// Color is a string representing a color value (ANSI 256 index or hex like "#FF5555").
// It implements color.Color so it can be passed directly to lipgloss style methods.
type Color string

// RGBA satisfies the image/color.Color interface so theme colors can be
// passed directly to lipgloss style methods like Foreground() and Background().
func (c Color) RGBA() (r, g, b, a uint32) {
	return lipgloss.Color(string(c)).RGBA()
}

// UI colors — chrome, borders, text.
var (
	Accent       Color = "#957FB8" // Oniviolet — focused borders, badges, selection bg
	Muted        Color = "#727169" // FujiGrey — unfocused borders, help text, status bar
	Highlight    Color = "#C8C093" // OldWhite — titles, keys, cursor, table header
	TextOnAccent Color = "#1F1F28" // SumiInk0 — text on accent backgrounds
	Error        Color = "#E82424" // SamuraiRed — error messages
	Warning      Color = "#FF9E3B" // RoninYellow — warning messages
	Subtle       Color = "#54546D" // SumiInk4 — indicators, YAML markers
	Prompt       Color = "#727169" // FujiGrey — input prompt labels in overlays
	Selection    Color = "#FF9E3B" // RoninYellow — multi-select marker foreground
)

// Status colors — Kubernetes resource health.
var (
	StatusRunning   Color = "#76946A" // AutumnGreen — Running, Ready, Bound, Active
	StatusSucceeded Color = "#727169" // FujiGrey — Succeeded, Completed
	StatusPending   Color = "#FF9E3B" // RoninYellow — Pending, Waiting, Warning
	StatusFailed    Color = "#E82424" // SamuraiRed — Failed, CrashLoopBackOff, Error
)

// Syntax colors — YAML and describe view highlighting.
var (
	SyntaxKey    Color = "#7E9CD8" // CrystalBlue — YAML keys, describe keys
	SyntaxString Color = "#98BB6C" // SpringGreen — YAML string values
	SyntaxNumber Color = "#D27E99" // SakuraPink — YAML numbers
	SyntaxBool   Color = "#957FB8" // Oniviolet — YAML booleans
	SyntaxNull   Color = "#727169" // FujiGrey — YAML null
	SyntaxMarker Color = "#54546D" // SumiInk4 — YAML markers
	SyntaxValue  Color = "#DCD7BA" // FujiWhite — describe plain values
)

// Search colors — search match highlighting.
var (
	SearchMatch    Color = "#FF9E3B" // RoninYellow — search highlight bg
	SearchSelected Color = "#957FB8" // Oniviolet — selected search match bg
	SearchFg       Color = "#1F1F28" // SumiInk0 — search match fg
)

// LogHighlightRule defines a word/regex highlight for log views.
type LogHighlightRule struct {
	Pattern string `yaml:"pattern"`
	Fg      string `yaml:"fg"`
	Bg      string `yaml:"bg"`
	Bold    bool   `yaml:"bold"`
}

// Log highlight rules loaded from theme.
var LogHighlights []LogHighlightRule

func init() {
	_ = Load(ThemePath())
}

// ThemePath returns the default theme file path.
func ThemePath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aku", "theme.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aku", "theme.yaml")
}

// Load reads a YAML theme file and overwrites only the colors present in it.
// If the file does not exist, the defaults are kept.
func Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var f themeFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return err
	}

	if f.UI != nil {
		setIf(&Accent, f.UI.Accent)
		setIf(&Muted, f.UI.Muted)
		setIf(&Highlight, f.UI.Highlight)
		setIf(&TextOnAccent, f.UI.TextOnAccent)
		setIf(&Error, f.UI.Error)
		setIf(&Warning, f.UI.Warning)
		setIf(&Subtle, f.UI.Subtle)
		setIf(&Prompt, f.UI.Prompt)
		setIf(&Selection, f.UI.Selection)
	}
	if f.Status != nil {
		setIf(&StatusRunning, f.Status.Running)
		setIf(&StatusSucceeded, f.Status.Succeeded)
		setIf(&StatusPending, f.Status.Pending)
		setIf(&StatusFailed, f.Status.Failed)
	}
	if f.Syntax != nil {
		setIf(&SyntaxKey, f.Syntax.Key)
		setIf(&SyntaxString, f.Syntax.String)
		setIf(&SyntaxNumber, f.Syntax.Number)
		setIf(&SyntaxBool, f.Syntax.Bool)
		setIf(&SyntaxNull, f.Syntax.Null)
		setIf(&SyntaxMarker, f.Syntax.Marker)
		setIf(&SyntaxValue, f.Syntax.Value)
	}
	if f.Search != nil {
		setIf(&SearchMatch, f.Search.Match)
		setIf(&SearchSelected, f.Search.Selected)
		setIf(&SearchFg, f.Search.Fg)
	}
	if len(f.Highlights) > 0 {
		LogHighlights = make([]LogHighlightRule, 0, len(f.Highlights))
		for _, h := range f.Highlights {
			if h.Pattern == nil {
				continue
			}
			rule := LogHighlightRule{Pattern: *h.Pattern, Bold: h.Bold}
			if h.Fg != nil {
				rule.Fg = *h.Fg
			}
			if h.Bg != nil {
				rule.Bg = *h.Bg
			}
			LogHighlights = append(LogHighlights, rule)
		}
	}
	return nil
}

func setIf(target *Color, value *string) {
	if value != nil {
		*target = Color(*value)
	}
}

type themeFile struct {
	UI         *uiColors        `yaml:"ui"`
	Status     *statusColors    `yaml:"status"`
	Syntax     *syntaxColors    `yaml:"syntax"`
	Search     *searchColors    `yaml:"search"`
	Highlights []highlightEntry `yaml:"highlights"`
}

type uiColors struct {
	Accent       *string `yaml:"accent"`
	Muted        *string `yaml:"muted"`
	Highlight    *string `yaml:"highlight"`
	TextOnAccent *string `yaml:"text_on_accent"`
	Error        *string `yaml:"error"`
	Warning      *string `yaml:"warning"`
	Subtle       *string `yaml:"subtle"`
	Prompt       *string `yaml:"prompt"`
	Selection    *string `yaml:"selection"`
}

type statusColors struct {
	Running   *string `yaml:"running"`
	Succeeded *string `yaml:"succeeded"`
	Pending   *string `yaml:"pending"`
	Failed    *string `yaml:"failed"`
}

type syntaxColors struct {
	Key    *string `yaml:"key"`
	String *string `yaml:"string"`
	Number *string `yaml:"number"`
	Bool   *string `yaml:"bool"`
	Null   *string `yaml:"null"`
	Marker *string `yaml:"marker"`
	Value  *string `yaml:"value"`
}

type searchColors struct {
	Match    *string `yaml:"match"`
	Selected *string `yaml:"selected"`
	Fg       *string `yaml:"fg"`
}

type highlightEntry struct {
	Pattern *string `yaml:"pattern"`
	Fg      *string `yaml:"fg"`
	Bg      *string `yaml:"bg"`
	Bold    bool    `yaml:"bold"`
}
