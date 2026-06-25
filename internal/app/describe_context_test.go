package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// focusedDetailApp builds an App with a single pods split whose detail pane is
// focused in the given mode. It is the shared fixture for the describe/yaml
// context tests below.
func focusedDetailApp(t *testing.T, mode msgs.DetailMode) App {
	t.Helper()
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	pods := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	plugin.Register(pods)
	a.layout.AddSplit(pods, "default", "")
	obj := &unstructured.Unstructured{}
	obj.SetName("p")
	obj.SetNamespace("default")
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	a.layout.ShowRightPanel()
	a.layout.RightPanel().SetMode(mode)
	a.layout.FocusDetails()
	if !a.layout.FocusedDetails() {
		t.Fatal("precondition: detail pane should be focused")
	}
	return a
}

// hasHintKey reports whether the hint slice contains a binding for key.
func hasHintKey(hints []config.KeyHint, key string) bool {
	for _, h := range hints {
		if h.Key == key {
			return true
		}
	}
	return false
}

// helpHasKey reports whether any help group lists a binding for key.
func helpHasKey(groups []config.HintGroup, key string) bool {
	for _, g := range groups {
		if hasHintKey(g.Hints, key) {
			return true
		}
	}
	return false
}

// TestCurrentContextDescribeCarriesResourceName asserts that a focused detail
// pane reports the underlying resource's plugin name, not an empty string. The
// empty-resourceName regression filtered every details-scoped binding with a
// non-empty For list (e.g. x → toggle-env-resolve) out of the trie, hints, and
// help overlay.
func TestCurrentContextDescribeCarriesResourceName(t *testing.T) {
	a := focusedDetailApp(t, msgs.DetailDescribe)

	ct, rn := a.currentContext()
	if ct != "describe" || rn != "pods" {
		t.Fatalf("describe context = (%q, %q), want (\"describe\", \"pods\")", ct, rn)
	}

	a.layout.RightPanel().SetMode(msgs.DetailYAML)
	if ct, rn := a.currentContext(); ct != "yaml" || rn != "pods" {
		t.Fatalf("yaml context = (%q, %q), want (\"yaml\", \"pods\")", ct, rn)
	}
}

// TestDescribeContextExposesEnvResolveBinding asserts the x / toggle-env-resolve
// binding is visible (statusbar + help) and dispatchable from the describe pane,
// while a non-For details binding (w) stays present — guarding against an
// over-correction that would drop the unfiltered bindings.
func TestDescribeContextExposesEnvResolveBinding(t *testing.T) {
	a := focusedDetailApp(t, msgs.DetailDescribe)
	ct, rn := a.currentContext()

	hints := a.bindingSet.StatusHints(ct, rn)
	if !hasHintKey(hints, "x") {
		t.Errorf("statusbar hints for %q/%q missing x (env resolve)", ct, rn)
	}
	if !hasHintKey(hints, "w") {
		t.Errorf("statusbar hints for %q/%q missing w (wrap) — over-correction", ct, rn)
	}

	if !helpHasKey(a.bindingSet.HelpGroups(ct, rn), "x") {
		t.Errorf("help overlay for %q/%q missing x (env resolve)", ct, rn)
	}

	trie := a.bindingSet.TrieFor(ct, rn)
	cmd, _, resolved := trie.Press("x")
	if !resolved || cmd != "toggle-env-resolve" {
		t.Fatalf("TrieFor(%q,%q).Press(\"x\") = (%q, resolved=%v), want toggle-env-resolve", ct, rn, cmd, resolved)
	}
}

// TestDescribeContextEnvResolveFiltersByResource asserts the For filter still
// applies in the detail pane: a resource not in the toggle-env-resolve For list
// (e.g. configmaps) does not get the x binding.
func TestDescribeContextEnvResolveFiltersByResource(t *testing.T) {
	a := focusedDetailApp(t, msgs.DetailDescribe)

	// configmaps is not in the toggle-env-resolve For list.
	if hasHintKey(a.bindingSet.StatusHints("describe", "configmaps"), "x") {
		t.Error("x should not be bound for configmaps describe context")
	}
}
