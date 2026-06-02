package contexts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Expected STATUS cell renderings.
var (
	statusGreen = plugin.StyledFg(glyphInUse, plugin.FgRunning) // connected
	statusRed   = plugin.StyledFg(glyphInUse, plugin.FgFailed)  // in-use but offline
	statusIdle  = glyphIdle                                     // no panes
)

// writeKubeconfig writes a minimal kubeconfig with the given context→cluster→server
// mappings and returns its path.
func writeKubeconfig(t *testing.T, entries map[string][2]string) string {
	t.Helper()
	b := "apiVersion: v1\nkind: Config\nclusters:\n"
	for _, v := range entries {
		clusterName, server := v[0], v[1]
		b += "- name: " + clusterName + "\n  cluster:\n    server: " + server + "\n"
	}
	b += "contexts:\n"
	for ctxName, v := range entries {
		b += "- name: " + ctxName + "\n  context:\n    cluster: " + v[0] + "\n"
	}
	b += "current-context: \"\"\nusers: []\n"

	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(b), 0o600); err != nil {
		t.Fatalf("write kubeconfig: %v", err)
	}
	return path
}

// findRow returns the object whose metadata.name == name.
func findRow(objs []*unstructured.Unstructured, name string) *unstructured.Unstructured {
	for _, o := range objs {
		if o.GetName() == name {
			return o
		}
	}
	return nil
}

func TestObjectsMapsEntriesAndRow(t *testing.T) {
	kc := writeKubeconfig(t, map[string][2]string{
		"alpha": {"alpha-cluster", "https://alpha.example:6443"},
		"beta":  {"beta-cluster", "https://beta.example:6443"},
	})
	entries := []cluster.ContextEntry{
		{Name: "alpha", File: kc},
		{Name: "beta", File: kc},
	}
	mgr := cluster.NewManager(entries, kc, time.Second)

	p := New(mgr)
	// alpha is referenced by panes but not connected → red ●; beta has no panes → ○.
	p.SetPaneCounts(map[string]int{"alpha": 2})

	objs := p.Objects()
	if len(objs) != 2 {
		t.Fatalf("Objects() len = %d, want 2", len(objs))
	}

	alpha := findRow(objs, "alpha")
	if alpha == nil {
		t.Fatal("no row for alpha")
	}
	row := p.Row(alpha)
	want := []string{"alpha", "alpha-cluster", "https://alpha.example:6443", statusRed}
	if !slices.Equal(row, want) {
		t.Errorf("Row(alpha) = %v, want %v", row, want)
	}

	beta := findRow(objs, "beta")
	if beta == nil {
		t.Fatal("no row for beta")
	}
	betaRow := p.Row(beta)
	wantBeta := []string{"beta", "beta-cluster", "https://beta.example:6443", statusIdle}
	if !slices.Equal(betaRow, wantBeta) {
		t.Errorf("Row(beta) = %v, want %v", betaRow, wantBeta)
	}
}

func TestStatusReflectsConnectionAndPanes(t *testing.T) {
	kc := writeKubeconfig(t, map[string][2]string{
		"up":     {"up-cluster", "https://up:6443"},
		"down":   {"down-cluster", "https://down:6443"},
		"idle":   {"idle-cluster", "https://idle:6443"},
		"failed": {"failed-cluster", "https://failed:6443"},
	})
	entries := []cluster.ContextEntry{
		{Name: "up", File: kc},
		{Name: "down", File: kc},
		{Name: "idle", File: kc},
		{Name: "failed", File: kc},
	}
	mgr := cluster.NewManager(entries, kc, time.Second)

	// "up": a connected cluster → green ● regardless of pane count.
	mgr.SetConnect(func(_, ctx string) (*k8s.Client, error) {
		return &k8s.Client{Context: ctx}, nil
	})
	if _, err := mgr.GetOrCreate("up"); err != nil {
		t.Fatalf("GetOrCreate(up): %v", err)
	}

	// "down": a degraded (offline) cluster cached with an error AND referenced by a
	// pane → red ●.
	mgr.Register(cluster.New("down", kc, nil, nil, nil, errors.New("dial refused")))

	// "failed": no Manager entry (a failed dial registers none) but a pane is on
	// it → red ●. This is the optimistic-switch-then-fail case.

	// "idle": never dialed, no panes → ○.

	p := New(mgr)
	p.SetPaneCounts(map[string]int{"down": 1, "failed": 1})
	objs := p.Objects()

	cases := map[string]string{
		"up":     statusGreen,
		"down":   statusRed,
		"failed": statusRed,
		"idle":   statusIdle,
	}
	for name, wantStatus := range cases {
		obj := findRow(objs, name)
		if obj == nil {
			t.Fatalf("no row for %q", name)
		}
		got, _, _ := unstructured.NestedString(obj.Object, "status")
		if got != wantStatus {
			t.Errorf("status(%q) = %q, want %q", name, got, wantStatus)
		}
	}
}

