// Package manifest reads Kubernetes manifests (e.g. the output of
// `helm template`) and turns them into in-memory objects that can be presented
// as a simulated cluster.
package manifest

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

// SourceAnnotation records the manifest source (the `# Source:` comment emitted
// by `helm template`) on each parsed object's metadata. Exported so later tasks
// can read provenance back off the objects.
const SourceAnnotation = "aku.dev/manifest-source"

// Warning records a non-fatal problem with one document in the stream. Reason is
// a human-readable message; where a specific document is at fault it is prefixed
// with its zero-based segment position (e.g. "document 2: ...").
type Warning struct {
	Reason string
}

// Parse reads a multi-document YAML/JSON stream and returns the contained
// Kubernetes objects as unstructured values. Documents are split on `---`
// boundaries so the `# Source:` comment preceding each document can be captured
// into the SourceAnnotation. `kind: List` (apiVersion v1) documents are
// unwrapped into their items, carrying the source annotation onto each item.
//
// Empty and comment-only documents are skipped silently. A document that fails
// to decode is recorded as a Warning and parsing continues with the next
// document; the returned error is reserved for failures reading the stream
// itself.
func Parse(r io.Reader) ([]*unstructured.Unstructured, []Warning, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read manifest stream: %w", err)
	}

	var (
		objs  []*unstructured.Unstructured
		warns []Warning
	)

	for i, seg := range splitDocs(string(raw)) {
		source := scanSource(seg)
		if isBlank(seg) {
			continue
		}

		var m map[string]any
		if err := yaml.Unmarshal([]byte(seg), &m); err != nil {
			warns = append(warns, Warning{Reason: fmt.Sprintf("document %d: %v", i, err)})
			continue
		}
		if len(m) == 0 {
			// Parsed to nothing useful (e.g. only comments / `null`).
			continue
		}

		u := &unstructured.Unstructured{Object: m}

		if isV1List(u) {
			items, err := unwrapList(u, source)
			if err != nil {
				warns = append(warns, Warning{Reason: fmt.Sprintf("document %d: %v", i, err)})
				continue
			}
			objs = append(objs, items...)
			continue
		}

		setSource(u, source)
		objs = append(objs, u)
	}

	return objs, warns, nil
}

// splitDocs splits a raw YAML stream into document segments on `---` lines. The
// separators themselves are dropped; each returned segment retains its leading
// comment lines so the source can be scanned.
func splitDocs(raw string) []string {
	var (
		segs []string
		cur  strings.Builder
	)
	flush := func() {
		segs = append(segs, cur.String())
		cur.Reset()
	}

	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if isDocSeparator(line) {
			flush()
			continue
		}
		cur.WriteString(line)
		cur.WriteByte('\n')
	}
	flush()
	return segs
}

// isDocSeparator reports whether a line is a YAML document boundary (`---`,
// optionally followed by trailing whitespace or a comment).
func isDocSeparator(line string) bool {
	t := strings.TrimRight(line, " \t")
	if t == "---" {
		return true
	}
	if rest, ok := strings.CutPrefix(t, "--- "); ok {
		// `--- # comment` style.
		return strings.HasPrefix(strings.TrimLeft(rest, " \t"), "#") || rest == ""
	}
	return false
}

// scanSource returns the value of the first `# Source:` comment in a segment,
// or "" if none is present.
func scanSource(seg string) string {
	sc := bufio.NewScanner(strings.NewReader(seg))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "#") {
			continue
		}
		body := strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if rest, ok := strings.CutPrefix(body, "Source:"); ok {
			return strings.TrimSpace(rest)
		}
	}
	// A scan error (e.g. a line exceeding the buffer) means we couldn't read the
	// whole segment; there is simply no Source comment to report.
	return ""
}

// isBlank reports whether a segment contains nothing but whitespace and
// comments (no YAML content).
func isBlank(seg string) bool {
	sc := bufio.NewScanner(strings.NewReader(seg))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return false
	}
	// On a scan error (e.g. a line exceeding the buffer) we cannot prove the
	// segment is blank, so treat it as non-blank and keep the document rather than
	// silently dropping a valid doc.
	if sc.Err() != nil {
		return false
	}
	return true
}

func isV1List(u *unstructured.Unstructured) bool {
	return u.GetKind() == "List" && u.GetAPIVersion() == "v1"
}

// unwrapList expands a `kind: List` object into its items, stamping each with
// the source annotation.
func unwrapList(u *unstructured.Unstructured, source string) ([]*unstructured.Unstructured, error) {
	items, found, err := unstructured.NestedSlice(u.Object, "items")
	if err != nil {
		return nil, fmt.Errorf("read List items: %w", err)
	}
	if !found {
		return nil, nil
	}
	out := make([]*unstructured.Unstructured, 0, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("List item is not an object: %T", it)
		}
		item := &unstructured.Unstructured{Object: m}
		setSource(item, source)
		out = append(out, item)
	}
	return out, nil
}

// setSource writes the manifest source onto the object's annotations when a
// source was captured.
func setSource(u *unstructured.Unstructured, source string) {
	if source == "" {
		return
	}
	ann := u.GetAnnotations()
	if ann == nil {
		ann = map[string]string{}
	}
	ann[SourceAnnotation] = source
	u.SetAnnotations(ann)
}
