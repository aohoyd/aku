package manifest

import (
	"crypto/sha1"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// uidNamespace is a fixed namespace OID used as the seed for SHA1-based UUIDs so
// that stableUID is deterministic across processes. It is an arbitrary,
// project-specific constant (not one of the RFC 4122 well-known namespaces).
var uidNamespace = uuid.MustParse("a1b2c3d4-0a0b-0c0d-0e0f-0a0b0c0d0e0f")

// stableUID returns a deterministic, UUID-shaped string derived from a
// resource's kind, namespace, and name. The same inputs always yield the same
// UID, and distinct inputs yield distinct UIDs — letting fabricated objects
// carry stable metadata.uid values across repeated manifest renders.
func stableUID(kind, namespace, name string) string {
	return uuid.NewSHA1(uidNamespace, []byte(kind+"/"+namespace+"/"+name)).String()
}

// podIP derives a deterministic, distinct, RFC 1918 pod IP from a Pod's
// identity (kind/namespace/name). Hashing the identity means two fabricated
// pods get different IPs and a re-render yields the same IP for a given pod, so
// synthesizeEndpoints emits one distinct address per matching pod rather than a
// shared literal. The address lives in 10.x.y.z; the low octets avoid 0/255 so
// the result is always a usable host address.
func podIP(pod *unstructured.Unstructured) string {
	sum := sha1.Sum([]byte("Pod/" + pod.GetNamespace() + "/" + pod.GetName()))
	b := 1 + int(sum[0])%254 // 1..254
	c := 1 + int(sum[1])%254 // 1..254
	d := 1 + int(sum[2])%254 // 1..254
	return fmt.Sprintf("10.%d.%d.%d", b, c, d)
}

// markPodHealthy stamps a healthy runtime status onto a fabricated Pod: a
// Running phase, a ready/started/running containerStatus per spec container, a
// synthetic podIP, and a startTime. If the Pod already carries a status.phase it
// is left untouched, so a user-supplied status is never clobbered.
func markPodHealthy(pod *unstructured.Unstructured) {
	if phase, _, _ := unstructured.NestedString(pod.Object, "status", "phase"); phase != "" {
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	containers, _, _ := unstructured.NestedSlice(pod.Object, "spec", "containers")
	statuses := make([]any, 0, len(containers))
	for _, c := range containers {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(cm, "name")
		image, _, _ := unstructured.NestedString(cm, "image")
		statuses = append(statuses, map[string]any{
			"name":         name,
			"image":        image,
			"ready":        true,
			"started":      true,
			"restartCount": int64(0),
			"state": map[string]any{
				"running": map[string]any{
					"startedAt": now,
				},
			},
		})
	}

	_ = unstructured.SetNestedField(pod.Object, "Running", "status", "phase")
	_ = unstructured.SetNestedField(pod.Object, podIP(pod), "status", "podIP")
	_ = unstructured.SetNestedField(pod.Object, now, "status", "startTime")
	if len(statuses) > 0 {
		_ = unstructured.SetNestedSlice(pod.Object, statuses, "status", "containerStatuses")
	}
}

// markDeploymentHealthy sets every replica-count status field
// (replicas/readyReplicas/availableReplicas/updatedReplicas) to spec.replicas,
// defaulting to 1 when spec.replicas is unset. If the Deployment already carries
// a status field at all — including an explicit empty `status: {}` — it is left
// untouched, so a user-supplied status is never clobbered.
func markDeploymentHealthy(dep *unstructured.Unstructured) {
	if _, found, _ := unstructured.NestedMap(dep.Object, "status"); found {
		return
	}

	replicas := int64(1)
	if r, found, _ := unstructured.NestedInt64(dep.Object, "spec", "replicas"); found {
		replicas = r
	}

	for _, field := range []string{"replicas", "readyReplicas", "availableReplicas", "updatedReplicas"} {
		_ = unstructured.SetNestedField(dep.Object, replicas, "status", field)
	}
}

// markStatefulSetHealthy sets every replica-count status field
// (replicas/readyReplicas/currentReplicas/updatedReplicas) to spec.replicas so
// the StatefulSet plugin renders N/N ready rather than 0/N. Pre-existing status
// (including an explicit empty `status: {}`) is preserved.
func markStatefulSetHealthy(sts *unstructured.Unstructured) {
	if _, found, _ := unstructured.NestedMap(sts.Object, "status"); found {
		return
	}
	replicas := replicaCount(sts)
	for _, field := range []string{"replicas", "readyReplicas", "currentReplicas", "updatedReplicas"} {
		_ = unstructured.SetNestedField(sts.Object, replicas, "status", field)
	}
}

// markDaemonSetHealthy stamps a healthy status matching the single fabricated
// Pod: desired/current/ready/available/updated all set to 1. Pre-existing status
// (including an explicit empty `status: {}`) is preserved.
func markDaemonSetHealthy(ds *unstructured.Unstructured) {
	if _, found, _ := unstructured.NestedMap(ds.Object, "status"); found {
		return
	}
	for _, field := range []string{
		"desiredNumberScheduled",
		"currentNumberScheduled",
		"numberReady",
		"numberAvailable",
		"updatedNumberScheduled",
	} {
		_ = unstructured.SetNestedField(ds.Object, int64(1), "status", field)
	}
}

// clusterScopedKinds is a static set of well-known cluster-scoped Kinds.
//
// Heuristic: we have no live discovery to ask whether a Kind is namespaced, so
// we treat this curated set of built-in cluster-scoped Kinds as cluster-scoped
// and default every other Kind to namespaced. Defaulting unknowns to namespaced
// is the safe fallback for manifest preview: the vast majority of resources
// (and effectively all CRDs people render with helm/kustomize) are namespaced,
// and a namespaced object missing a namespace simply gets the default stamped —
// whereas wrongly treating a namespaced object as cluster-scoped would lose its
// namespace entirely. Later tasks may share Kind metadata via the loader's
// Kind→GVR map; keep this a plain set rather than over-engineering a registry.
var clusterScopedKinds = map[string]bool{
	"Namespace":                      true,
	"Node":                           true,
	"PersistentVolume":               true,
	"ClusterRole":                    true,
	"ClusterRoleBinding":             true,
	"CustomResourceDefinition":       true,
	"StorageClass":                   true,
	"PriorityClass":                  true,
	"ValidatingWebhookConfiguration": true,
	"MutatingWebhookConfiguration":   true,
	"APIService":                     true,
	"IngressClass":                   true,
	"RuntimeClass":                   true,
	"VolumeAttachment":               true,
	"CSIDriver":                      true,
	"CSINode":                        true,
	"PodSecurityPolicy":              true,
	"ClusterIssuer":                  true,
}

// isClusterScoped reports whether a Kind is cluster-scoped per the static set
// above. Unknown Kinds default to namespaced.
func isClusterScoped(kind string) bool {
	return clusterScopedKinds[kind]
}

// assignNamespaces stamps defaultNS onto namespaced objects that lack a
// namespace, then fabricates a Namespace object (status.phase=Active) for every
// distinct namespace actually referenced by namespaced objects that does not
// already have a Namespace object among the inputs.
//
// The returned slice contains the input objects (now possibly namespace-stamped)
// followed by any fabricated Namespace objects. Cluster-scoped objects are left
// untouched and never trigger Namespace fabrication.
func assignNamespaces(objs []*unstructured.Unstructured, defaultNS string) []*unstructured.Unstructured {
	// Namespaces that already have a Namespace object in the input.
	existing := map[string]bool{}
	for _, o := range objs {
		if o.GetKind() == "Namespace" && o.GetAPIVersion() == "v1" {
			existing[o.GetName()] = true
		}
	}

	used := map[string]bool{}
	for _, o := range objs {
		if isClusterScoped(o.GetKind()) {
			continue
		}
		ns := o.GetNamespace()
		if ns == "" {
			ns = defaultNS
			o.SetNamespace(ns)
		}
		if ns != "" {
			used[ns] = true
		}
	}

	out := objs
	for ns := range used {
		if existing[ns] {
			continue
		}
		out = append(out, fabricateNamespace(ns))
	}
	return out
}

// synthesizeWorkloads returns the input objects plus fabricated runtime children
// for each recognised workload controller, so aku's owner-reference-based
// drill-down works on a static manifest. Deployments yield a ReplicaSet and its
// Pods; StatefulSets/DaemonSets/Jobs yield Pods directly; CronJobs yield a Job
// which in turn yields a Pod. Every controller is given a stable metadata.uid (if
// it lacks one) so children can reference it, and every fabricated object is
// tagged with SourceAnnotation="synthesized". Unrecognised kinds pass through
// untouched.
func synthesizeWorkloads(objs []*unstructured.Unstructured) []*unstructured.Unstructured {
	out := objs
	for _, o := range objs {
		switch o.GetKind() {
		case "Deployment":
			out = append(out, synthDeployment(o)...)
		case "StatefulSet":
			out = append(out, synthStatefulSet(o)...)
		case "DaemonSet":
			out = append(out, synthDaemonSet(o)...)
		case "Job":
			out = append(out, synthJobPods(o)...)
		case "CronJob":
			out = append(out, synthCronJob(o)...)
		}
	}
	return out
}

// ensureUID stamps a deterministic stableUID onto an object if it has none, and
// returns the resulting uid.
func ensureUID(o *unstructured.Unstructured) string {
	if uid := string(o.GetUID()); uid != "" {
		return uid
	}
	uid := stableUID(o.GetKind(), o.GetNamespace(), o.GetName())
	o.SetUID(types.UID(uid))
	return uid
}

// replicaCount reads spec.replicas, defaulting to 1 when unset. Zero is honoured.
func replicaCount(o *unstructured.Unstructured) int64 {
	if r, found, _ := unstructured.NestedInt64(o.Object, "spec", "replicas"); found {
		return r
	}
	return 1
}

// podTemplate returns spec.template.spec (the pod spec) and
// spec.template.metadata.labels for a controller object.
func podTemplate(o *unstructured.Unstructured) (spec map[string]any, labels map[string]any) {
	spec, _, _ = unstructured.NestedMap(o.Object, "spec", "template", "spec")
	labels, _, _ = unstructured.NestedMap(o.Object, "spec", "template", "metadata", "labels")
	return spec, labels
}

// newChild builds a fabricated child object of the given kind/apiVersion in the
// owner's namespace, with a single controller ownerReference pointing at the
// owner and a stable uid. It is stamped with the "synthesized" provenance
// annotation.
func newChild(kind, apiVersion, namespace, name, ownerUID, ownerKind, ownerName, ownerAPIVersion string) *unstructured.Unstructured {
	c := &unstructured.Unstructured{Object: map[string]any{}}
	c.SetAPIVersion(apiVersion)
	c.SetKind(kind)
	if namespace != "" {
		c.SetNamespace(namespace)
	}
	c.SetName(name)
	c.SetUID(types.UID(stableUID(kind, namespace, name)))
	controller := true
	c.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: ownerAPIVersion,
		Kind:       ownerKind,
		Name:       ownerName,
		UID:        types.UID(ownerUID),
		Controller: &controller,
	}})
	c.SetAnnotations(map[string]string{SourceAnnotation: "synthesized"})
	return c
}

