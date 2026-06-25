package manifest

import (
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// newObj is a small helper for building unstructured test objects.
func newObj(apiVersion, kind, namespace, name string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{Object: map[string]any{}}
	u.SetAPIVersion(apiVersion)
	u.SetKind(kind)
	if namespace != "" {
		u.SetNamespace(namespace)
	}
	u.SetName(name)
	return u
}

// findNamespaces returns the fabricated/passed-through Namespace objects keyed by name.
func collectNamespaces(objs []*unstructured.Unstructured) map[string]*unstructured.Unstructured {
	out := map[string]*unstructured.Unstructured{}
	for _, o := range objs {
		if o.GetKind() == "Namespace" && o.GetAPIVersion() == "v1" {
			out[o.GetName()] = o
		}
	}
	return out
}

func TestAssignNamespacesStampsDefault(t *testing.T) {
	cm := newObj("v1", "ConfigMap", "", "cfg")
	out := assignNamespaces([]*unstructured.Unstructured{cm}, "myns")

	// The original object should have been stamped with the default namespace.
	if got := cm.GetNamespace(); got != "myns" {
		t.Fatalf("expected ConfigMap namespace 'myns', got %q", got)
	}

	// Exactly one Namespace object (for "myns") should be fabricated.
	nss := collectNamespaces(out)
	if len(nss) != 1 {
		t.Fatalf("expected 1 fabricated Namespace, got %d: %v", len(nss), keys(nss))
	}
	ns, ok := nss["myns"]
	if !ok {
		t.Fatalf("expected fabricated Namespace 'myns', got %v", keys(nss))
	}
	phase, _, _ := unstructured.NestedString(ns.Object, "status", "phase")
	if phase != "Active" {
		t.Fatalf("expected fabricated Namespace phase 'Active', got %q", phase)
	}

	// A fabricated Namespace must carry a stable uid and the synthesized
	// provenance annotation, matching newChild/fabricateEndpoints.
	if string(ns.GetUID()) == "" {
		t.Fatalf("expected fabricated Namespace to carry a stable uid")
	}
	if ns.GetUID() != types.UID(stableUID("Namespace", "", "myns")) {
		t.Fatalf("expected fabricated Namespace uid to be stableUID-derived, got %q", ns.GetUID())
	}
	if ns.GetAnnotations()[SourceAnnotation] != "synthesized" {
		t.Fatalf("expected fabricated Namespace annotation %s=synthesized, got %q", SourceAnnotation, ns.GetAnnotations()[SourceAnnotation])
	}
}

func TestAssignNamespacesLeavesClusterScopedUntouched(t *testing.T) {
	tests := []struct {
		apiVersion string
		kind       string
	}{
		{"v1", "Namespace"},
		{"rbac.authorization.k8s.io/v1", "ClusterRole"},
		{"v1", "Node"},
		{"v1", "PersistentVolume"},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			obj := newObj(tt.apiVersion, tt.kind, "", "thing")
			out := assignNamespaces([]*unstructured.Unstructured{obj}, "myns")

			if got := obj.GetNamespace(); got != "" {
				t.Fatalf("expected cluster-scoped %s to keep empty namespace, got %q", tt.kind, got)
			}
			// A cluster-scoped object must not cause any Namespace fabrication:
			// nothing beyond the single input object should be returned.
			if len(out) != 1 {
				t.Fatalf("expected cluster-scoped %s to add no fabricated objects, got %d objects", tt.kind, len(out))
			}
			if out[0] != obj {
				t.Fatalf("expected the single returned object to be the input %s", tt.kind)
			}
		})
	}
}

func TestAssignNamespacesOneNamespacePerDistinct(t *testing.T) {
	objs := []*unstructured.Unstructured{
		newObj("v1", "ConfigMap", "alpha", "a"),
		newObj("v1", "ConfigMap", "alpha", "b"), // same ns -> no second fabrication
		newObj("v1", "ConfigMap", "beta", "c"),
		newObj("v1", "ConfigMap", "", "d"), // no ns -> default "gamma"
	}
	out := assignNamespaces(objs, "gamma")

	nss := collectNamespaces(out)
	want := map[string]bool{"alpha": true, "beta": true, "gamma": true}
	if len(nss) != len(want) {
		t.Fatalf("expected %d fabricated Namespaces, got %d: %v", len(want), len(nss), keys(nss))
	}
	for name := range want {
		if _, ok := nss[name]; !ok {
			t.Fatalf("expected fabricated Namespace %q, got %v", name, keys(nss))
		}
	}
}