func TestDescribeContainsExpectedFields(t *testing.T) {
	kc := writeKubeconfig(t, map[string][2]string{
		"alpha": {"alpha-cluster", "https://alpha.example:6443"},
	})
	mgr := cluster.NewManager([]cluster.ContextEntry{{Name: "alpha", File: kc}}, kc, time.Second)
	p := New(mgr)

	obj := findRow(p.Objects(), "alpha")
	if obj == nil {
		t.Fatal("no row for alpha")
	}
	content, err := p.Describe(context.Background(), obj)
	if err != nil {
		t.Fatalf("Describe err = %v", err)
	}
	out := content.Raw
	for _, want := range []string{"Context", "Cluster", "Server", "Status"} {
		if !strings.Contains(out, want) {
			t.Errorf("Describe output missing field label %q; got:\n%s", want, out)
		}
	}
	// The resolved values should also appear. PANES is no longer shown.
	for _, want := range []string{"alpha", "alpha-cluster", "https://alpha.example:6443"} {
		if !strings.Contains(out, want) {
			t.Errorf("Describe output missing value %q; got:\n%s", want, out)
		}
	}
}

func TestObjectsDegradesGracefully(t *testing.T) {
	// Entry whose kubeconfig file does not exist, and an entry whose context has
	// no matching cluster entry in an otherwise-valid file. Both must degrade to
	// empty CLUSTER/SERVER without panicking.
	validKC := writeKubeconfig(t, map[string][2]string{
		"hascluster": {"some-cluster", "https://some:6443"},
	})
	mgr := cluster.NewManager([]cluster.ContextEntry{
		{Name: "missingfile", File: "/no/such/kubeconfig/path"},
		// "orphan" references validKC but has no context entry there.
		{Name: "orphan", File: validKC},
	}, validKC, time.Second)
	p := New(mgr)

	var objs []*unstructured.Unstructured
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Objects() panicked: %v", r)
			}
		}()
		objs = p.Objects()
	}()

	for _, name := range []string{"missingfile", "orphan"} {
		obj := findRow(objs, name)
		if obj == nil {
			t.Fatalf("no row for %q", name)
		}
		clusterName, _, _ := unstructured.NestedString(obj.Object, "cluster")
		server, _, _ := unstructured.NestedString(obj.Object, "server")
		if clusterName != "" || server != "" {
			t.Errorf("%q should degrade to empty cluster/server, got cluster=%q server=%q", name, clusterName, server)
		}
	}
}

func TestObjectsNilManagerReturnsNil(t *testing.T) {
	p := New(nil)
	if objs := p.Objects(); objs != nil {
		t.Errorf("Objects() with nil manager = %v, want nil", objs)
	}
}

func TestCommandReturnsSwitchContext(t *testing.T) {
	mgr := cluster.NewManager(nil, "", time.Second)
	p := New(mgr)

	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "prod"},
	}}
	cmd, ok := p.Command(obj)
	if !ok {
		t.Fatal("Command ok = false, want true")
	}
	if cmd != "pane-switch-context prod" {
		t.Errorf("Command = %q, want %q", cmd, "pane-switch-context prod")
	}

	// An object with no name does not produce a command.
	empty := &unstructured.Unstructured{Object: map[string]any{}}
	if _, ok := p.Command(empty); ok {
		t.Error("Command for empty name ok = true, want false")
	}
}

func TestDefaultSortByName(t *testing.T) {
	mgr := cluster.NewManager(nil, "", time.Second)
	p := New(mgr)

	pref := p.DefaultSort()
	if pref.Column != "NAME" || !pref.Ascending {
		t.Errorf("DefaultSort = %+v, want {NAME true}", pref)
	}

	// All columns fall back to the built-in (lexical) sort.
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "alpha"},
	}}
	if v := p.SortValue(obj, "NAME"); v != "" {
		t.Errorf("SortValue(NAME) = %q, want empty (built-in fallback)", v)
	}
	if v := p.SortValue(obj, "STATUS"); v != "" {
		t.Errorf("SortValue(STATUS) = %q, want empty (built-in fallback)", v)
	}
}

func TestPluginMetadata(t *testing.T) {
	p := New(cluster.NewManager(nil, "", time.Second))
	if p.Name() != "contexts" {
		t.Errorf("Name = %q, want contexts", p.Name())
	}
	if p.ShortName() != "ctx" {
		t.Errorf("ShortName = %q, want ctx", p.ShortName())
	}
	if !p.IsClusterScoped() {
		t.Error("IsClusterScoped = false, want true")
	}
	gvr := p.GVR()
	if gvr.Group != "_ktui" || gvr.Version != "v1" || gvr.Resource != "contexts" {
		t.Errorf("GVR = %+v, want {_ktui v1 contexts}", gvr)
	}
	cols := p.Columns()
	wantCols := []string{"NAME", "CLUSTER", "SERVER", "STATUS"}
	if len(cols) != len(wantCols) {
		t.Fatalf("Columns len = %d, want %d", len(cols), len(wantCols))
	}
	for i, c := range cols {
		if c.Title != wantCols[i] {
			t.Errorf("Columns[%d].Title = %q, want %q", i, c.Title, wantCols[i])
		}
	}
}
