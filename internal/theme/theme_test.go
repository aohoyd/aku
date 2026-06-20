package theme

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/config"
)

// snapshotColors captures every package-level color global and returns a
// function that restores them all. Use it in tests that mutate theme globals
// (directly or via Load/resolve) so mutated state never leaks into later tests:
//
//	defer snapshotColors()()
func snapshotColors() func() {
	type saved struct {
		// UI
		accent, muted, highlight, textOnAccent, errc, warning Color
		subtle, prompt, selection, background, foreground     Color
		contextOnline, contextOffline                         Color
		// Status
		statusRunning, statusSucceeded, statusPending, statusFailed Color
		// Syntax
		syntaxKey, syntaxString, syntaxNumber, syntaxBool Color
		syntaxNull, syntaxMarker, syntaxValue             Color
		// Search
		searchMatch, searchSelected, searchFg Color
		// Log
		logTimestamp, logTime, logTimezone, logIP Color
	}
	s := saved{
		accent: Accent, muted: Muted, highlight: Highlight, textOnAccent: TextOnAccent,
		errc: Error, warning: Warning, subtle: Subtle, prompt: Prompt, selection: Selection,
		background: Background, foreground: Foreground,
		contextOnline: ContextOnline, contextOffline: ContextOffline,
		statusRunning: StatusRunning, statusSucceeded: StatusSucceeded,
		statusPending: StatusPending, statusFailed: StatusFailed,
		syntaxKey: SyntaxKey, syntaxString: SyntaxString, syntaxNumber: SyntaxNumber,
		syntaxBool: SyntaxBool, syntaxNull: SyntaxNull, syntaxMarker: SyntaxMarker,
		syntaxValue: SyntaxValue,
		searchMatch: SearchMatch, searchSelected: SearchSelected, searchFg: SearchFg,
		logTimestamp: LogTimestamp, logTime: LogTime, logTimezone: LogTimezone, logIP: LogIP,
	}
	return func() {
		Accent, Muted, Highlight, TextOnAccent = s.accent, s.muted, s.highlight, s.textOnAccent
		Error, Warning, Subtle, Prompt, Selection = s.errc, s.warning, s.subtle, s.prompt, s.selection
		Background, Foreground = s.background, s.foreground
		ContextOnline, ContextOffline = s.contextOnline, s.contextOffline
		StatusRunning, StatusSucceeded = s.statusRunning, s.statusSucceeded
		StatusPending, StatusFailed = s.statusPending, s.statusFailed
		SyntaxKey, SyntaxString, SyntaxNumber = s.syntaxKey, s.syntaxString, s.syntaxNumber
		SyntaxBool, SyntaxNull, SyntaxMarker, SyntaxValue = s.syntaxBool, s.syntaxNull, s.syntaxMarker, s.syntaxValue
		SearchMatch, SearchSelected, SearchFg = s.searchMatch, s.searchSelected, s.searchFg
		LogTimestamp, LogTime, LogTimezone, LogIP = s.logTimestamp, s.logTime, s.logTimezone, s.logIP
	}
}

// TestDefaultColors is a fast smoke test asserting the compile-time color
// globals are non-empty without mutating or reloading them. It complements
// (and is deliberately cheaper than) TestKanagawaWaveDefaults, which pins the
// exact default values after a Load round-trip.
func TestDefaultColors(t *testing.T) {
	if Accent == "" {
		t.Fatal("Accent must not be empty")
	}
	if StatusRunning == "" {
		t.Fatal("StatusRunning must not be empty")
	}
	if SyntaxKey == "" {
		t.Fatal("SyntaxKey must not be empty")
	}
	if LogTimestamp == "" {
		t.Fatal("LogTimestamp must not be empty")
	}
}

func TestLoadMissingFile(t *testing.T) {
	orig := StatusRunning
	defer func() { StatusRunning = orig }()

	err := Load("/nonexistent/path/theme.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if StatusRunning != orig {
		t.Fatal("missing file should not change defaults")
	}
}

func TestLoadPartialOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	os.WriteFile(path, []byte("status:\n  running: \"#00FF00\"\n"), 0644)

	origFailed := StatusFailed
	defer snapshotColors()()

	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if string(StatusRunning) != "#00FF00" {
		t.Fatalf("expected #00FF00, got %s", string(StatusRunning))
	}
	if StatusFailed != origFailed {
		t.Fatal("non-overridden field should keep default")
	}
}

