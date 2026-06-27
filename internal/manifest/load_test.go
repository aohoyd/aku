package manifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	testDeploymentsGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	testPodsGVR        = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	testNamespacesGVR  = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
)

const loadDeploymentYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: foo
spec:
  replicas: 2
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: c
          image: nginx
`

func TestLoadDualKeysNamespacedObjects(t *testing.T) {
	cl, warns, err := Load(strings.NewReader(loadDeploymentYAML), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}

	st := cl.Store()
	if st == nil {
		t.Fatalf("expected non-nil store")
	}

	// Deployment present in the foo bucket and the all-namespaces ("") bucket.
	if got := st.List(testDeploymentsGVR, "foo"); len(got) != 1 {
		t.Fatalf("expected 1 deployment in ns foo, got %d", len(got))
	}
	if got := st.List(testDeploymentsGVR, ""); len(got) != 1 {
		t.Fatalf("expected 1 deployment in all-namespaces bucket, got %d", len(got))
	}

	// Synthesized pods present in both buckets (replicas=2).
	if got := st.List(testPodsGVR, "foo"); len(got) != 2 {
		t.Fatalf("expected 2 pods in ns foo, got %d", len(got))
	}
	if got := st.List(testPodsGVR, ""); len(got) != 2 {
		t.Fatalf("expected 2 pods in all-namespaces bucket, got %d", len(got))
	}
}

func TestLoadDiscoveryResolvesGVR(t *testing.T) {
	cl, _, err := Load(strings.NewReader(loadDeploymentYAML), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	disc := cl.Discovery()
	if disc == nil {
		t.Fatalf("expected non-nil discovery")
	}

	gvr, ok := disc.ResolveGVR("apps/v1", "Deployment")
	if !ok {
		t.Fatalf("expected to resolve apps/v1 Deployment")
	}
	if gvr.Resource != "deployments" {
		t.Fatalf("expected resource 'deployments', got %q", gvr.Resource)
	}
	if gvr != testDeploymentsGVR {
		t.Fatalf("resolved GVR %v does not match bucket GVR %v", gvr, testDeploymentsGVR)
	}
	// The deployment must actually live under the resolved GVR's bucket.
	if got := cl.Store().List(gvr, "foo"); len(got) != 1 {
		t.Fatalf("expected deployment under resolved GVR bucket, got %d", len(got))
	}
}

func TestLoadClusterScopedKind(t *testing.T) {
	in := `apiVersion: v1
kind: Namespace
metadata:
  name: prod
`
	cl, _, err := Load(strings.NewReader(in), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gvr, ok := cl.Discovery().ResolveGVR("v1", "Namespace")
	if !ok {
		t.Fatalf("expected to resolve v1 Namespace")
	}
	if gvr.Resource != "namespaces" {
		t.Fatalf("expected resource 'namespaces', got %q", gvr.Resource)
	}
	if gvr != testNamespacesGVR {
		t.Fatalf("resolved GVR %v does not match expected %v", gvr, testNamespacesGVR)
	}
	// Cluster-scoped objects live only under the "" bucket.
	if got := cl.Store().List(testNamespacesGVR, ""); len(got) != 1 {
		t.Fatalf("expected 1 namespace under '' bucket, got %d", len(got))
	}
}

func TestLoadNotConnected(t *testing.T) {
	cl, _, err := Load(strings.NewReader(loadDeploymentYAML), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cl.Connected() {
		t.Fatalf("expected manifest cluster to report not connected")
	}
}

func TestLoadPropagatesWarnings(t *testing.T) {
	in := `apiVersion: v1
kind: ConfigMap
metadata:
  name: good
---
this: is: not: valid: yaml: at: all
  - broken
`
	_, warns, err := Load(strings.NewReader(in), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) == 0 {
		t.Fatalf("expected at least one warning propagated from parse")
	}
}

func TestLoadEmptyInput(t *testing.T) {
	cl, warns, err := Load(strings.NewReader(""), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cl == nil {
		t.Fatalf("expected non-nil cluster for empty input")
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings for empty input, got %v", warns)
	}
	if cl.Store() == nil {
		t.Fatalf("expected non-nil store for empty input")
	}
	// Empty store: no objects under any bucket.
	if got := cl.Store().List(testPodsGVR, ""); len(got) != 0 {
		t.Fatalf("expected empty store, got %d pods", len(got))
	}
}

func TestLoadGuessedPluralWarns(t *testing.T) {
	// A kind not in the static table; loader should derive a best-effort plural
	// and record a warning.
	in := `apiVersion: example.com/v1
kind: Widget
metadata:
  name: w
  namespace: foo