func TestAssignNamespacesDoesNotDuplicateExisting(t *testing.T) {
	existing := newObj("v1", "Namespace", "", "alpha")
	objs := []*unstructured.Unstructured{
		existing,
		newObj("v1", "ConfigMap", "alpha", "a"), // references existing "alpha"
		newObj("v1", "ConfigMap", "beta", "b"),  // references new "beta"
	}
	out := assignNamespaces(objs, "default")

	nss := collectNamespaces(out)
	// alpha (already present) + beta (fabricated). No duplicate "alpha".
	if len(nss) != 2 {
		t.Fatalf("expected 2 Namespace objects (alpha existing + beta fabricated), got %d: %v", len(nss), keys(nss))
	}

	// Ensure exactly one Namespace named "alpha" and it is the original instance.
	var alphaCount int
	for _, o := range out {
		if o.GetKind() == "Namespace" && o.GetName() == "alpha" {
			alphaCount++
			if o != existing {
				t.Fatalf("expected the existing 'alpha' Namespace instance to be reused, got a different object")
			}
		}
	}
	if alphaCount != 1 {
		t.Fatalf("expected exactly 1 Namespace named 'alpha', got %d", alphaCount)
	}
}

func TestAssignNamespacesReturnsInputsAndFabricated(t *testing.T) {
	cm := newObj("v1", "ConfigMap", "", "cfg")
	out := assignNamespaces([]*unstructured.Unstructured{cm}, "myns")

	// Returned slice must include the original (stamped) ConfigMap and the fabricated Namespace.
	var sawCM, sawNS bool
	for _, o := range out {
		switch {
		case o.GetKind() == "ConfigMap" && o.GetName() == "cfg":
			sawCM = true
			if o.GetNamespace() != "myns" {
				t.Fatalf("expected returned ConfigMap stamped with 'myns', got %q", o.GetNamespace())
			}
		case o.GetKind() == "Namespace" && o.GetName() == "myns":
			sawNS = true
		}
	}
	if !sawCM {
		t.Fatalf("expected returned slice to contain the input ConfigMap")
	}
	if !sawNS {
		t.Fatalf("expected returned slice to contain the fabricated Namespace")
	}
}

func TestStableUIDDeterministic(t *testing.T) {
	a := stableUID("Pod", "ns", "web")
	b := stableUID("Pod", "ns", "web")
	if a == "" {
		t.Fatalf("expected non-empty UID")
	}
	if a != b {
		t.Fatalf("expected stableUID to be deterministic, got %q then %q", a, b)
	}
	// UUID-shaped: 36 chars, 8-4-4-4-12 with hyphens.
	if len(a) != 36 || a[8] != '-' || a[13] != '-' || a[18] != '-' || a[23] != '-' {
		t.Fatalf("expected UUID-shaped string, got %q", a)
	}
}

func TestStableUIDDistinct(t *testing.T) {
	base := stableUID("Pod", "ns", "web")
	tests := []struct {
		name             string
		kind, ns, objNam string
	}{
		{"different kind", "Deployment", "ns", "web"},
		{"different namespace", "Pod", "other", "web"},
		{"different name", "Pod", "ns", "api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stableUID(tt.kind, tt.ns, tt.objNam); got == base {
				t.Fatalf("expected distinct UID for %s, got same as base %q", tt.name, got)
			}
		})
	}
}

func newPod(namespace, name string, containers ...string) *unstructured.Unstructured {
	p := newObj("v1", "Pod", namespace, name)
	cs := make([]any, 0, len(containers))
	for _, c := range containers {
		cs = append(cs, map[string]any{"name": c, "image": "img"})
	}
	_ = unstructured.SetNestedSlice(p.Object, cs, "spec", "containers")
	return p
}

func TestMarkPodHealthyStampsStatus(t *testing.T) {
	pod := newPod("ns", "web", "app", "sidecar")
	markPodHealthy(pod)

	phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
	if phase != "Running" {
		t.Fatalf("expected status.phase 'Running', got %q", phase)
	}

	ip, _, _ := unstructured.NestedString(pod.Object, "status", "podIP")
	if ip == "" {
		t.Fatalf("expected non-empty status.podIP")
	}

	start, _, _ := unstructured.NestedString(pod.Object, "status", "startTime")
	if start == "" {
		t.Fatalf("expected non-empty status.startTime")
	}

	cs, found, _ := unstructured.NestedSlice(pod.Object, "status", "containerStatuses")
	if !found || len(cs) != 2 {
		t.Fatalf("expected 2 containerStatuses, found=%v len=%d", found, len(cs))
	}
	for i, entry := range cs {
		m, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("containerStatuses[%d] not a map: %T", i, entry)
		}
		ready, _, _ := unstructured.NestedBool(m, "ready")
		if !ready {
			t.Fatalf("expected containerStatuses[%d].ready=true", i)
		}
		started, _, _ := unstructured.NestedBool(m, "started")
		if !started {
			t.Fatalf("expected containerStatuses[%d].started=true", i)
		}
		_, running, _ := unstructured.NestedMap(m, "state", "running")
		if !running {
			t.Fatalf("expected containerStatuses[%d].state.running to be present", i)
		}
	}
}

