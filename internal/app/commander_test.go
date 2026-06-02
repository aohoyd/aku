package app

import (
	"testing"

	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// commanderPlugin embeds mockPlugin and additionally implements
// plugin.Commander, mapping Enter to a fixed app command string.
type commanderPlugin struct {
	mockPlugin
	cmd string
	ok  bool
}

func (c *commanderPlugin) Command(_ *unstructured.Unstructured) (string, bool) {
	return c.cmd, c.ok
}

// TestEnterDetailDispatchesCommanderCommand verifies that when the focused
// plugin implements Commander and returns ok==true, enter-detail dispatches
// the returned command string (here goto-deployments) instead of falling
// through to detail/drill-down behavior.
func TestEnterDetailDispatchesCommanderCommand(t *testing.T) {
	app := newTestApp()

	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	ctxPlugin := &commanderPlugin{
		mockPlugin: mockPlugin{
			name: "contexts",
			gvr:  schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "contexts"},
		},
		cmd: "goto-deployments",
		ok:  true,
	}
	plugin.Register(deploymentsPlugin)
	plugin.Register(ctxPlugin)
	t.Cleanup(func() { plugin.Reset() })

	app.layout.AddSplit(ctxPlugin, "default", "")

	obj := &unstructured.Unstructured{}
	obj.SetName("some-context")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	model, _ := app.executeCommand("enter-detail")
	app = model.(App)

	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split after enter-detail")
	}
	if focused.Plugin().Name() != "deployments" {
		t.Fatalf("expected Commander to dispatch goto-deployments, got plugin %q", focused.Plugin().Name())
	}
}

// TestEnterDetailCommanderNotOkFallsThrough verifies that when Commander
// returns ok==false, enter-detail falls through to the default behavior
// (focusing the detail panel), exactly as a plugin without Commander would.
func TestEnterDetailCommanderNotOkFallsThrough(t *testing.T) {
	app := newTestApp()

	ctxPlugin := &commanderPlugin{
		mockPlugin: mockPlugin{
			name: "contexts",
			gvr:  schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "contexts"},
		},
		cmd: "goto-deployments",
		ok:  false,
	}
	plugin.Register(ctxPlugin)
	t.Cleanup(func() { plugin.Reset() })

	app.layout.AddSplit(ctxPlugin, "default", "")

	obj := &unstructured.Unstructured{}
	obj.SetName("some-context")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Open the panel so the default enter-detail behavior focuses details.
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	model, _ = app.executeCommand("enter-detail")
	app = model.(App)

	if app.layout.FocusedSplit().Plugin().Name() != "contexts" {
		t.Fatalf("ok==false should not dispatch a command; plugin changed to %q", app.layout.FocusedSplit().Plugin().Name())
	}
	if !app.layout.FocusedDetails() {
		t.Fatal("ok==false should fall through to default enter-detail (focus details)")
	}
}

// TestEnterDetailWithoutCommanderPreservesBehavior verifies that a plugin that
// does NOT implement Commander keeps the existing enter-detail behavior:
// with the panel open, focus moves to the detail panel.
func TestEnterDetailWithoutCommanderPreservesBehavior(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	t.Cleanup(func() { plugin.Reset() })
	app.layout.AddSplit(podsPlugin, "default", "")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Open the panel first.
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	model, _ = app.executeCommand("enter-detail")
	app = model.(App)

	if !app.layout.FocusedDetails() {
		t.Fatal("plugin without Commander should focus details on enter-detail")
	}
}
