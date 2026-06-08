package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"charm.land/lipgloss/v2"
	"gopkg.in/yaml.v3"

	"github.com/aohoyd/aku/internal/config"
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

	// Background is unset by default — no global background. Only set by
	// themes that want a full-screen canvas (e.g. Midnight Commander).
	Background Color = ""
	// Foreground is unset by default — terminal default fg. Only set by
	// themes that want a full-screen canvas (e.g. Midnight Commander).
	Foreground Color = ""

	// Pane context badge — muted so it sits quietly on the top border.
	ContextOnline  Color = "#8A9A7B" // muted green — pane context badge, connected
	ContextOffline Color = "#C34043" // autumnRed — pane context badge, offline
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

// Log colors — log viewer timestamp and IP highlighting.
var (
	LogTimestamp Color = "#FFA066" // SurimiOrange — timestamp date part
	LogTime      Color = "#7E9CD8" // CrystalBlue — timestamp time part
	LogTimezone  Color = "#727169" // FujiGrey — timestamp timezone part
	LogIP        Color = "#7FB4CA" // WaveBlue — IP addresses
)

// initWarning records a non-fatal problem encountered while resolving the
// theme at init time (e.g. a configured theme name that doesn't exist). The
// app can surface it to the user. A nil value means the theme resolved cleanly.
var initWarning error

// InitWarning returns any non-fatal warning produced during theme resolution
// at package init time, or nil if the theme resolved cleanly.
func InitWarning() error { return initWarning }

// init resolves the configured theme at startup, applying named-theme and
// theme.yaml layers over the compile-time defaults before any other package
// (e.g. ui, highlight) captures these color globals into styles.
//
// It reads the config here — even though run() also loads config later — by
// design: theme resolution must happen at package-init time, before downstream
// package-level vars that depend on these colors are initialized. The
// duplicate read is intentional, not a redundancy to remove.
func init() {
	cfg, _ := config.LoadConfig(config.ConfigPath())
	themeName := ""
	if cfg != nil {
		themeName = cfg.Theme
	}
	initWarning = resolve(ThemesDir(), themeName, ThemePath())
}

// ThemePath returns the default theme file path.
func ThemePath() string {
	return filepath.Join(config.ConfigDir(), "theme.yaml")
}

// ThemesDir returns the directory holding named theme files.
func ThemesDir() string {
	return filepath.Join(config.ConfigDir(), "themes")
}

// resolve applies themes in layered order over the current (default) colors:
//
//  1. If themeName is non-empty, load themesDir/<themeName>.yaml. A missing
//     named theme is returned as a warning (it's likely a typo or missing file)
//     but resolution continues. A parse error is also returned as a warning.
//  2. theme.yaml (at themeYAMLPath) is applied on top, so it always wins over
//     the named theme. A missing theme.yaml is normal and not a warning; a
//     parse error in it is returned as a warning.
//
// Only the first warning encountered is returned; any later warnings are
// intentionally dropped. In particular, if a named-theme stat/parse error
// already set the warning, a subsequent theme.yaml parse error is not
// surfaced (the layering still proceeds and theme.yaml is still applied).
// Returns nil when both layers resolve cleanly. themeName is sanitized with
// filepath.Base to guard against path traversal.
func resolve(themesDir, themeName, themeYAMLPath string) error {
	var warning error

	if themeName != "" {
		named := filepath.Join(themesDir, filepath.Base(themeName)+".yaml")
		if _, err := os.Stat(named); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				warning = fmt.Errorf("theme %q not found at %s", themeName, named)
			} else {
				warning = fmt.Errorf("theme %q: %w", themeName, err)
			}
		} else if err := Load(named); err != nil {
			warning = fmt.Errorf("theme %q: %w", themeName, err)
		}
	}

	if err := Load(themeYAMLPath); err != nil && warning == nil {
		warning = fmt.Errorf("theme.yaml: %w", err)
	}

	return warning
}

// Load reads a YAML theme file and overwrites only the colors present in it.
// If the file does not exist, the defaults are kept (nil error). A YAML parse
// error is returned; because unmarshal runs to completion before any color is
// applied, a parse error leaves all colors untouched (no partial application).
func Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
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
		setIf(&Background, f.UI.Background)
		setIf(&Foreground, f.UI.Foreground)
		setIf(&ContextOnline, f.UI.ContextOnline)
		setIf(&ContextOffline, f.UI.ContextOffline)
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
	if f.Log != nil {
		setIf(&LogTimestamp, f.Log.Timestamp)
		setIf(&LogTime, f.Log.Time)
		setIf(&LogTimezone, f.Log.Timezone)
		setIf(&LogIP, f.Log.IP)
	}
	return nil
}

func setIf(target *Color, value *string) {
	if value != nil {
		*target = Color(*value)
	}
}

type themeFile struct {
	UI     *uiColors     `yaml:"ui"`
	Status *statusColors `yaml:"status"`
	Syntax *syntaxColors `yaml:"syntax"`
	Search *searchColors `yaml:"search"`
	Log    *logColors    `yaml:"log"`
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
	Background   *string `yaml:"background"`
	Foreground   *string `yaml:"foreground"`

	ContextOnline  *string `yaml:"context_online"`
	ContextOffline *string `yaml:"context_offline"`
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

type logColors struct {
	Timestamp *string `yaml:"timestamp"`
	Time      *string `yaml:"time"`
	Timezone  *string `yaml:"timezone"`
	IP        *string `yaml:"ip"`
}