// makePod fabricates a healthy Pod owned by owner, copying the controller's pod
// template spec and labels.
func makePod(ns, name, ownerUID, ownerKind, ownerName, ownerAPIVersion string, podSpec, labels map[string]any) *unstructured.Unstructured {
	p := newChild("Pod", "v1", ns, name, ownerUID, ownerKind, ownerName, ownerAPIVersion)
	if podSpec != nil {
		_ = unstructured.SetNestedMap(p.Object, podSpec, "spec")
	}
	if len(labels) > 0 {
		_ = unstructured.SetNestedMap(p.Object, labels, "metadata", "labels")
	}
	markPodHealthy(p)
	return p
}

// shortHash derives a short, deterministic suffix from a uid for naming
// ReplicaSets and their pods. UID strings are ASCII (UUID-shaped), so stripping
// hyphens and slicing the first 8 bytes is safe.
func shortHash(uid string) string {
	clean := strings.ReplaceAll(uid, "-", "")
	if len(clean) > 8 {
		clean = clean[:8]
	}
	return clean
}

func synthDeployment(dep *unstructured.Unstructured) []*unstructured.Unstructured {
	depUID := ensureUID(dep)
	markDeploymentHealthy(dep)

	ns := dep.GetNamespace()
	replicas := replicaCount(dep)
	podSpec, labels := podTemplate(dep)

	rsName := fmt.Sprintf("%s-%s", dep.GetName(), shortHash(depUID))
	rs := newChild("ReplicaSet", "apps/v1", ns, rsName, depUID, "Deployment", dep.GetName(), dep.GetAPIVersion())
	_ = unstructured.SetNestedField(rs.Object, replicas, "spec", "replicas")
	rsUID := string(rs.GetUID())

	out := []*unstructured.Unstructured{rs}
	for i := int64(0); i < replicas; i++ {
		podName := fmt.Sprintf("%s-%s", rsName, shortHash(stableUID("Pod", ns, fmt.Sprintf("%s-%d", rsName, i))))
		out = append(out, makePod(ns, podName, rsUID, "ReplicaSet", rsName, "apps/v1", podSpec, labels))
	}
	return out
}

