package app

import (
	"errors"
	"slices"
	"testing"

	"github.com/aohoyd/aku/internal/cluster"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestHeartbeatRearmIsOneToOne locks the invariant that a health tick re-arms a
// heartbeat for EXACTLY the cluster that ticked — never the full set of
// pane-referenced clusters. Re-arming all of them on every tick made the
// in-flight heartbeat count double each interval once a second context existed,
// which was the CPU/memory blow-up. The scenario below mirrors that case: two
// connected clusters, each referenced by a pane.
func TestHeartbeatRearmIsOneToOne(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// Connect a second cluster and put a pane on it, so multiple connected
	// clusters are referenced by panes — the exact shape whose fan-out exploded.
	app.mgr.RegisterConnected("staging", paneFakeClient("staging", nil))
	pods := app.layout.FocusedSplit().Plugin()
	app.layout.AddSplit(pods, "default", "")
	app.layout.SplitAt(app.layout.SplitCount() - 1).SetContext("staging")

	// A tick re-arms exactly the cluster that ticked, regardless of how many
	// other connected clusters panes reference.
	if got := app.heartbeatRearmContexts("global"); !slices.Equal(got, []string{"global"}) {
		t.Fatalf("rearm(global) = %v, want [global] (one-to-one, no fan-out)", got)
	}
	if got := app.heartbeatRearmContexts("staging"); !slices.Equal(got, []string{"staging"}) {
		t.Fatalf("rearm(staging) = %v, want [staging]", got)
	}
}

// TestHeartbeatRearmDropsDeadClusters proves a tick for a cluster that is no
// longer a connected Manager entry re-arms nothing, so its probe loop ends with
// no leak (a torn-down cluster) and a degraded cluster is not probed via this
// loop.
func TestHeartbeatRearmDropsDeadClusters(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global": {testPod("global-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// Never registered → unknown.
	if got := app.heartbeatRearmContexts("never-dialed"); got != nil {
		t.Fatalf("rearm(never-dialed) = %v, want nil", got)
	}

	// Registered but degraded (has an error, no client) → Connected() is false,
	// so it is not re-armed.
	app.mgr.Register(cluster.New("down", "", nil, nil, nil, errors.New("dial refused")))
	if got := app.heartbeatRearmContexts("down"); got != nil {
		t.Fatalf("rearm(down) = %v, want nil (degraded cluster not re-armed)", got)
	}
}