func TestPodIPOctetsInUsableRange(t *testing.T) {
	// podIP must clamp every variable octet (b, c, d) to 1..254 so it never yields
	// a network/broadcast-ish address. Hash many distinct identities to exercise a
	// wide spread of SHA1 bytes, including ones that would map to 0 or 255 without
	// clamping.
	for i := range 5000 {
		pod := newPod("ns", fmt.Sprintf("pod-%d", i), "c")
		ip := podIP(pod)
		var a, b, c, d int
		if _, err := fmt.Sscanf(ip, "%d.%d.%d.%d", &a, &b, &c, &d); err != nil {
			t.Fatalf("podIP returned unparseable address %q: %v", ip, err)
		}
		if a != 10 {
			t.Fatalf("expected 10.x.y.z, got %q", ip)
		}
		for _, octet := range []struct {
			name string
			val  int
		}{{"b", b}, {"c", c}, {"d", d}} {
			if octet.val < 1 || octet.val > 254 {
				t.Fatalf("octet %s out of usable range 1..254 in %q: %d", octet.name, ip, octet.val)
			}
		}
	}
}

func TestMarkPodHealthyPreservesExisting(t *testing.T) {
	pod := newPod("ns", "web", "app")
	_ = unstructured.SetNestedField(pod.Object, "Pending", "status", "phase")
	markPodHealthy(pod)

	phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase")
	if phase != "Pending" {
		t.Fatalf("expected pre-existing status.phase 'Pending' to be preserved, got %q", phase)
	}
	// No status should have been stamped over.
	if ip, found, _ := unstructured.NestedString(pod.Object, "status", "podIP"); found && ip != "" {
		t.Fatalf("expected no fabricated podIP when status preexists, got %q", ip)
	}
}

func newDeployment(namespace, name string, replicas *int64) *unstructured.Unstructured {
	d := newObj("apps/v1", "Deployment", namespace, name)
	if replicas != nil {
		_ = unstructured.SetNestedField(d.Object, *replicas, "spec", "replicas")
	}
	return d
}

func TestMarkDeploymentHealthyStampsStatus(t *testing.T) {
	tests := []struct {
		name     string
		replicas *int64
		want     int64
	}{
		{"explicit replicas", new(int64(3)), 3},
		{"default replicas", nil, 1},
		{"zero replicas", new(int64(0)), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep := newDeployment("ns", "web", tt.replicas)
			markDeploymentHealthy(dep)

			for _, field := range []string{"replicas", "readyReplicas", "availableReplicas", "updatedReplicas"} {
				got, found, _ := unstructured.NestedInt64(dep.Object, "status", field)
				if !found {
					t.Fatalf("expected status.%s to be set", field)
				}
				if got != tt.want {
					t.Fatalf("expected status.%s=%d, got %d", field, tt.want, got)
				}
			}
		})
	}
}

func TestMarkDeploymentHealthyPreservesExisting(t *testing.T) {
	dep := newDeployment("ns", "web", new(int64(3)))
	_ = unstructured.SetNestedField(dep.Object, int64(1), "status", "readyReplicas")
	markDeploymentHealthy(dep)

	got, _, _ := unstructured.NestedInt64(dep.Object, "status", "readyReplicas")
	if got != 1 {
		t.Fatalf("expected pre-existing status.readyReplicas=1 to be preserved, got %d", got)
	}

	// The guard is all-or-nothing: because a status block already existed, the
	// other replica-count fields must NOT be stamped (the synthesizer does not
	// merge into a partially-populated status).
	for _, field := range []string{"replicas", "availableReplicas", "updatedReplicas"} {
		if _, found, _ := unstructured.NestedInt64(dep.Object, "status", field); found {
			t.Fatalf("expected status.%s to remain absent when a status block preexists, but it was stamped", field)
		}
	}
}

func TestMarkDeploymentHealthyPreservesExplicitEmptyStatus(t *testing.T) {
	// A user who ships `status: {}` explicitly must keep their empty status; the
	// synthesizer must not stamp synthetic replica counts over it.
	dep := newDeployment("ns", "web", new(int64(3)))
	_ = unstructured.SetNestedMap(dep.Object, map[string]any{}, "status")
	markDeploymentHealthy(dep)

	status, found, _ := unstructured.NestedMap(dep.Object, "status")
	if !found {
		t.Fatalf("expected status to remain present")
	}
	if len(status) != 0 {
		t.Fatalf("expected explicit empty status to be preserved, got %v", status)
	}
}

