package theme

import (
	"os"
	"path/filepath"
	"testing"
)

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

	origRunning := StatusRunning
	origFailed := StatusFailed
	defer func() {
		StatusRunning = origRunning
		StatusFailed = origFailed
	}()

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
`
	os.WriteFile(path, []byte(content), 0644)

	origAccent := Accent
	origMuted := Muted
	origHighlight := Highlight
	origTextOnAccent := TextOnAccent
	origError := Error
	origWarning := Warning
	origSubtle := Subtle
	origPrompt := Prompt
	origStatusRunning := StatusRunning
	origStatusSucceeded := StatusSucceeded
	origStatusPending := StatusPending
	origStatusFailed := StatusFailed
	origSyntaxKey := SyntaxKey
	origSyntaxString := SyntaxString
	origSyntaxNumber := SyntaxNumber
	origSyntaxBool := SyntaxBool
	origSyntaxNull := SyntaxNull
	origSyntaxMarker := SyntaxMarker
	origSyntaxValue := SyntaxValue
	origSearchMatch := SearchMatch
	origSearchSelected := SearchSelected
	origSearchFg := SearchFg
	defer func() {
		Accent = origAccent
		Muted = origMuted
		Highlight = origHighlight
		TextOnAccent = origTextOnAccent
		Error = origError
		Warning = origWarning
		Subtle = origSubtle
		Prompt = origPrompt
		StatusRunning = origStatusRunning
		StatusSucceeded = origStatusSucceeded
		StatusPending = origStatusPending
		StatusFailed = origStatusFailed
		SyntaxKey = origSyntaxKey
		SyntaxString = origSyntaxString
		SyntaxNumber = origSyntaxNumber
		SyntaxBool = origSyntaxBool
		SyntaxNull = origSyntaxNull
		SyntaxMarker = origSyntaxMarker
		SyntaxValue = origSyntaxValue
		SearchMatch = origSearchMatch
		SearchSelected = origSearchSelected
		SearchFg = origSearchFg
	}()

	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if string(Accent) != "99" {
		t.Fatalf("expected Accent 99, got %s", string(Accent))
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
}

func TestKanagawaWaveDefaults(t *testing.T) {
	// Point to an empty config dir so init()'s Load() doesn't interfere.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Reset all to compile-time defaults then reload (no file found).
	Accent = "#957FB8"
	Muted = "#727169"
	Highlight = "#C8C093"
	TextOnAccent = "#1F1F28"
	Error = "#E82424"
	Warning = "#FF9E3B"
	Subtle = "#54546D"
	Prompt = "#727169"
	StatusRunning = "#76946A"
	StatusSucceeded = "#727169"
	StatusPending = "#FF9E3B"
	StatusFailed = "#E82424"
	SyntaxKey = "#7E9CD8"
	SyntaxString = "#98BB6C"
	SyntaxNumber = "#D27E99"
	SyntaxBool = "#957FB8"
	SyntaxNull = "#727169"
	SyntaxMarker = "#54546D"
	SyntaxValue = "#DCD7BA"
	SearchMatch = "#FF9E3B"
	SearchSelected = "#957FB8"
	SearchFg = "#1F1F28"

	if err := Load(ThemePath()); err != nil {
		t.Fatal(err)
	}

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
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}
