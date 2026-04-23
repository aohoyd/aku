package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/helmreleases"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// stubHelmClient records the most recent GetValues call. The fetch path runs
// the returned tea.Cmd in a goroutine; this stub is safe because tests
// synchronously execute the cmd and inspect lastCall afterwards.
type stubHelmClient struct {
	lastName string
	lastNs   string
	lastAll  bool
	values   map[string]any
	err      error
}

func (s *stubHelmClient) ListReleases(_ string) ([]helm.ReleaseInfo, error) { return nil, nil }
func (s *stubHelmClient) GetRelease(_, _ string) (*helm.ReleaseInfo, error) { return nil, nil }
func (s *stubHelmClient) GetValues(name, ns string, all bool) (map[string]any, error) {
	s.lastName = name
	s.lastNs = ns
	s.lastAll = all
	if s.err != nil {
		return nil, s.err
	}
	return s.values, nil
}
func (s *stubHelmClient) History(_, _ string) ([]helm.RevisionInfo, error) { return nil, nil }
func (s *stubHelmClient) Upgrade(_, _ string, _ map[string]any) error      { return nil }
func (s *stubHelmClient) Rollback(_, _ string, _ int) error                { return nil }
func (s *stubHelmClient) Uninstall(_, _ string) error                      { return nil }

// setupHelmAppWithRelease prepares an App with a real *helmreleases.Plugin
// (so the type-cast in refreshDetailPanelOpts succeeds) backed by the supplied
// stub client. The split is focused on a single release object and the right
// panel is shown in the requested mode.
func setupHelmAppWithRelease(t *testing.T, hc helm.Client, releaseName, ns string, mode msgs.DetailMode) App {
	t.Helper()
	a := newTestApp()
	a.helmClient = hc
	p := helmreleases.NewWithClient(hc)
	plugin.Register(p)
	a.layout.AddSplit(p, ns)
	obj := &unstructured.Unstructured{}
	obj.SetName(releaseName)
	obj.SetNamespace(ns)
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})
	a.layout.ShowRightPanel()
	a.layout.RightPanel().SetMode(mode)
	return a
}

func TestRefreshDetailPanelOpts_ValuesUserDispatchesFetch(t *testing.T) {
	hc := &stubHelmClient{values: map[string]any{"replicaCount": 5}}
	a := setupHelmAppWithRelease(t, hc, "myrelease", "default", msgs.DetailValues)

	a, cmd := a.refreshDetailPanelOpts(false)
	if cmd == nil {
		t.Fatal("expected a tea.Cmd to fetch helm values")
	}

	view := a.layout.RightPanel().View()
	if !strings.Contains(view, "loading values") {
		t.Fatalf("expected loading placeholder in panel, got:\n%s", view)
	}

	loaded, ok := cmd().(msgs.HelmValuesLoadedMsg)
	if !ok {
		t.Fatalf("expected HelmValuesLoadedMsg, got %T", cmd())
	}
	if hc.lastName != "myrelease" || hc.lastNs != "default" || hc.lastAll {
		t.Errorf("unexpected GetValues args: name=%q ns=%q all=%v", hc.lastName, hc.lastNs, hc.lastAll)
	}
	if loaded.Mode != msgs.DetailValues {
		t.Errorf("expected mode DetailValues in msg, got %v", loaded.Mode)
	}
	if loaded.Err != nil {
		t.Errorf("unexpected Err: %v", loaded.Err)
	}
	if !strings.Contains(loaded.Content.Raw, "replicaCount") {
		t.Errorf("expected marshalled values in content raw, got %q", loaded.Content.Raw)
	}
	if loaded.Content.Display == loaded.Content.Raw {
		t.Errorf("expected styled Display (ANSI codes) different from Raw — got identical strings, indicating render.YAML was bypassed")
	}
}