// --- synthesizeWorkloads tests ---

// childrenOf returns the fabricated objects whose ownerReferences[0].uid matches
// the given uid.
func childrenOf(objs []*unstructured.Unstructured, ownerUID string) []*unstructured.Unstructured {
	out := []*unstructured.Unstructured{}
	for _, o := range objs {
		refs := o.GetOwnerReferences()
		if len(refs) > 0 && string(refs[0].UID) == ownerUID {
			out = append(out, o)
		}
	}
	return out
}

// byKind returns objects of the given kind from a slice.
func byKind(objs []*unstructured.Unstructured, kind string) []*unstructured.Unstructured {
	out := []*unstructured.Unstructured{}
	for _, o := range objs {
		if o.GetKind() == kind {
			out = append(out, o)
		}
	}
	return out
}

func TestSynthesizeWorkloadsDeployment(t *testing.T) {
	dep := newDeployment("ns", "web", new(int64(3)))
	_ = unstructured.SetNestedMap(dep.Object, map[string]any{"app": "web"}, "spec", "template", "metadata", "labels")
	_ = unstructured.SetNestedSlice(dep.Object, []any{
		map[string]any{"name": "app", "image": "nginx"},
	}, "spec", "template", "spec", "containers")

	out := synthesizeWorkloads([]*unstructured.Unstructured{dep})

	// The Deployment must get a stable uid.
	depUID := string(dep.GetUID())
	if depUID == "" {
		t.Fatalf("expected Deployment to receive a stable uid")
	}
	if depUID != stableUID("Deployment", "ns", "web") {
		t.Fatalf("expected Deployment uid to be stableUID-derived, got %q", depUID)
	}

	// Exactly 1 ReplicaSet owned by the Deployment.
	rsList := byKind(out, "ReplicaSet")
	if len(rsList) != 1 {
		t.Fatalf("expected exactly 1 ReplicaSet, got %d", len(rsList))
	}
	rs := rsList[0]
	if rs.GetAPIVersion() != "apps/v1" {
		t.Fatalf("expected ReplicaSet apiVersion apps/v1, got %q", rs.GetAPIVersion())
	}
	if rs.GetNamespace() != "ns" {
		t.Fatalf("expected ReplicaSet namespace ns, got %q", rs.GetNamespace())
	}
	rsRefs := rs.GetOwnerReferences()
	if len(rsRefs) == 0 || string(rsRefs[0].UID) != depUID {
		t.Fatalf("expected ReplicaSet ownerReference uid == deployment uid")
	}
	if rsRefs[0].Controller == nil || !*rsRefs[0].Controller {
		t.Fatalf("expected ReplicaSet ownerReference controller=true")
	}
	rsUID := string(rs.GetUID())
	if rsUID == "" {
		t.Fatalf("expected ReplicaSet to have its own uid")
	}
	rsReplicas, found, _ := unstructured.NestedInt64(rs.Object, "spec", "replicas")
	if !found || rsReplicas != 3 {
		t.Fatalf("expected ReplicaSet spec.replicas=3, got %d (found=%v)", rsReplicas, found)
	}
	if !strings.HasPrefix(rs.GetName(), "web-") {
		t.Fatalf("expected ReplicaSet name like web-<suffix>, got %q", rs.GetName())
	}

	// Exactly 3 Pods owned by the ReplicaSet.
	pods := childrenOf(out, rsUID)
	if len(pods) != 3 {
		t.Fatalf("expected exactly 3 Pods owned by ReplicaSet, got %d", len(pods))
	}
	seen := map[string]bool{}
	for _, p := range pods {
		if p.GetKind() != "Pod" || p.GetAPIVersion() != "v1" {
			t.Fatalf("expected Pod v1, got %s/%s", p.GetAPIVersion(), p.GetKind())
		}
		if p.GetNamespace() != "ns" {
			t.Fatalf("expected Pod namespace ns, got %q", p.GetNamespace())
		}
		if !strings.HasPrefix(p.GetName(), rs.GetName()+"-") {
			t.Fatalf("expected Pod name like %s-<suffix>, got %q", rs.GetName(), p.GetName())
		}
		if seen[p.GetName()] {
			t.Fatalf("duplicate Pod name %q", p.GetName())
		}
		seen[p.GetName()] = true

		// Pod labels copied from template.
		labels := p.GetLabels()
		if labels["app"] != "web" {
			t.Fatalf("expected Pod label app=web, got %v", labels)
		}
		// Pod spec copied from template.spec.
		containers, found, _ := unstructured.NestedSlice(p.Object, "spec", "containers")
		if !found || len(containers) != 1 {
			t.Fatalf("expected 1 container copied to pod spec, got %d (found=%v)", len(containers), found)
		}
		// Healthy.
		phase, _, _ := unstructured.NestedString(p.Object, "status", "phase")
		if phase != "Running" {
			t.Fatalf("expected Pod phase Running, got %q", phase)
		}
	}

	// Deployment marked healthy.
	ready, found, _ := unstructured.NestedInt64(dep.Object, "status", "readyReplicas")
	if !found || ready != 3 {
		t.Fatalf("expected Deployment status.readyReplicas=3, got %d (found=%v)", ready, found)
	}

	// Fabricated pod carries provenance annotation.
	ann := pods[0].GetAnnotations()
	if ann[SourceAnnotation] != "synthesized" {
		t.Fatalf("expected fabricated Pod annotation %s=synthesized, got %q", SourceAnnotation, ann[SourceAnnotation])
	}

	// Fabricated ReplicaSet carries provenance annotation too.
	if rsAnn := rs.GetAnnotations(); rsAnn[SourceAnnotation] != "synthesized" {
		t.Fatalf("expected fabricated ReplicaSet annotation %s=synthesized, got %q", SourceAnnotation, rsAnn[SourceAnnotation])
	}
}