func TestLoadBackgroundForeground(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	os.WriteFile(path, []byte("ui:\n  background: \"#000080\"\n  foreground: \"#FFFFFF\"\n"), 0644)

	defer snapshotColors()()

	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if string(Background) != "#000080" {
		t.Fatalf("expected Background #000080, got %s", string(Background))
	}
	if string(Foreground) != "#FFFFFF" {
		t.Fatalf("expected Foreground #FFFFFF, got %s", string(Foreground))
	}
}

func TestLoadBackgroundForegroundAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	os.WriteFile(path, []byte("ui:\n  accent: \"99\"\n"), 0644)

	defer snapshotColors()()

	// Force known defaults so the test is independent of init()/other tests.
	Background = ""
	Foreground = ""

	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if Background != "" {
		t.Fatalf("expected Background unset, got %q", string(Background))
	}
	if Foreground != "" {
		t.Fatalf("expected Foreground unset, got %q", string(Foreground))
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	os.WriteFile(path, []byte("{{invalid"), 0644)

	if err := Load(path); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadFullOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	content := `
ui:
  accent: "99"
  muted: "100"
  highlight: "101"
  text_on_accent: "102"
  error: "103"
  warning: "105"
  subtle: "104"
  prompt: "106"
  selection: "107"
  background: "110"
  foreground: "111"
  context_online: "108"
  context_offline: "109"
status:
  running: "#AABBCC"
  succeeded: "#DDEEFF"
  pending: "#112233"
  failed: "#445566"
syntax:
  key: "200"
  string: "201"
  number: "202"
  bool: "203"
  "null": "204"
  marker: "205"
  value: "206"
search:
  match: "207"
  selected: "208"
  fg: "209"
log:
  timestamp: "210"
  time: "211"
  timezone: "212"
  ip: "213"
`
	os.WriteFile(path, []byte(content), 0644)

	defer snapshotColors()()

	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if string(Accent) != "99" {
		t.Fatalf("expected Accent 99, got %s", string(Accent))
	}
	if string(Selection) != "107" {
		t.Fatalf("expected Selection 107, got %s", string(Selection))
	}
	if string(Background) != "110" {
		t.Fatalf("expected Background 110, got %s", string(Background))
	}
	if string(Foreground) != "111" {
		t.Fatalf("expected Foreground 111, got %s", string(Foreground))
	}
	if string(ContextOnline) != "108" {
		t.Fatalf("expected ContextOnline 108, got %s", string(ContextOnline))
	}
	if string(ContextOffline) != "109" {
		t.Fatalf("expected ContextOffline 109, got %s", string(ContextOffline))
	}
	if string(StatusRunning) != "#AABBCC" {
		t.Fatalf("expected StatusRunning #AABBCC, got %s", string(StatusRunning))
	}
	if string(SyntaxKey) != "200" {
		t.Fatalf("expected SyntaxKey 200, got %s", string(SyntaxKey))
	}
	if string(Warning) != "105" {
		t.Fatalf("expected Warning 105, got %s", string(Warning))
	}
	if string(Prompt) != "106" {
		t.Fatalf("expected Prompt 106, got %s", string(Prompt))
	}
	if string(SearchMatch) != "207" {
		t.Fatalf("expected SearchMatch 207, got %s", string(SearchMatch))
	}
	if string(LogTimestamp) != "210" {
		t.Fatalf("expected LogTimestamp 210, got %s", string(LogTimestamp))
	}
}