func TestRefreshDetailPanelOpts_EmptyValuesPlaceholderUser(t *testing.T) {
	hc := &stubHelmClient{values: map[string]any{}}
	a := setupHelmAppWithRelease(t, hc, "norel", "default", msgs.DetailValues)

	_, cmd := a.refreshDetailPanelOpts(false)
	if cmd == nil {
		t.Fatal("expected a tea.Cmd")
	}
	loaded, ok := cmd().(msgs.HelmValuesLoadedMsg)
	if !ok {
		t.Fatalf("expected HelmValuesLoadedMsg")
	}
	if !strings.Contains(loaded.Content.Raw, "# No user-supplied values") {
		t.Fatalf("expected user-values placeholder, got %q", loaded.Content.Raw)
	}
}

func TestRefreshDetailPanelOpts_EmptyValuesPlaceholderAll(t *testing.T) {
	hc := &stubHelmClient{values: map[string]any{}}
	a := setupHelmAppWithRelease(t, hc, "norel", "default", msgs.DetailValuesAll)

	_, cmd := a.refreshDetailPanelOpts(false)
	loaded, ok := cmd().(msgs.HelmValuesLoadedMsg)
	if !ok {
		t.Fatalf("expected HelmValuesLoadedMsg")
	}
	if !strings.Contains(loaded.Content.Raw, "# No values") {
		t.Fatalf("expected all-values placeholder, got %q", loaded.Content.Raw)
	}
}

func TestRefreshDetailPanelOpts_ValuesAllPropagatesFlag(t *testing.T) {
	hc := &stubHelmClient{values: map[string]any{"a": 1}}
	a := setupHelmAppWithRelease(t, hc, "rel2", "kube-system", msgs.DetailValuesAll)

	_, cmd := a.refreshDetailPanelOpts(false)
	if cmd == nil {
		t.Fatal("expected a tea.Cmd")
	}
	_ = cmd()
	if !hc.lastAll {
		t.Errorf("expected GetValues called with all=true, got false")
	}
}

func TestRefreshDetailPanelOpts_ValuesModeNonHelmFallsBackToManifest(t *testing.T) {
	a := newTestApp()
	pods := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	plugin.Register(pods)
	a.layout.AddSplit(pods, "default")
	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})
	a.layout.ShowRightPanel()
	a.layout.RightPanel().SetMode(msgs.DetailValues)

	_, cmd := a.refreshDetailPanelOpts(false)
	if cmd != nil {
		t.Fatalf("expected nil cmd for non-helmrelease row, got %T", cmd)
	}
	view := a.layout.RightPanel().View()
	if !strings.Contains(view, "apiVersion") {
		t.Fatalf("expected manifest YAML rendered for non-helmrelease, got:\n%s", view)
	}
}

func TestRefreshDetailPanelOpts_NilHelmClientErrorsViaPlugin(t *testing.T) {
	a := setupHelmAppWithRelease(t, nil, "rel", "default", msgs.DetailValues)
	a.helmClient = nil

	_, cmd := a.refreshDetailPanelOpts(false)
	if cmd == nil {
		t.Fatal("expected a tea.Cmd that returns an error")
	}
	loaded, ok := cmd().(msgs.HelmValuesLoadedMsg)
	if !ok {
		t.Fatalf("expected HelmValuesLoadedMsg, got %T", cmd())
	}
	if loaded.Err == nil {
		t.Fatal("expected non-nil Err when plugin has no helm client")
	}
}