`
	cl, warns, err := Load(strings.NewReader(in), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) == 0 {
		t.Fatalf("expected a warning for guessed plural")
	}
	gvr, ok := cl.Discovery().ResolveGVR("example.com/v1", "Widget")
	if !ok {
		t.Fatalf("expected to resolve example.com/v1 Widget")
	}
	if gvr.Resource != "widgets" {
		t.Fatalf("expected guessed plural 'widgets', got %q", gvr.Resource)
	}
}

func TestLoadEndpointSliceNoGuessWarning(t *testing.T) {
	// EndpointSlice is a built-in kind: it must resolve to discovery.k8s.io/v1
	// endpointslices via the static table, never the guessPlural fallback, so no
	// "guessed plural" warning fires for it.
	in := `apiVersion: discovery.k8s.io/v1
kind: EndpointSlice
metadata:
  name: web-abc
  namespace: foo
addressType: IPv4
`
	cl, warns, err := Load(strings.NewReader(in), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, w := range warns {
		if strings.Contains(w.Reason, "EndpointSlice") {
			t.Fatalf("expected no guessed-plural warning for EndpointSlice, got %q", w.Reason)
		}
	}
	gvr, ok := cl.Discovery().ResolveGVR("discovery.k8s.io/v1", "EndpointSlice")
	if !ok {
		t.Fatalf("expected to resolve discovery.k8s.io/v1 EndpointSlice")
	}
	if gvr.Group != "discovery.k8s.io" || gvr.Version != "v1" || gvr.Resource != "endpointslices" {
		t.Fatalf("expected discovery.k8s.io/v1 endpointslices, got %+v", gvr)
	}
}

func TestLoadGuessedKindIsListable(t *testing.T) {
	// An unknown/CRD kind must remain browsable: resolvable via discovery AND
	// listable from the store under both its namespace and the all-namespaces
	// bucket, in addition to producing a warning.
	in := `apiVersion: example.com/v1
kind: Foo
metadata:
  name: myfoo
  namespace: foo
`
	cl, warns, err := Load(strings.NewReader(in), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) == 0 {
		t.Fatalf("expected a warning for the unknown kind")
	}

	gvr, ok := cl.Discovery().ResolveGVR("example.com/v1", "Foo")
	if !ok {
		t.Fatalf("expected to resolve example.com/v1 Foo")
	}
	if gvr.Resource != "foos" {
		t.Fatalf("expected guessed plural 'foos', got %q", gvr.Resource)
	}
	// Listable in its namespace and in the all-namespaces bucket (browseable).
	if got := cl.Store().List(gvr, "foo"); len(got) != 1 {
		t.Fatalf("expected 1 Foo listable in ns foo, got %d", len(got))
	}
	if got := cl.Store().List(gvr, ""); len(got) != 1 {
		t.Fatalf("expected 1 Foo listable in all-namespaces bucket, got %d", len(got))
	}
}

func TestLoadMalformedAPIVersionFallback(t *testing.T) {
	// An apiVersion that ParseGroupVersion rejects (more than one "/") must not
	// crash the load: resolveGVR falls back to {Version: "v1"} (empty group) so the
	// object is still stored and listable under that fallback GVR.
	in := `apiVersion: v1/v2/v3
kind: Widget
metadata:
  name: w
  namespace: foo
`
	cl, _, err := Load(strings.NewReader(in), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Fallback group is "", version "v1"; plural is guessed from the kind.
	fallbackGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "widgets"}
	if got := cl.Store().List(fallbackGVR, "foo"); len(got) != 1 {
		t.Fatalf("expected 1 Widget listable under fallback GVR in ns foo, got %d", len(got))
	}
	if got := cl.Store().List(fallbackGVR, ""); len(got) != 1 {
		t.Fatalf("expected 1 Widget listable under fallback GVR in all-namespaces bucket, got %d", len(got))
	}
}

func TestLoadDuplicateObjectLastWins(t *testing.T) {
	// The same kind+ns+name appearing twice in the stream must collapse to a
	// single stored object carrying the second document's content (last wins),
	// matching the store's keyed-upsert semantics.
	in := `apiVersion: v1
kind: ConfigMap
metadata:
  name: dup
  namespace: foo
data:
  v: "1"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: dup
  namespace: foo
data:
  v: "2"