func TestSynthesizeWorkloadsDeploymentZeroReplicas(t *testing.T) {
	dep := newDeployment("ns", "web", new(int64(0)))
	out := synthesizeWorkloads([]*unstructured.Unstructured{dep})

	if len(byKind(out, "ReplicaSet")) != 1 {
		t.Fatalf("expected 1 ReplicaSet for zero-replica Deployment")
	}
	if pods := byKind(out, "Pod"); len(pods) != 0 {
		t.Fatalf("expected 0 Pods for zero-replica Deployment, got %d", len(pods))
	}
}

func TestSynthesizeWorkloadsDeploymentUnsetReplicas(t *testing.T) {
	dep := newDeployment("ns", "web", nil)
	out := synthesizeWorkloads([]*unstructured.Unstructured{dep})

	if pods := byKind(out, "Pod"); len(pods) != 1 {
		t.Fatalf("expected 1 Pod for unset-replica Deployment (default 1), got %d", len(pods))
	}
}

func TestSynthesizeWorkloadsStatefulSet(t *testing.T) {
	sts := newObj("apps/v1", "StatefulSet", "ns", "db")
	_ = unstructured.SetNestedField(sts.Object, int64(2), "spec", "replicas")

	out := synthesizeWorkloads([]*unstructured.Unstructured{sts})

	stsUID := string(sts.GetUID())
	if stsUID == "" {
		t.Fatalf("expected StatefulSet to receive a uid")
	}

	pods := childrenOf(out, stsUID)
	if len(pods) != 2 {
		t.Fatalf("expected 2 Pods for StatefulSet, got %d", len(pods))
	}
	names := map[string]bool{}
	for _, p := range pods {
		names[p.GetName()] = true
		if p.GetNamespace() != "ns" {
			t.Fatalf("expected Pod namespace ns, got %q", p.GetNamespace())
		}
		phase, _, _ := unstructured.NestedString(p.Object, "status", "phase")
		if phase != "Running" {
			t.Fatalf("expected StatefulSet Pod healthy, got phase %q", phase)
		}
	}
	if !names["db-0"] || !names["db-1"] {
		t.Fatalf("expected StatefulSet pods db-0 and db-1, got %v", names)
	}

	// The StatefulSet itself must be stamped healthy (N/N ready) so the plugin
	// doesn't render it 0/N.
	for _, field := range []string{"replicas", "readyReplicas", "currentReplicas", "updatedReplicas"} {
		got, found, _ := unstructured.NestedInt64(sts.Object, "status", field)
		if !found || got != 2 {
			t.Fatalf("expected StatefulSet status.%s=2, got %d (found=%v)", field, got, found)
		}
	}
}

func TestMarkStatefulSetHealthyPreservesExisting(t *testing.T) {
	sts := newObj("apps/v1", "StatefulSet", "ns", "db")
	_ = unstructured.SetNestedField(sts.Object, int64(2), "spec", "replicas")
	_ = unstructured.SetNestedMap(sts.Object, map[string]any{}, "status")
	markStatefulSetHealthy(sts)

	status, _, _ := unstructured.NestedMap(sts.Object, "status")
	if len(status) != 0 {
		t.Fatalf("expected explicit empty StatefulSet status to be preserved, got %v", status)
	}
}