func TestHelmValuesLoadedMsg_StaleMismatchedDropped(t *testing.T) {
	cases := []struct {
		name string
		msg  msgs.HelmValuesLoadedMsg
	}{
		{
			name: "different release name",
			msg: msgs.HelmValuesLoadedMsg{
				ReleaseName: "other",
				Namespace:   "ns1",
				Mode:        msgs.DetailValues,
				Content:     render.Content{Raw: "k: v\n", Display: "k: v\n"},
			},
		},
		{
			name: "different namespace",
			msg: msgs.HelmValuesLoadedMsg{
				ReleaseName: "current",
				Namespace:   "other-ns",
				Mode:        msgs.DetailValues,
				Content:     render.Content{Raw: "k: v\n", Display: "k: v\n"},
			},
		},
		{
			name: "different mode",
			msg: msgs.HelmValuesLoadedMsg{
				ReleaseName: "current",
				Namespace:   "ns1",
				Mode:        msgs.DetailValuesAll,
				Content:     render.Content{Raw: "k: v\n", Display: "k: v\n"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hc := &stubHelmClient{}
			a := setupHelmAppWithRelease(t, hc, "current", "ns1", msgs.DetailValues)
			sentinel := "# sentinel\n"
			a.layout.RightPanel().SetContent(render.Content{Raw: sentinel, Display: sentinel}, true)

			model, _ := a.handleHelmValuesLoaded(tc.msg)
			updated := model.(App)
			view := updated.layout.RightPanel().View()
			if !strings.Contains(view, "sentinel") {
				t.Fatalf("stale msg should not update panel, got view:\n%s", view)
			}
		})
	}
}

func TestHelmValuesLoadedMsg_MatchingUpdatesPanel(t *testing.T) {
	hc := &stubHelmClient{}
	a := setupHelmAppWithRelease(t, hc, "myrel", "default", msgs.DetailValues)

	body := "replicaCount: 7\n"
	model, _ := a.handleHelmValuesLoaded(msgs.HelmValuesLoadedMsg{
		ReleaseName: "myrel",
		Namespace:   "default",
		Mode:        msgs.DetailValues,
		Content:     render.Content{Raw: body, Display: body},
	})
	updated := model.(App)
	view := updated.layout.RightPanel().View()
	if !strings.Contains(view, "replicaCount: 7") {
		t.Fatalf("expected panel updated with values, got:\n%s", view)
	}
}

func TestHelmValuesLoadedMsg_ErrorRendersComment(t *testing.T) {
	hc := &stubHelmClient{}
	a := setupHelmAppWithRelease(t, hc, "myrel", "default", msgs.DetailValues)

	model, _ := a.handleHelmValuesLoaded(msgs.HelmValuesLoadedMsg{
		ReleaseName: "myrel",
		Namespace:   "default",
		Mode:        msgs.DetailValues,
		Err:         &stubErr{msg: "boom"},
	})
	updated := model.(App)
	view := updated.layout.RightPanel().View()
	if !strings.Contains(view, "# error: boom") {
		t.Fatalf("expected error comment in panel, got:\n%s", view)
	}
}

func TestHandleHelmValuesLoaded_PanelNotVisibleDropped(t *testing.T) {
	hc := &stubHelmClient{}
	a := setupHelmAppWithRelease(t, hc, "myrel", "default", msgs.DetailValues)
	sentinel := "# sentinel\n"
	a.layout.RightPanel().SetContent(render.Content{Raw: sentinel, Display: sentinel}, true)
	a.layout.HideRightPanel()

	model, _ := a.handleHelmValuesLoaded(msgs.HelmValuesLoadedMsg{
		ReleaseName: "myrel",
		Namespace:   "default",
		Mode:        msgs.DetailValues,
		Content:     render.Content{Raw: "k: v\n", Display: "k: v\n"},
	})
	updated := model.(App)
	if updated.layout.RightPanelVisible() {
		t.Fatalf("right panel should remain hidden after stale msg")
	}
}

func TestHandleHelmValuesLoaded_LogModeDropped(t *testing.T) {
	hc := &stubHelmClient{}
	a := setupHelmAppWithRelease(t, hc, "myrel", "default", msgs.DetailValues)
	sentinel := "# sentinel\n"
	a.layout.RightPanel().SetContent(render.Content{Raw: sentinel, Display: sentinel}, true)
	a.layout.SetLogMode(true)

	model, _ := a.handleHelmValuesLoaded(msgs.HelmValuesLoadedMsg{
		ReleaseName: "myrel",
		Namespace:   "default",
		Mode:        msgs.DetailValues,
		Content:     render.Content{Raw: "k: v\n", Display: "k: v\n"},
	})
	updated := model.(App)
	view := updated.layout.RightPanel().View()
	if !strings.Contains(view, "sentinel") {
		t.Fatalf("log-mode active: panel content should not change, got:\n%s", view)
	}
}

func TestHandleHelmValuesLoaded_ManifestModeDropped(t *testing.T) {
	hc := &stubHelmClient{}
	a := setupHelmAppWithRelease(t, hc, "myrel", "default", msgs.DetailYAML)
	sentinel := "# sentinel\n"
	a.layout.RightPanel().SetContent(render.Content{Raw: sentinel, Display: sentinel}, true)

	model, _ := a.handleHelmValuesLoaded(msgs.HelmValuesLoadedMsg{
		ReleaseName: "myrel",
		Namespace:   "default",
		Mode:        msgs.DetailValues,
		Content:     render.Content{Raw: "k: v\n", Display: "k: v\n"},
	})
	updated := model.(App)
	view := updated.layout.RightPanel().View()
	if !strings.Contains(view, "sentinel") {
		t.Fatalf("mode mismatch: panel content should not change, got:\n%s", view)
	}
}

type stubErr struct{ msg string }

func (e *stubErr) Error() string { return e.msg }

// findHelmValuesLoaded walks a tea.Cmd (possibly a Batch) and returns the
// HelmValuesLoadedMsg produced by any sub-cmd. Bubbletea flattens batches so
// a tea.BatchMsg never nests another tea.BatchMsg in practice.
func findHelmValuesLoaded(t *testing.T, cmd tea.Cmd) (msgs.HelmValuesLoadedMsg, bool) {
	t.Helper()
	if cmd == nil {
		return msgs.HelmValuesLoadedMsg{}, false
	}
	out := cmd()
	if loaded, ok := out.(msgs.HelmValuesLoadedMsg); ok {
		return loaded, true
	}
	batch, ok := out.(tea.BatchMsg)
	if !ok {
		return msgs.HelmValuesLoadedMsg{}, false
	}
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		if loaded, ok := sub().(msgs.HelmValuesLoadedMsg); ok {
			return loaded, true
		}
	}
	return msgs.HelmValuesLoadedMsg{}, false
}