func synthStatefulSet(sts *unstructured.Unstructured) []*unstructured.Unstructured {
	stsUID := ensureUID(sts)
	markStatefulSetHealthy(sts)
	ns := sts.GetNamespace()
	replicas := replicaCount(sts)
	podSpec, labels := podTemplate(sts)

	out := []*unstructured.Unstructured{}
	for i := int64(0); i < replicas; i++ {
		podName := fmt.Sprintf("%s-%d", sts.GetName(), i)
		out = append(out, makePod(ns, podName, stsUID, "StatefulSet", sts.GetName(), sts.GetAPIVersion(), podSpec, labels))
	}
	return out
}

func synthDaemonSet(ds *unstructured.Unstructured) []*unstructured.Unstructured {
	// No node list is available in a static manifest, so we fabricate a single
	// representative Pod.
	dsUID := ensureUID(ds)
	markDaemonSetHealthy(ds)
	ns := ds.GetNamespace()
	podSpec, labels := podTemplate(ds)
	podName := fmt.Sprintf("%s-%s", ds.GetName(), shortHash(dsUID))
	return []*unstructured.Unstructured{
		makePod(ns, podName, dsUID, "DaemonSet", ds.GetName(), ds.GetAPIVersion(), podSpec, labels),
	}
}

// synthJobPods fabricates a single Pod for a Job (whether top-level or
// CronJob-owned).
func synthJobPods(job *unstructured.Unstructured) []*unstructured.Unstructured {
	jobUID := ensureUID(job)
	ns := job.GetNamespace()
	podSpec, labels := podTemplate(job)
	podName := fmt.Sprintf("%s-%s", job.GetName(), shortHash(jobUID))
	return []*unstructured.Unstructured{
		makePod(ns, podName, jobUID, "Job", job.GetName(), job.GetAPIVersion(), podSpec, labels),
	}
}

