package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/manifest"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	akupods "github.com/aohoyd/aku/internal/plugins/pods"
)

const podManifest = `
apiVersion: v1
kind: Pod
metadata:
  name: web
  namespace: foo
spec:
  containers:
    - name: c
      image: nginx:1
`

// newManifestPodsApp loads podManifest into a pinned static cluster, registers
// the real pods plugin, and builds an App focused on the pods list for the
// "manifests" context. initialDetail mirrors the --details flag. The focused pane
// is seeded from the static store (no informers in tests).
func newManifestPodsApp(t *testing.T, initialDetail *msgs.DetailMode) App {
	t.Helper()
	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	plugin.Reset()
	t.Cleanup(plugin.Reset)

	podsPlugin := akupods.New()
	plugin.Register(podsPlugin)

	cl, _, err := manifest.Load(strings.NewReader(podManifest), "default")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	mgr := cluster.NewManager(nil, "", 0)
	mgr.RegisterPinned(cl)

	specs := []ResourceSpec{{Plugin: podsPlugin, Namespace: ""}}
	app := New(mgr, km, cfg, nil, nil, nil, specs, initialDetail, layout.OrientationVertical, manifestCtx)
	app.layout.FocusedSplit().SetObjects(app.clusterForFocused().Store().List(podsGVRForTest, ""))
	return app
}

// runAndCollect runs cmd and returns the produced messages, flattening one level
// of tea.BatchMsg (bubbletea does not nest batches).
func runAndCollect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	out := cmd()
	if batch, ok := out.(tea.BatchMsg); ok {
		var collected []tea.Msg
		for _, sub := range batch {
			if sub != nil {
				collected = append(collected, sub())
			}
		}
		return collected
	}
	return []tea.Msg{out}
}

func hasMsg[T any](list []tea.Msg) bool {
	for _, m := range list {
		if _, ok := m.(T); ok {
			return true
		}
	}
	return false
}

// TestManifestStartupShowsManifestContextBadge asserts a manifest (pinned,
// nil-client) startup stamps the statusbar context badge with "manifests" and
// marks it offline, instead of leaving the "default" zero value.
func TestManifestStartupShowsManifestContextBadge(t *testing.T) {
	app := newManifestPodsApp(t, nil)

	if got := app.statusBar.ContextName(); got != manifestCtx {
		t.Errorf("context badge = %q, want %q", got, manifestCtx)
	}
	if app.statusBar.Online() {
		t.Error("manifest context should be offline (no live connection)")
	}
}

// TestInitSchedulesDetailRefreshWhenPanelOpen asserts App.Init dispatches an
// InitDetailRefreshMsg when a detail panel was opened at startup, even for the
// non-connected manifest cluster (whose discovery/heartbeat path is skipped).
func TestInitSchedulesDetailRefreshWhenPanelOpen(t *testing.T) {
	describe := msgs.DetailDescribe
	app := newManifestPodsApp(t, &describe)

	if !hasMsg[msgs.InitDetailRefreshMsg](runAndCollect(app.Init())) {
		t.Fatal("Init should dispatch InitDetailRefreshMsg when the detail panel is open")
	}

	// Without a startup detail panel, Init must NOT schedule a refresh.
	plain := newManifestPodsApp(t, nil)
	if hasMsg[msgs.InitDetailRefreshMsg](runAndCollect(plain.Init())) {
		t.Error("Init should not dispatch InitDetailRefreshMsg when no detail panel is open")
	}
}

// TestManifestStartupPopulatesDescribe is the end-to-end fix for "-dd describe is
// empty on start" in manifest mode: routing InitDetailRefreshMsg through Update
// fills the describe panel from the already-selected first row.
func TestManifestStartupPopulatesDescribe(t *testing.T) {
	describe := msgs.DetailDescribe
	app := newManifestPodsApp(t, &describe)

	// Precondition: the panel is open in describe mode with a selected row but no
	// content yet (New does not refresh).
	if !app.layout.RightPanelVisible() {
		t.Fatal("precondition: right panel should be visible")
	}
	if app.layout.FocusedSplit().Selected() == nil {
		t.Fatal("precondition: first row should be selected")
	}

	// Drive the startup refresh through Update so its App mutations persist.
	model, cmd := app.update(msgs.InitDetailRefreshMsg{})
	app = model.(App)
	if cmd == nil {
		t.Fatal("InitDetailRefreshMsg should produce a describe command")
	}

	// The describe command yields a DescribeLoadedMsg with the pod's content.
	var loaded msgs.DescribeLoadedMsg
	var found bool
	for _, m := range runAndCollect(cmd) {
		if dl, ok := m.(msgs.DescribeLoadedMsg); ok {
			loaded, found = dl, true
		}
	}
	if !found {
		t.Fatal("expected a DescribeLoadedMsg from the startup refresh")
	}
	if loaded.Err != nil {
		t.Fatalf("describe load errored: %v", loaded.Err)
	}
	if loaded.Gen != app.describeGen {
		t.Fatalf("describe gen = %d, want %d (persisted by Update) — would be dropped as stale", loaded.Gen, app.describeGen)
	}

	// Feed it back; the panel must now hold the pod's describe output.
	model, _ = app.update(loaded)
	app = model.(App)
	view := app.layout.RightPanel().View()
	if !strings.Contains(view, "web") {
		t.Fatalf("describe panel should contain the pod name after startup populate, got:\n%s", view)
	}
}
