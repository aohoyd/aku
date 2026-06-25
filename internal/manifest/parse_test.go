package manifest

import (
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// failingReader returns an error on the first Read, exercising Parse's
// stream-read error path.
type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

func TestParseMultiDoc(t *testing.T) {
	in := `apiVersion: v1
kind: ConfigMap
metadata:
  name: a
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: b
`
	objs, warns, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}
	if got := objs[0].GetName(); got != "a" {
		t.Fatalf("expected first object name 'a', got %q", got)
	}
	if got := objs[1].GetName(); got != "b" {
		t.Fatalf("expected second object name 'b', got %q", got)
	}
}

func TestParseSourceAnnotation(t *testing.T) {
	in := `# Source: mychart/templates/cm.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: a
`
	objs, _, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
	ann := objs[0].GetAnnotations()
	if ann[SourceAnnotation] != "mychart/templates/cm.yaml" {
		t.Fatalf("expected source annotation, got %q", ann[SourceAnnotation])
	}
	// Assert it survives marshalling to YAML (it is a real metadata annotation).
	out, err := yaml.Marshal(objs[0].Object)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), SourceAnnotation) ||
		!strings.Contains(string(out), "mychart/templates/cm.yaml") {
		t.Fatalf("expected annotation in marshalled output, got:\n%s", out)
	}
}

func TestParseUnwrapsList(t *testing.T) {
	in := `# Source: mychart/templates/list.yaml
apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: a
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: b
`
	objs, warns, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 unwrapped objects, got %d", len(objs))
	}
	for i, want := range []string{"a", "b"} {
		if got := objs[i].GetName(); got != want {
			t.Fatalf("item %d: expected name %q, got %q", i, want, got)
		}
		if got := objs[i].GetKind(); got != "ConfigMap" {
			t.Fatalf("item %d: expected kind ConfigMap, got %q", i, got)
		}
		// Source annotation is carried onto each item.
		if ann := objs[i].GetAnnotations(); ann[SourceAnnotation] != "mychart/templates/list.yaml" {
			t.Fatalf("item %d: expected source annotation carried, got %q", i, ann[SourceAnnotation])
		}
	}
}

func TestParseSkipsEmptyAndCommentOnly(t *testing.T) {
	in := `
---
# just a comment
---

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: a
---

`
	objs, warns, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings for empty/comment-only docs, got %v", warns)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
	if got := objs[0].GetName(); got != "a" {
		t.Fatalf("expected name 'a', got %q", got)
	}
}

func TestParseHandlesLongLines(t *testing.T) {
	// A document containing a single line far larger than bufio.Scanner's default
	// 64KB buffer must still parse: isBlank/scanSource must not stop early and
	// silently drop the doc. Build a ConfigMap whose data value is a ~200KB
	// single-line string.
	big := strings.Repeat("x", 200*1024)
	in := "apiVersion: v1\n" +
		"kind: ConfigMap\n" +
		"metadata:\n" +
		"  name: big\n" +
		"data:\n" +
		"  blob: " + big + "\n"

	objs, warns, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}
	if len(objs) != 1 {
		t.Fatalf("expected the long-lined doc to be kept (1 object), got %d", len(objs))
	}
	if got := objs[0].GetName(); got != "big" {
		t.Fatalf("expected ConfigMap name 'big', got %q", got)
	}
	blob, _, _ := unstructured.NestedString(objs[0].Object, "data", "blob")
	if len(blob) != len(big) {
		t.Fatalf("expected blob length %d, got %d", len(big), len(blob))
	}
}

func TestParseReadError(t *testing.T) {
	sentinel := errors.New("boom")
	objs, warns, err := Parse(failingReader{err: sentinel})
	if err == nil {
		t.Fatalf("expected an error from a failing reader, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected wrapped sentinel error, got %v", err)
	}
	if !strings.Contains(err.Error(), "read manifest stream") {
		t.Fatalf("expected 'read manifest stream' in error, got %v", err)
	}
	if objs != nil || warns != nil {
		t.Fatalf("expected nil objs/warns on read error, got %v / %v", objs, warns)
	}
}

func TestParseListItemNotObjectWarns(t *testing.T) {
	// A List whose items contain a non-map (here a bare string) must produce a
	// warning rather than parsing that item; the document is skipped. The valid
	// ConfigMap document that follows must still parse.
	in := `apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: a
  - just-a-string
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: b
`
	objs, warns, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d: %v", len(warns), warns)
	}
	if !strings.Contains(warns[0].Reason, "List item is not an object") {
		t.Fatalf("expected 'List item is not an object' warning, got %q", warns[0].Reason)
	}
	// The List failed wholesale (unwrapList returns nil,error on the bad item),
	// which is all-or-nothing: the preceding VALID item "a" is dropped along with
	// the bad one. The following standalone ConfigMap "b" still parses.
	var sawA, sawB bool
	for _, o := range objs {
		switch o.GetName() {
		case "a":
			sawA = true
		case "b":
			sawB = true
		}
	}
	if sawA {
		t.Fatalf("expected the valid List item 'a' to be dropped with the bad item (all-or-nothing), but it parsed")
	}
	if !sawB {
		t.Fatalf("expected the trailing ConfigMap 'b' to still parse, got %d objects", len(objs))
	}
}

func TestParseMalformedContinues(t *testing.T) {
	in := `apiVersion: v1
kind: ConfigMap
metadata:
  name: good1
---
this: is: not: valid: yaml: at: all
  - broken
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: good2
`
	objs, warns, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatalf("expected no fatal error (parsing should continue), got %v", err)
	}
	if len(warns) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d: %v", len(warns), warns)
	}
	if warns[0].Reason == "" {
		t.Fatalf("expected a non-empty warning reason")
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 valid objects despite malformed doc, got %d", len(objs))
	}
	if objs[0].GetName() != "good1" || objs[1].GetName() != "good2" {
		t.Fatalf("expected good1/good2, got %q/%q", objs[0].GetName(), objs[1].GetName())
	}
}