func TestKanagawaWaveDefaults(t *testing.T) {
	// Restore on exit, but do NOT pre-assign the globals: this test verifies the
	// compile-time defaults from theme.go's var initializers. snapshotColors()
	// restores any leakage from earlier tests after this one runs; the globals
	// read here are the package-initializer defaults because we point Load at an
	// empty config dir (no theme.yaml found) so nothing is layered on top.
	defer snapshotColors()()

	// Point to an empty config dir so Load(ThemePath()) finds no file and the
	// globals keep their package-initializer defaults.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := Load(ThemePath()); err != nil {
		t.Fatal(err)
	}

	// Expected values are the defaults documented in theme.go's var blocks.
	// Changing a default in theme.go without updating this list fails the test.
	checks := []struct {
		name string
		got  Color
		want Color
	}{
		{"Accent", Accent, "#957FB8"},
		{"Muted", Muted, "#727169"},
		{"Highlight", Highlight, "#C8C093"},
		{"TextOnAccent", TextOnAccent, "#1F1F28"},
		{"Error", Error, "#E82424"},
		{"Warning", Warning, "#FF9E3B"},
		{"Subtle", Subtle, "#54546D"},
		{"Prompt", Prompt, "#727169"},
		{"Selection", Selection, "#FF9E3B"},
		{"Background", Background, ""},
		{"Foreground", Foreground, ""},
		{"ContextOnline", ContextOnline, "#8A9A7B"},
		{"ContextOffline", ContextOffline, "#C34043"},
		{"StatusRunning", StatusRunning, "#76946A"},
		{"StatusSucceeded", StatusSucceeded, "#727169"},
		{"StatusPending", StatusPending, "#FF9E3B"},
		{"StatusFailed", StatusFailed, "#E82424"},
		{"SyntaxKey", SyntaxKey, "#7E9CD8"},
		{"SyntaxString", SyntaxString, "#98BB6C"},
		{"SyntaxNumber", SyntaxNumber, "#D27E99"},
		{"SyntaxBool", SyntaxBool, "#957FB8"},
		{"SyntaxNull", SyntaxNull, "#727169"},
		{"SyntaxMarker", SyntaxMarker, "#54546D"},
		{"SyntaxValue", SyntaxValue, "#DCD7BA"},
		{"SearchMatch", SearchMatch, "#FF9E3B"},
		{"SearchSelected", SearchSelected, "#957FB8"},
		{"SearchFg", SearchFg, "#1F1F28"},
		{"LogTimestamp", LogTimestamp, "#FFA066"},
		{"LogTime", LogTime, "#7E9CD8"},
		{"LogTimezone", LogTimezone, "#727169"},
		{"LogIP", LogIP, "#7FB4CA"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestConfigToResolveToGlobals exercises the full chain: a config.yaml names a
// theme, a themes/<name>.yaml defines a color, and resolve (driven by the theme
// name read from the loaded config) must update the package-level color global.
func TestConfigToResolveToGlobals(t *testing.T) {
	defer snapshotColors()()

	confDir := t.TempDir()
	themesDir := filepath.Join(confDir, "themes")
	if err := os.MkdirAll(themesDir, 0755); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(confDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte("theme: mytheme\n"), 0644); err != nil {
		t.Fatal(err)
	}
	namedPath := filepath.Join(themesDir, "mytheme.yaml")
	if err := os.WriteFile(namedPath, []byte("ui:\n  accent: \"#ABCDEF\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Theme != "mytheme" {
		t.Fatalf("expected config theme %q, got %q", "mytheme", cfg.Theme)
	}

	// No theme.yaml present, so the named theme's color reaches the global.
	missingThemeYAML := filepath.Join(confDir, "theme.yaml") // does not exist
	if warn := resolve(themesDir, cfg.Theme, missingThemeYAML); warn != nil {
		t.Fatalf("expected clean resolve, got warning: %v", warn)
	}
	if string(Accent) != "#ABCDEF" {
		t.Fatalf("expected Accent #ABCDEF from named theme via config, got %s", string(Accent))
	}
}

func TestResolveLayeringOrder(t *testing.T) {
	themesDir := t.TempDir()
	confDir := t.TempDir()
	namedPath := filepath.Join(themesDir, "mytheme.yaml")
	themeYAML := filepath.Join(confDir, "theme.yaml")
	os.WriteFile(namedPath, []byte("ui:\n  accent: \"#111111\"\n"), 0644)
	os.WriteFile(themeYAML, []byte("ui:\n  accent: \"#222222\"\n"), 0644)

	defer snapshotColors()()

	if err := resolve(themesDir, "mytheme", themeYAML); err != nil {
		t.Fatalf("expected no warning, got %v", err)
	}
	// theme.yaml is applied on top of the named theme and must win.
	if string(Accent) != "#222222" {
		t.Fatalf("expected Accent #222222 (theme.yaml overrides named), got %s", string(Accent))
	}
}

func TestResolveMissingNamedTheme(t *testing.T) {
	themesDir := t.TempDir()

	defer snapshotColors()()
	// Force known sentinel values so we can confirm nothing changed when the
	// named theme is missing and there is no theme.yaml to layer.
	Accent = "#957FB8"
	Background = "#ABCDEF"
	Foreground = "#123456"

	nonexistentThemeYAML := filepath.Join(t.TempDir(), "theme.yaml") // does not exist
	err := resolve(themesDir, "doesnotexist", nonexistentThemeYAML)
	if err == nil {
		t.Fatal("expected a warning for missing named theme")
	}
	if string(Accent) != "#957FB8" {
		t.Fatalf("missing named theme should leave Accent unchanged, got %s", string(Accent))
	}
	if string(Background) != "#ABCDEF" {
		t.Fatalf("missing named theme should leave Background unchanged, got %s", string(Background))
	}
	if string(Foreground) != "#123456" {
		t.Fatalf("missing named theme should leave Foreground unchanged, got %s", string(Foreground))
	}
}

func TestResolveEmptyName(t *testing.T) {
	themesDir := t.TempDir()
	confDir := t.TempDir()
	themeYAML := filepath.Join(confDir, "theme.yaml")
	os.WriteFile(themeYAML, []byte("ui:\n  accent: \"#333333\"\n"), 0644)

	defer snapshotColors()()

	if err := resolve(themesDir, "", themeYAML); err != nil {
		t.Fatalf("expected no warning, got %v", err)
	}
	if string(Accent) != "#333333" {
		t.Fatalf("expected Accent #333333 from theme.yaml, got %s", string(Accent))
	}
}

func TestResolveNamedThemeParseError(t *testing.T) {
	defer snapshotColors()()

	themesDir := t.TempDir()
	// os.Stat succeeds but Load fails because the file is invalid YAML.
	os.WriteFile(filepath.Join(themesDir, "broken.yaml"), []byte("{{invalid"), 0644)

	nonexistentThemeYAML := filepath.Join(t.TempDir(), "theme.yaml") // does not exist
	err := resolve(themesDir, "broken", nonexistentThemeYAML)
	if err == nil {
		t.Fatal("expected a warning for parse error in named theme")
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Fatalf("warning should mention the theme name, got %v", err)
	}
}

func TestResolveThemeYAMLParseError(t *testing.T) {
	defer snapshotColors()()

	themesDir := t.TempDir() // no named theme used (empty name)
	confDir := t.TempDir()
	themeYAML := filepath.Join(confDir, "theme.yaml")
	os.WriteFile(themeYAML, []byte("{{invalid"), 0644)

	err := resolve(themesDir, "", themeYAML)
	if err == nil {
		t.Fatal("expected a warning for parse error in theme.yaml")
	}
	if !strings.Contains(err.Error(), "theme.yaml") {
		t.Fatalf("warning should mention theme.yaml, got %v", err)
	}
}

func TestResolveFirstWarningWins(t *testing.T) {
	defer snapshotColors()()

	// Both the named theme is missing AND theme.yaml is invalid. resolve must
	// return the named-theme warning; the theme.yaml error is suppressed.
	themesDir := t.TempDir() // "missing" theme does not exist here
	confDir := t.TempDir()
	themeYAML := filepath.Join(confDir, "theme.yaml")
	os.WriteFile(themeYAML, []byte("{{invalid"), 0644)

	err := resolve(themesDir, "missing", themeYAML)
	if err == nil {
		t.Fatal("expected a warning")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("expected named-theme warning to win, got %v", err)
	}
	if strings.Contains(err.Error(), "theme.yaml") {
		t.Fatalf("theme.yaml error should be suppressed, got %v", err)
	}
}

// TestResolveContinuesAfterMissingNamedTheme verifies the continue-after-warning
// contract: when the named theme is missing but a valid theme.yaml is present,
// resolve returns a warning naming the missing theme AND still applies theme.yaml.
func TestResolveContinuesAfterMissingNamedTheme(t *testing.T) {
	defer snapshotColors()()

	themesDir := t.TempDir() // "missing" theme does not exist here
	confDir := t.TempDir()
	themeYAML := filepath.Join(confDir, "theme.yaml")
	os.WriteFile(themeYAML, []byte("ui:\n  accent: \"#ABCDEF\"\n"), 0644)

	err := resolve(themesDir, "missing", themeYAML)
	if err == nil {
		t.Fatal("expected a warning for the missing named theme")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("warning should name the missing theme, got %v", err)
	}
	// Resolution must continue: theme.yaml is still applied despite the warning.
	if string(Accent) != "#ABCDEF" {
		t.Fatalf("expected Accent #ABCDEF from theme.yaml applied after warning, got %s", string(Accent))
	}
}

// TestResolvePathTraversal verifies themeName is sanitized with filepath.Base so
// a traversal payload can only resolve inside themesDir. The payload reduces to
// "passwd", which matches a file we plant in themesDir; resolution succeeds with
// no warning and applies that file's color, proving the traversal was stripped.
func TestResolvePathTraversal(t *testing.T) {
	defer snapshotColors()()

	themesDir := t.TempDir()
	os.WriteFile(filepath.Join(themesDir, "passwd.yaml"), []byte("ui:\n  accent: \"#FEEDED\"\n"), 0644)

	nonexistentThemeYAML := filepath.Join(t.TempDir(), "theme.yaml") // does not exist
	err := resolve(themesDir, "../../../etc/passwd", nonexistentThemeYAML)
	if err != nil {
		t.Fatalf("expected clean resolve (basename resolves inside themesDir), got %v", err)
	}
	if string(Accent) != "#FEEDED" {
		t.Fatalf("expected Accent #FEEDED from themesDir/passwd.yaml, got %s", string(Accent))
	}
}

func TestResolveStatError(t *testing.T) {
	defer snapshotColors()()

	// Create a regular file and treat it as if it were the themes directory.
	// os.Stat(filepath.Join(notADir, name+".yaml")) then fails with a
	// non-ErrNotExist error (ENOTDIR on POSIX) because a path component is a
	// file, exercising resolve()'s generic stat-error branch.
	notADir := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(notADir, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	themesDir := filepath.Join(notADir, "themes")

	if _, statErr := os.Stat(filepath.Join(themesDir, "x.yaml")); errors.Is(statErr, fs.ErrNotExist) || statErr == nil {
		// Platform doesn't produce a non-ErrNotExist error here; nothing to test.
		t.Skipf("platform does not yield a non-ErrNotExist stat error: %v", statErr)
	}

	nonexistentThemeYAML := filepath.Join(t.TempDir(), "theme.yaml") // does not exist
	err := resolve(themesDir, "x", nonexistentThemeYAML)
	if err == nil {
		t.Fatal("expected a warning for non-ErrNotExist stat error")
	}
	if !strings.Contains(err.Error(), "x") {
		t.Fatalf("warning should mention the theme name, got %v", err)
	}
}

func TestMidnightCommanderGolden(t *testing.T) {
	path := filepath.Join("..", "..", "themes", "midnight-commander.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("shipped theme file missing at %s: %v", path, err)
	}

	defer snapshotColors()()

	if err := Load(path); err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}

	checks := []struct {
		name string
		got  Color
		want Color
	}{
		{"Background", Background, "#0000AA"},
		{"Accent", Accent, "#00AAAA"},
		{"Highlight", Highlight, "#FFFF55"},
		{"Foreground", Foreground, "#AAAAAA"},
		// Representative coverage across every theme section.
		{"Muted", Muted, "#0088AA"},
		{"TextOnAccent", TextOnAccent, "#000000"},
		{"Error", Error, "#FF5555"},
		{"Selection", Selection, "#FFFF55"},
		{"StatusRunning", StatusRunning, "#55FF55"},
		{"SyntaxKey", SyntaxKey, "#55FFFF"},
		{"SearchMatch", SearchMatch, "#FFFF55"},
		{"LogIP", LogIP, "#55FFFF"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestKakuDarkGolden(t *testing.T) {
	path := filepath.Join("..", "..", "themes", "kaku-dark.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("shipped theme file missing at %s: %v", path, err)
	}

	defer snapshotColors()()

	if err := Load(path); err != nil {
		t.Fatalf("Load(%s): %v", path, err)
	}

	checks := []struct {
		name string
		got  Color
		want Color
	}{
		{"Background", Background, "#15141B"},
		{"Accent", Accent, "#8E6AD9"},
		{"Highlight", Highlight, "#DAAE76"},
		{"Foreground", Foreground, "#D5D4D6"},
		// Representative coverage across every theme section.
		{"Muted", Muted, "#6D6D6D"},
		{"TextOnAccent", TextOnAccent, "#15141B"},
		{"Error", Error, "#D85D5D"},
		{"Selection", Selection, "#DAAE76"},
		{"StatusRunning", StatusRunning, "#58D8AD"},
		{"SyntaxKey", SyntaxKey, "#68AFDA"},
		{"SearchMatch", SearchMatch, "#DAAE76"},
		{"LogIP", LogIP, "#90C9E6"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestLoadPartialOverrideLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.yaml")
	os.WriteFile(path, []byte("log:\n  timestamp: \"#00FF00\"\n"), 0644)

	origTimestamp := LogTimestamp
	origTime := LogTime
	defer func() {
		LogTimestamp = origTimestamp
		LogTime = origTime
	}()

	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if string(LogTimestamp) != "#00FF00" {
		t.Fatalf("expected #00FF00, got %s", string(LogTimestamp))
	}
	if LogTime != origTime {
		t.Fatal("non-overridden field should keep default")
	}
}