func TestSynthesizeWorkloadsDaemonSet(t *testing.T) {
	ds := newObj("apps/v1", "DaemonSet", "ns", "agent")
	out := synthesizeWorkloads([]*unstructured.Unstructured{ds})

	dsUID := string(ds.GetUID())
	pods := childrenOf(out, dsUID)
	if len(pods) != 1 {
		t.Fatalf("expected exactly 1 Pod for DaemonSet, got %d", len(pods))
	}
	phase, _, _ := unstructured.NestedString(pods[0].Object, "status", "phase")
	if phase != "Running" {
		t.Fatalf("expected DaemonSet Pod healthy, got phase %q", phase)
	}

	// The DaemonSet itself must be stamped healthy (1/1 matching the single
	// fabricated pod) so the plugin doesn't render it 0/1.
	for _, field := range []string{"desiredNumberScheduled", "currentNumberScheduled", "numberReady", "numberAvailable", "updatedNumberScheduled"} {
		got, found, _ := unstructured.NestedInt64(ds.Object, "status", field)
		if !found || got != 1 {
			t.Fatalf("expected DaemonSet status.%s=1, got %d (found=%v)", field, got, found)
		}
	}
}

func TestMarkDaemonSetHealthyPreservesExisting(t *testing.T) {
	ds := newObj("apps/v1", "DaemonSet", "ns", "agent")
	_ = unstructured.SetNestedMap(ds.Object, map[string]any{}, "status")
	markDaemonSetHealthy(ds)

	status, _, _ := unstructured.NestedMap(ds.Object, "status")
	if len(status) != 0 {
		t.Fatalf("expected explicit empty DaemonSet status to be preserved, got %v", status)
	}
}

func TestSynthesizeWorkloadsJob(t *testing.T) {
	job := newObj("batch/v1", "Job", "ns", "migrate")
	out := synthesizeWorkloads([]*unstructured.Unstructured{job})

	jobUID := string(job.GetUID())
	pods := childrenOf(out, jobUID)
	if len(pods) != 1 {
		t.Fatalf("expected exactly 1 Pod for Job, got %d", len(pods))
	}
}

func TestSynthesizeWorkloadsCronJob(t *testing.T) {
	cj := newObj("batch/v1", "CronJob", "ns", "report")
	out := synthesizeWorkloads([]*unstructured.Unstructured{cj})

	cjUID := string(cj.GetUID())
	jobs := childrenOf(out, cjUID)
	if len(jobs) != 1 {
		t.Fatalf("expected exactly 1 Job owned by CronJob, got %d", len(jobs))
	}
	job := jobs[0]
	if job.GetKind() != "Job" || job.GetAPIVersion() != "batch/v1" {
		t.Fatalf("expected Job batch/v1, got %s/%s", job.GetAPIVersion(), job.GetKind())
	}
	if job.GetNamespace() != "ns" {
		t.Fatalf("expected Job namespace ns, got %q", job.GetNamespace())
	}

	pods := childrenOf(out, string(job.GetUID()))
	if len(pods) != 1 {
		t.Fatalf("expected exactly 1 Pod owned by the fabricated Job, got %d", len(pods))
	}
}

func TestSynthesizeWorkloadsPassesThroughOtherKinds(t *testing.T) {
	cm := newObj("v1", "ConfigMap", "ns", "cfg")
	out := synthesizeWorkloads([]*unstructured.Unstructured{cm})
	if len(out) != 1 || out[0] != cm {
		t.Fatalf("expected ConfigMap to pass through untouched, got %d objects", len(out))
	}
}

// --- synthesizeEndpoints tests ---

// newService builds a Service with the given spec.selector (nil for a
// selectorless service) and optional spec.ports.
func newService(namespace, name string, selector map[string]any, ports ...map[string]any) *unstructured.Unstructured {
	svc := newObj("v1", "Service", namespace, name)
	if selector != nil {
		_ = unstructured.SetNestedMap(svc.Object, selector, "spec", "selector")
	}
	if len(ports) > 0 {
		ps := make([]any, 0, len(ports))
		for _, p := range ports {
			ps = append(ps, p)
		}
		_ = unstructured.SetNestedSlice(svc.Object, ps, "spec", "ports")
	}
	return svc
}

// newReadyPod builds a Pod with the given labels and a status.podIP.
func newReadyPod(namespace, name, ip string, labels map[string]any) *unstructured.Unstructured {
	p := newObj("v1", "Pod", namespace, name)
	if len(labels) > 0 {
		_ = unstructured.SetNestedMap(p.Object, labels, "metadata", "labels")
	}
	_ = unstructured.SetNestedField(p.Object, ip, "status", "podIP")
	return p
}

// endpointsFor returns the Endpoints object with the given namespace/name, or nil.
func endpointsFor(objs []*unstructured.Unstructured, namespace, name string) *unstructured.Unstructured {
	for _, o := range objs {
		if o.GetKind() == "Endpoints" && o.GetAPIVersion() == "v1" &&
			o.GetNamespace() == namespace && o.GetName() == name {
			return o
		}
	}
	return nil
}

