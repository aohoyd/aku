// Package clipboard wraps OSC52 clipboard writes (via tea.SetClipboard) with a
// best-effort native fallback (github.com/atotto/clipboard) and provides pure
// helpers for concatenating resource names and YAML documents.
package clipboard

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	nativeclip "github.com/atotto/clipboard"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Copy returns a tea.Cmd that writes s to the system clipboard. It batches a
// best-effort native write (the native tool may be absent, e.g. over SSH, so
// the error is intentionally ignored) with an OSC52 clipboard write via
// tea.SetClipboard so terminals that support it (including remote/SSH sessions)
// receive the value too. The native write runs inside the returned closure so
// it executes when the cmd is run (off the Update loop), not at dispatch time —
// a slow native tool can't block the UI.
func Copy(s string) tea.Cmd {
	return tea.Batch(
		func() tea.Msg { _ = nativeclip.WriteAll(s); return nil },
		tea.SetClipboard(s),
	)
}

// JoinNames returns the GetName() of each object joined by newlines.
func JoinNames(objs []*unstructured.Unstructured) string {
	names := make([]string, 0, len(objs))
	for _, obj := range objs {
		names = append(names, obj.GetName())
	}
	return strings.Join(names, "\n")
}

// JoinYAML joins YAML documents with a "\n---\n" separator, trimming trailing
// whitespace from each document so the separators stay clean. A leading "---"
// document-separator line is also trimmed from each doc so a doc that already
// starts with "---" doesn't produce a doubled separator after the join. (The
// renderer never emits a leading separator today; this just keeps the join
// robust if it ever does.)
func JoinYAML(docs []string) string {
	trimmed := make([]string, 0, len(docs))
	for _, doc := range docs {
		doc = strings.TrimRight(doc, "\n \t")
		// Drop a single leading "---" separator line if present.
		if rest, ok := strings.CutPrefix(doc, "---\n"); ok {
			doc = rest
		} else if doc == "---" {
			doc = ""
		}
		trimmed = append(trimmed, doc)
	}
	return strings.Join(trimmed, "\n---\n")
}