func synthCronJob(cj *unstructured.Unstructured) []*unstructured.Unstructured {
	cjUID := ensureUID(cj)
	ns := cj.GetNamespace()

	jobName := fmt.Sprintf("%s-%s", cj.GetName(), shortHash(cjUID))
	job := newChild("Job", "batch/v1", ns, jobName, cjUID, "CronJob", cj.GetName(), cj.GetAPIVersion())
	// Carry the jobTemplate's pod template onto the fabricated Job so its Pod can
	// inherit the spec/labels.
	if tmpl, found, _ := unstructured.NestedMap(cj.Object, "spec", "jobTemplate", "spec", "template"); found {
		_ = unstructured.SetNestedMap(job.Object, tmpl, "spec", "template")
	}

	out := []*unstructured.Unstructured{job}
	out = append(out, synthJobPods(job)...)
	return out
}

// synthesizeEndpoints returns the input objects plus a fabricated core/v1
// Endpoints object for every Service that has a non-empty spec.selector, so the
// Endpoints resource list isn't empty in the simulated cluster. The Endpoints
// share the Service's name+namespace and address every Pod in the same namespace
// whose labels are a superset of the selector (reading each Pod's
// status.podIP). Selectorless / headless Services — whose endpoints are normally
// managed manually — are skipped. Each fabricated Endpoints carries a stable
// metadata.uid and SourceAnnotation="synthesized".
//
// This is intended to run after synthesizeWorkloads so fabricated Pods (with
// IPs) already exist in the slice.
func synthesizeEndpoints(objs []*unstructured.Unstructured) []*unstructured.Unstructured {
	// Index pods by namespace, capturing labels + podIP.
	type podInfo struct {
		labels map[string]string
		ip     string
	}
	podsByNS := map[string][]podInfo{}
	for _, o := range objs {
		if o.GetKind() != "Pod" || o.GetAPIVersion() != "v1" {
			continue
		}
		ip, _, _ := unstructured.NestedString(o.Object, "status", "podIP")
		podsByNS[o.GetNamespace()] = append(podsByNS[o.GetNamespace()], podInfo{
			labels: o.GetLabels(),
			ip:     ip,
		})
	}

	out := objs
	for _, o := range objs {
		if o.GetKind() != "Service" || o.GetAPIVersion() != "v1" {
			continue
		}
		selector, found, _ := unstructured.NestedStringMap(o.Object, "spec", "selector")
		if !found || len(selector) == 0 {
			// Selectorless / headless Service: endpoints are managed manually.
			continue
		}

		var addresses []any
		for _, p := range podsByNS[o.GetNamespace()] {
			if p.ip == "" || !labelsMatch(p.labels, selector) {
				continue
			}
			addresses = append(addresses, map[string]any{"ip": p.ip})
		}

		out = append(out, fabricateEndpoints(o, addresses))
	}
	return out
}

