package notifications

import (
	"strings"
	"testing"
	"time"

	"github.com/aohoyd/aku/internal/notify"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// seededStore returns a store with three messages added oldest-to-newest, so
// store.List() yields them newest-first.
func seededStore(t *testing.T) *notify.Store {
	t.Helper()
	s := notify.NewStore(10)
	s.Add(notify.LevelInfo, "first message", "ctx-info", "src-info")
	s.Add(notify.LevelWarning, "second message", "ctx-warn", "src-warn")
	s.Add(notify.LevelError, "third message", "ctx-error", "src-error")
	return s
}

func TestPluginName(t *testing.T) {
	p := New(notify.NewStore(10))
	if p.Name() != "aku-messages" {
		t.Errorf("expected name aku-messages, got %s", p.Name())
	}
	if p.ShortName() != "msg" {
		t.Errorf("expected short name msg, got %s", p.ShortName())
	}
	if !p.IsClusterScoped() {
		t.Error("expected cluster scoped")
	}
	gvr := p.GVR()
	if gvr.Group != "_ktui" || gvr.Version != "v1" || gvr.Resource != "aku-messages" {
		t.Errorf("unexpected GVR: %+v", gvr)
	}
}

func TestPluginColumns(t *testing.T) {
	p := New(notify.NewStore(10))
	cols := p.Columns()
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
	expected := []string{"TIME", "LEVEL", "CONTEXT", "SOURCE", "MESSAGE"}
	for i, want := range expected {
		if cols[i].Title != want {
			t.Errorf("column %d: expected %q, got %q", i, want, cols[i].Title)
		}
	}
	if !cols[4].Flex {
		t.Error("expected MESSAGE column to be flex")
	}
}

func TestPluginObjectsCountAndOrder(t *testing.T) {
	s := seededStore(t)
	p := New(s)

	objs := p.Objects()
	list := s.List()
	if len(objs) != len(list) {
		t.Fatalf("expected %d objects, got %d", len(list), len(objs))
	}
	if len(objs) != 3 {
		t.Fatalf("expected 3 objects, got %d", len(objs))
	}
	// Newest-first: store.List() returns the error message first.
	for i, m := range list {
		text, _, _ := unstructured.NestedString(objs[i].Object, "text")
		if text != m.Text {
			t.Errorf("object %d: expected text %q (matching store order), got %q", i, m.Text, text)
		}
	}
	// Sanity: first object is the newest (error) message.
	firstText, _, _ := unstructured.NestedString(objs[0].Object, "text")
	if firstText != "third message" {
		t.Errorf("expected newest-first ordering, first text = %q", firstText)
	}
}

func TestPluginRow(t *testing.T) {
	s := seededStore(t)
	p := New(s)
	objs := p.Objects()

	// objs[1] is expected to be the warning message (newest-first: error,
	// warning, info). Assert its level up front so a future ordering change can't
	// silently re-target this test at the wrong object.
	if got, _, _ := unstructured.NestedString(objs[1].Object, "level"); got != "warning" {
		t.Fatalf("objs[1] level = %q, want %q (ordering assumption broken)", got, "warning")
	}

	row := p.Row(objs[1])
	if len(row) != 5 {
		t.Fatalf("expected 5 row values, got %d", len(row))
	}
	if row[0] == "" || row[0] == "<unknown>" {
		t.Errorf("expected non-empty TIME, got %q", row[0])
	}
	if row[1] != "warning" {
		t.Errorf("expected LEVEL 'warning', got %q", row[1])
	}
	if row[2] != "ctx-warn" {
		t.Errorf("expected CONTEXT 'ctx-warn', got %q", row[2])
	}
	if row[3] != "src-warn" {
		t.Errorf("expected SOURCE 'src-warn', got %q", row[3])
	}
	if row[4] != "second message" {
		t.Errorf("expected MESSAGE 'second message', got %q", row[4])
	}
}

func TestDefaultSort(t *testing.T) {
	p := New(notify.NewStore(10))
	pref := p.DefaultSort()
	if pref.Column != "TIME" {
		t.Fatalf("expected column 'TIME', got %q", pref.Column)
	}
	if pref.Ascending {
		t.Fatal("expected descending sort")
	}
}

func TestSortable(t *testing.T) {
	s := seededStore(t)
	p := New(s)
	var sortable plugin.Sortable = p
	objs := p.Objects()

	v := sortable.SortValue(objs[0], "TIME")
	if v == "" {
		t.Fatal("SortValue for TIME should be non-empty")
	}
	if _, err := time.Parse(time.RFC3339, v); err != nil {
		t.Errorf("SortValue for TIME should be RFC3339-parseable, got %q: %v", v, err)
	}

	// Non-TIME columns return the displayed value so they sort
	// lexicographically (returning "" would make ui/sort.go fall back to the
	// numeric message-ID NAME, silently sorting by ID instead of the column).
	// objs[0] is the newest message: the seeded error entry.
	for _, tc := range []struct {
		column, want string
	}{
		{"LEVEL", "error"},
		{"CONTEXT", "ctx-error"},
		{"SOURCE", "src-error"},
		{"MESSAGE", "third message"},
	} {
		if got := sortable.SortValue(objs[0], tc.column); got != tc.want {
			t.Errorf("SortValue(%s) = %q, want %q", tc.column, got, tc.want)
		}
	}

	if got := sortable.SortValue(objs[0], "BOGUS"); got != "" {
		t.Errorf("SortValue for unknown column should be empty, got %q", got)
	}
}

func TestDescribe(t *testing.T) {
	s := seededStore(t)
	p := New(s)
	objs := p.Objects()

	// objs[0] is the error message.
	c, err := p.Describe(t.Context(), objs[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Display == "" {
		t.Fatal("display output should not be empty")
	}
	if stripped := ansi.Strip(c.Display); stripped != c.Raw {
		t.Errorf("strip invariant violated:\nstripped: %q\nraw:      %q", stripped, c.Raw)
	}
	checks := []string{"third message", "error", "ctx-error", "src-error"}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func TestObjectsNilStore(t *testing.T) {
	p := New(nil)
	if objs := p.Objects(); len(objs) != 0 {
		t.Fatalf("expected 0 objects for nil store, got %d", len(objs))
	}
}

// TestObjectsEmptyStore verifies a non-nil but empty store yields a non-nil
// empty slice (so callers can range/append without a nil check).
func TestObjectsEmptyStore(t *testing.T) {
	p := New(notify.NewStore(10))
	objs := p.Objects()
	if objs == nil {
		t.Fatal("Objects() on empty store should return a non-nil slice")
	}
	if len(objs) != 0 {
		t.Fatalf("expected 0 objects for empty store, got %d", len(objs))
	}
}

// TestYAML verifies the YAML interface method renders the message's fields.
func TestYAML(t *testing.T) {
	s := seededStore(t)
	p := New(s)
	objs := p.Objects()

	// objs[0] is the newest (error) message.
	c, err := p.YAML(objs[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Raw == "" {
		t.Fatal("YAML output should not be empty")
	}
	for _, want := range []string{"third message", "error"} {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("YAML output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

// TestFormatTimeUnknown verifies formatTime falls back to "<unknown>" when the
// time field is missing or malformed.
func TestFormatTimeUnknown(t *testing.T) {
	cases := []struct {
		name string
		obj  *unstructured.Unstructured
	}{
		{"missing", &unstructured.Unstructured{Object: map[string]any{}}},
		{"malformed", &unstructured.Unstructured{Object: map[string]any{"time": "not-a-timestamp"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatTime(tc.obj); got != "<unknown>" {
				t.Errorf("formatTime(%s) = %q, want %q", tc.name, got, "<unknown>")
			}
		})
	}
}

// TestRegistration verifies the plugin is reachable from the global registry by
// both its name and short name after Register — the same path cmd/root.go uses.
func TestRegistration(t *testing.T) {
	plugin.Reset()
	t.Cleanup(plugin.Reset)

	store := notify.NewStore(10)
	plugin.Register(New(store))

	got, ok := plugin.ByName("aku-messages")
	if !ok {
		t.Fatal("expected aku-messages to be registered and resolvable by name")
	}
	if got.Name() != "aku-messages" {
		t.Errorf("expected name aku-messages, got %q", got.Name())
	}

	byShort, ok := plugin.ByName("msg")
	if !ok {
		t.Fatal("expected aku-messages to be resolvable by short name 'msg'")
	}
	if byShort.Name() != "aku-messages" {
		t.Errorf("short-name lookup resolved to %q, want aku-messages", byShort.Name())
	}
}