func TestActionResultMsg_HelmEditValuesRefreshesValuesPanel(t *testing.T) {
	hc := &stubHelmClient{values: map[string]any{"replicaCount": 9}}
	a := setupHelmAppWithRelease(t, hc, "myrel", "default", msgs.DetailValues)

	model, cmd := a.Update(msgs.ActionResultMsg{ActionID: "helm-edit-values:myrel"})
	if _, ok := model.(App); !ok {
		t.Fatalf("expected App model, got %T", model)
	}
	loaded, found := findHelmValuesLoaded(t, cmd)
	if !found {
		t.Fatal("expected a HelmValuesLoadedMsg from re-fetch after helm-edit-values")
	}
	if hc.lastName != "myrel" || hc.lastNs != "default" || hc.lastAll {
		t.Errorf("unexpected GetValues args: name=%q ns=%q all=%v", hc.lastName, hc.lastNs, hc.lastAll)
	}
	if loaded.Mode != msgs.DetailValues {
		t.Errorf("expected loaded mode DetailValues, got %v", loaded.Mode)
	}
	if !strings.Contains(loaded.Content.Raw, "replicaCount") {
		t.Errorf("expected new values in content, got %q", loaded.Content.Raw)
	}
}

func TestActionResultMsg_HelmEditValuesAllModePropagatesAllFlag(t *testing.T) {
	hc := &stubHelmClient{values: map[string]any{"k": "v"}}
	a := setupHelmAppWithRelease(t, hc, "rel", "ns", msgs.DetailValuesAll)

	_, cmd := a.Update(msgs.ActionResultMsg{ActionID: "helm-edit-values:rel"})
	if _, found := findHelmValuesLoaded(t, cmd); !found {
		t.Fatal("expected HelmValuesLoadedMsg")
	}
	if !hc.lastAll {
		t.Errorf("expected GetValues called with all=true after edit on DetailValuesAll mode, got false")
	}
}

func TestActionResultMsg_HelmEditValuesManifestModeNoRefetch(t *testing.T) {
	hc := &stubHelmClient{values: map[string]any{"k": "v"}}
	a := setupHelmAppWithRelease(t, hc, "rel", "ns", msgs.DetailYAML)

	_, cmd := a.Update(msgs.ActionResultMsg{ActionID: "helm-edit-values:rel"})
	if _, found := findHelmValuesLoaded(t, cmd); found {
		t.Fatal("did not expect a values re-fetch when mode is DetailYAML")
	}
}