// labelsMatch reports whether labels is a superset of selector (every selector
// key/value is present in labels).
func labelsMatch(labels, selector map[string]string) bool {
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// fabricateEndpoints builds a core/v1 Endpoints object for svc, addressing the
// given addresses (each a {"ip": ...} map) plus ports derived from svc
// spec.ports. With no addresses, subsets is left empty.
func fabricateEndpoints(svc *unstructured.Unstructured, addresses []any) *unstructured.Unstructured {
	ns := svc.GetNamespace()
	name := svc.GetName()

	ep := &unstructured.Unstructured{Object: map[string]any{}}
	ep.SetAPIVersion("v1")
	ep.SetKind("Endpoints")
	if ns != "" {
		ep.SetNamespace(ns)
	}
	ep.SetName(name)
	ep.SetUID(types.UID(stableUID("Endpoints", ns, name)))
	ep.SetAnnotations(map[string]string{SourceAnnotation: "synthesized"})

	if len(addresses) > 0 {
		subset := map[string]any{"addresses": addresses}
		if ports := endpointPorts(svc); len(ports) > 0 {
			subset["ports"] = ports
		}
		_ = unstructured.SetNestedSlice(ep.Object, []any{subset}, "subsets")
	}
	return ep
}

// endpointPorts derives an Endpoints subset ports section from a Service's
// spec.ports, carrying over name/protocol/port (using the port number as-is;
// targetPort resolution is out of scope for a static preview).
func endpointPorts(svc *unstructured.Unstructured) []any {
	svcPorts, _, _ := unstructured.NestedSlice(svc.Object, "spec", "ports")
	var out []any
	for _, p := range svcPorts {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		port := map[string]any{}
		if name, _, _ := unstructured.NestedString(pm, "name"); name != "" {
			port["name"] = name
		}
		if proto, _, _ := unstructured.NestedString(pm, "protocol"); proto != "" {
			port["protocol"] = proto
		}
		if n, found, _ := unstructured.NestedInt64(pm, "port"); found {
			port["port"] = n
		}
		out = append(out, port)
	}
	return out
}

// fabricateNamespace builds an Active Namespace object for the given name.
func fabricateNamespace(name string) *unstructured.Unstructured {
	ns := &unstructured.Unstructured{Object: map[string]any{}}
	ns.SetAPIVersion("v1")
	ns.SetKind("Namespace")
	ns.SetName(name)
	ns.SetUID(types.UID(stableUID("Namespace", "", name)))
	ns.SetAnnotations(map[string]string{SourceAnnotation: "synthesized"})
	_ = unstructured.SetNestedField(ns.Object, "Active", "status", "phase")
	return ns
}