// endpointAddresses collects all subset address IPs from an Endpoints object.
func endpointAddresses(ep *unstructured.Unstructured) []string {
	var out []string
	subsets, _, _ := unstructured.NestedSlice(ep.Object, "subsets")
	for _, s := range subsets {
		sm, ok := s.(map[string]any)
		if !ok {
			continue
		}
		addrs, _, _ := unstructured.NestedSlice(sm, "addresses")
		for _, a := range addrs {
			am, ok := a.(map[string]any)
			if !ok {
				continue
			}
			ip, _, _ := unstructured.NestedString(am, "ip")
			out = append(out, ip)
		}
	}
	return out
}

func TestSynthesizeEndpointsMatchingPods(t *testing.T) {
	svc := newService("ns", "web", map[string]any{"app": "web"},
		map[string]any{"name": "http", "port": int64(80), "protocol": "TCP", "targetPort": int64(8080)})
	pod1 := newReadyPod("ns", "web-1", "10.0.0.1", map[string]any{"app": "web", "tier": "frontend"})
	pod2 := newReadyPod("ns", "web-2", "10.0.0.2", map[string]any{"app": "web"})
	other := newReadyPod("ns", "db-1", "10.0.0.9", map[string]any{"app": "db"})

	out := synthesizeEndpoints([]*unstructured.Unstructured{svc, pod1, pod2, other})

	ep := endpointsFor(out, "ns", "web")
	if ep == nil {
		t.Fatalf("expected an Endpoints object named web in ns")
	}

	// Endpoints name == service name, same namespace.
	if ep.GetName() != "web" || ep.GetNamespace() != "ns" {
		t.Fatalf("expected Endpoints ns/web, got %s/%s", ep.GetNamespace(), ep.GetName())
	}

	addrs := endpointAddresses(ep)
	want := map[string]bool{"10.0.0.1": true, "10.0.0.2": true}
	if len(addrs) != 2 {
		t.Fatalf("expected 2 endpoint addresses, got %d: %v", len(addrs), addrs)
	}
	for _, ip := range addrs {
		if !want[ip] {
			t.Fatalf("unexpected endpoint address %q (want one of %v)", ip, want)
		}
	}

	// The subset must carry the Service's named port (name/port/protocol),
	// exercising endpointPorts output.
	ports := endpointPortsOf(ep)
	if len(ports) != 1 {
		t.Fatalf("expected exactly 1 endpoint subset port, got %d: %v", len(ports), ports)
	}
	p := ports[0]
	if name, _, _ := unstructured.NestedString(p, "name"); name != "http" {
		t.Fatalf("expected endpoint port name 'http', got %q", name)
	}
	if proto, _, _ := unstructured.NestedString(p, "protocol"); proto != "TCP" {
		t.Fatalf("expected endpoint port protocol 'TCP', got %q", proto)
	}
	if num, found, _ := unstructured.NestedInt64(p, "port"); !found || num != 80 {
		t.Fatalf("expected endpoint port 80, got %d (found=%v)", num, found)
	}

	// Provenance + stable uid.
	if ep.GetAnnotations()[SourceAnnotation] != "synthesized" {
		t.Fatalf("expected Endpoints annotation %s=synthesized, got %q", SourceAnnotation, ep.GetAnnotations()[SourceAnnotation])
	}
	if string(ep.GetUID()) == "" {
		t.Fatalf("expected Endpoints to carry a stable uid")
	}
}

// endpointPortsOf collects the ports of the first subset of an Endpoints object.
func endpointPortsOf(ep *unstructured.Unstructured) []map[string]any {
	subsets, _, _ := unstructured.NestedSlice(ep.Object, "subsets")
	if len(subsets) == 0 {
		return nil
	}
	sm, ok := subsets[0].(map[string]any)
	if !ok {
		return nil
	}
	raw, _, _ := unstructured.NestedSlice(sm, "ports")
	out := make([]map[string]any, 0, len(raw))
	for _, p := range raw {
		if pm, ok := p.(map[string]any); ok {
			out = append(out, pm)
		}
	}
	return out
}