`
	cl, _, err := Load(strings.NewReader(in), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cmGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	cms := cl.Store().List(cmGVR, "foo")
	if len(cms) != 1 {
		t.Fatalf("expected duplicate to collapse to 1 ConfigMap, got %d", len(cms))
	}
	v, _, _ := unstructured.NestedString(cms[0].Object, "data", "v")
	if v != "2" {
		t.Fatalf("expected last document to win (data.v=2), got %q", v)
	}
}

func TestLoadBarePodIsBrowseable(t *testing.T) {
	// A bare user Pod (no controller, no status) must still be listable. It is
	// NOT run through markPodHealthy (that only stamps synthesized pods), so it
	// passes through with whatever status it had — here, an empty phase. This
	// documents the current behavior: a bare status-less Pod is browseable but
	// not auto-marked healthy.
	in := `apiVersion: v1
kind: Pod
metadata:
  name: solo
  namespace: foo
spec:
  containers:
    - name: c
      image: nginx
`
	cl, warns, err := Load(strings.NewReader(in), "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings for a bare Pod, got %v", warns)
	}
	pods := cl.Store().List(testPodsGVR, "")
	if len(pods) != 1 {
		t.Fatalf("expected the bare Pod to be browseable (1 listed), got %d", len(pods))
	}
	if pods[0].GetName() != "solo" {
		t.Fatalf("expected the bare Pod 'solo', got %q", pods[0].GetName())
	}
	// Current behavior: a bare Pod with no status is passed through untouched —
	// the synthesizer only stamps health on pods it fabricates.
	phase, _, _ := unstructured.NestedString(pods[0].Object, "status", "phase")
	if phase != "" {
		t.Fatalf("expected bare Pod to keep its empty phase (passed through), got %q", phase)
	}
}

// TestLoadFilesMultipleFilesAndNestedDir passes a DIRECTORY path containing
// .yaml files in nested subdirs and asserts all are loaded, exercising the
// documented `aku -f ./manifests/` recursive-scan behavior.
func TestLoadFilesMultipleFilesAndNestedDir(t *testing.T) {
	dir := t.TempDir()

	a := filepath.Join(dir, "a.yaml")
	if err := os.WriteFile(a, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-a
  namespace: foo
`), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}

	nested := filepath.Join(dir, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	b := filepath.Join(nested, "b.yml")
	if err := os.WriteFile(b, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-b
  namespace: foo
`), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	// Pass the directory itself — it must be scanned recursively for *.yaml/*.yml.
	cl, warns, err := LoadFiles([]string{dir}, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %v", warns)
	}

	cmGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	got := cl.Store().List(cmGVR, "foo")
	if len(got) != 2 {
		t.Fatalf("expected 2 configmaps from nested dir scan, got %d", len(got))
	}
}

// TestLoadFilesDirIgnoresNonYAML asserts that scanning a directory only picks up
// *.yaml/*.yml files and silently ignores everything else (README, .json, etc.).
func TestLoadFilesDirIgnoresNonYAML(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-a
  namespace: foo
`), 0o644); err != nil {
		t.Fatalf("write cm.yaml: %v", err)
	}
	// Non-YAML files that must be ignored. If the .json/.txt content were read and
	// parsed as YAML it would either add objects or produce warnings.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# not a manifest\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte(`{"kind":"ConfigMap","metadata":{"name":"json-cm"}}`), 0o644); err != nil {
		t.Fatalf("write data.json: %v", err)
	}

	cl, warns, err := LoadFiles([]string{dir}, "default")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("expected no warnings (non-yaml ignored), got %v", warns)
	}

	cmGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	got := cl.Store().List(cmGVR, "foo")
	if len(got) != 1 {
		t.Fatalf("expected only the 1 yaml ConfigMap, got %d", len(got))
	}
	if got[0].GetName() != "cm-a" {
		t.Fatalf("expected cm-a, got %q", got[0].GetName())
	}
}

func TestGuessPlural(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		// y -> ies
		{"Policy", "policies"},
		{"Proxy", "proxies"},
		// trailing s -> ses
		{"Ingress", "ingresses"},
		{"Bus", "buses"},
		// default -> +s
		{"Widget", "widgets"},
		{"Foo", "foos"},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			if got := guessPlural(tt.kind); got != tt.want {
				t.Fatalf("guessPlural(%q) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func TestLoadFilesMissingPathWarns(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.yaml")
	if err := os.WriteFile(a, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: cm-a
  namespace: foo
`), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	missing := filepath.Join(dir, "does-not-exist.yaml")

	cl, warns, err := LoadFiles([]string{a, missing}, "default")
	if err != nil {
		t.Fatalf("expected no hard error for missing path, got %v", err)
	}
	if len(warns) == 0 {
		t.Fatalf("expected a warning for the missing path")
	}
	cmGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	if got := cl.Store().List(cmGVR, "foo"); len(got) != 1 {
		t.Fatalf("expected 1 configmap from the existing file, got %d", len(got))
	}
}