func TestMarkPodHealthyDistinctIPs(t *testing.T) {
	// Two fabricated pods with distinct identities must get distinct podIPs so
	// synthesizeEndpoints doesn't emit duplicate addresses.
	p1 := newObj("v1", "Pod", "ns", "web-1")
	p2 := newObj("v1", "Pod", "ns", "web-2")
	markPodHealthy(p1)
	markPodHealthy(p2)

	ip1, _, _ := unstructured.NestedString(p1.Object, "status", "podIP")
	ip2, _, _ := unstructured.NestedString(p2.Object, "status", "podIP")
	if ip1 == "" || ip2 == "" {
		t.Fatalf("expected both pods to get a podIP, got %q and %q", ip1, ip2)
	}
	if ip1 == ip2 {
		t.Fatalf("expected distinct podIPs for distinct pods, both got %q", ip1)
	}

	// Deterministic: same identity yields the same IP across renders.
	p1b := newObj("v1", "Pod", "ns", "web-1")
	markPodHealthy(p1b)
	if ipb, _, _ := unstructured.NestedString(p1b.Object, "status", "podIP"); ipb != ip1 {
		t.Fatalf("expected deterministic podIP for the same identity, got %q != %q", ipb, ip1)
	}
}

func TestSynthesizeEndpointsDistinctAddressesPerPod(t *testing.T) {
	// A Deployment with 3 replicas fabricates 3 pods; a matching Service must
	// yield 3 distinct endpoint addresses (no shared literal IP).
	dep := newDeployment("ns", "web", new(int64(3)))
	_ = unstructured.SetNestedMap(dep.Object, map[string]any{"app": "web"}, "spec", "template", "metadata", "labels")
	svc := newService("ns", "web", map[string]any{"app": "web"})

	objs := synthesizeWorkloads([]*unstructured.Unstructured{dep, svc})
	objs = synthesizeEndpoints(objs)

	ep := endpointsFor(objs, "ns", "web")
	if ep == nil {
		t.Fatalf("expected an Endpoints object named web in ns")
	}
	addrs := endpointAddresses(ep)
	if len(addrs) != 3 {
		t.Fatalf("expected 3 endpoint addresses (one per fabricated pod), got %d: %v", len(addrs), addrs)
	}
	seen := map[string]bool{}
	for _, ip := range addrs {
		if seen[ip] {
			t.Fatalf("duplicate endpoint address %q (expected one distinct IP per pod): %v", ip, addrs)
		}
		seen[ip] = true
	}
}

func TestSynthesizeEndpointsNoMatchingPods(t *testing.T) {
	svc := newService("ns", "web", map[string]any{"app": "web"})
	pod := newReadyPod("ns", "db-1", "10.0.0.9", map[string]any{"app": "db"})

	out := synthesizeEndpoints([]*unstructured.Unstructured{svc, pod})

	ep := endpointsFor(out, "ns", "web")
	if ep == nil {
		t.Fatalf("expected an Endpoints object even with no matching pods")
	}
	if addrs := endpointAddresses(ep); len(addrs) != 0 {
		t.Fatalf("expected no endpoint addresses for non-matching selector, got %v", addrs)
	}
}

func TestSynthesizeEndpointsSelectorless(t *testing.T) {
	// A selectorless (headless / manually-managed) Service must not get a
	// fabricated Endpoints object.
	svc := newService("ns", "external", nil)
	pod := newReadyPod("ns", "web-1", "10.0.0.1", map[string]any{"app": "web"})

	out := synthesizeEndpoints([]*unstructured.Unstructured{svc, pod})

	if ep := endpointsFor(out, "ns", "external"); ep != nil {
		t.Fatalf("expected no Endpoints fabricated for selectorless Service, got one")
	}
}

func TestSynthesizeEndpointsNamespaceScoped(t *testing.T) {
	// A pod with matching labels in a different namespace must not be included.
	svc := newService("ns", "web", map[string]any{"app": "web"})
	same := newReadyPod("ns", "web-1", "10.0.0.1", map[string]any{"app": "web"})
	otherNS := newReadyPod("other", "web-2", "10.9.9.9", map[string]any{"app": "web"})

	out := synthesizeEndpoints([]*unstructured.Unstructured{svc, same, otherNS})

	ep := endpointsFor(out, "ns", "web")
	if ep == nil {
		t.Fatalf("expected Endpoints object for web")
	}
	addrs := endpointAddresses(ep)
	if len(addrs) != 1 || addrs[0] != "10.0.0.1" {
		t.Fatalf("expected only the in-namespace pod ip, got %v", addrs)
	}
}

func TestSynthesizeEndpointsReturnsOriginals(t *testing.T) {
	svc := newService("ns", "web", map[string]any{"app": "web"})
	pod := newReadyPod("ns", "web-1", "10.0.0.1", map[string]any{"app": "web"})
	in := []*unstructured.Unstructured{svc, pod}

	out := synthesizeEndpoints(in)

	var sawSvc, sawPod bool
	for _, o := range out {
		switch o {
		case svc:
			sawSvc = true
		case pod:
			sawPod = true
		}
	}
	if !sawSvc || !sawPod {
		t.Fatalf("expected originals to be returned (svc=%v pod=%v)", sawSvc, sawPod)
	}
}

func keys(m map[string]*unstructured.Unstructured) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
