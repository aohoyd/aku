package manifest

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utiljson "k8s.io/apimachinery/pkg/util/json"
)

// ContextName is the synthetic kube-context name used for clusters built from
// static manifests. The app routes informer updates by context, so a fixed name
// keeps manifest panes distinct from any live cluster.
const ContextName = "manifests"

// kindMeta describes a built-in Kind's mapping to a GroupVersionResource and its
// scope. Group is the empty string for core/v1 kinds.
type kindMeta struct {
	Group      string
	Version    string
	Resource   string // plural
	Namespaced bool
}

// builtinKinds maps a Kind to its GVR + scope for the common built-in API
// resources. It is keyed by Kind alone: the manifest's own apiVersion supplies
// the authoritative group/version at resolve time, so this table is only
// consulted for the plural Resource and the namespaced flag.
var builtinKinds = map[string]kindMeta{
	// core/v1
	"Pod":                   {"", "v1", "pods", true},
	"Service":               {"", "v1", "services", true},
	"Endpoints":             {"", "v1", "endpoints", true},
	"ConfigMap":             {"", "v1", "configmaps", true},
	"Secret":                {"", "v1", "secrets", true},
	"ServiceAccount":        {"", "v1", "serviceaccounts", true},
	"PersistentVolumeClaim": {"", "v1", "persistentvolumeclaims", true},
	"PersistentVolume":      {"", "v1", "persistentvolumes", false},
	"Namespace":             {"", "v1", "namespaces", false},
	"Node":                  {"", "v1", "nodes", false},
	"ReplicationController": {"", "v1", "replicationcontrollers", true},
	"Event":                 {"", "v1", "events", true},
	"LimitRange":            {"", "v1", "limitranges", true},
	"ResourceQuota":         {"", "v1", "resourcequotas", true},

	// apps/v1
	"Deployment":  {"apps", "v1", "deployments", true},
	"ReplicaSet":  {"apps", "v1", "replicasets", true},
	"StatefulSet": {"apps", "v1", "statefulsets", true},
	"DaemonSet":   {"apps", "v1", "daemonsets", true},

	// batch/v1
	"Job":     {"batch", "v1", "jobs", true},
	"CronJob": {"batch", "v1", "cronjobs", true},

	// discovery.k8s.io/v1
	"EndpointSlice": {"discovery.k8s.io", "v1", "endpointslices", true},

	// networking.k8s.io/v1
	"Ingress":       {"networking.k8s.io", "v1", "ingresses", true},
	"IngressClass":  {"networking.k8s.io", "v1", "ingressclasses", false},
	"NetworkPolicy": {"networking.k8s.io", "v1", "networkpolicies", true},

	// rbac.authorization.k8s.io/v1
	"Role":               {"rbac.authorization.k8s.io", "v1", "roles", true},
	"RoleBinding":        {"rbac.authorization.k8s.io", "v1", "rolebindings", true},
	"ClusterRole":        {"rbac.authorization.k8s.io", "v1", "clusterroles", false},
	"ClusterRoleBinding": {"rbac.authorization.k8s.io", "v1", "clusterrolebindings", false},

	// autoscaling
	"HorizontalPodAutoscaler": {"autoscaling", "v2", "horizontalpodautoscalers", true},

	// policy/v1
	"PodDisruptionBudget": {"policy", "v1", "poddisruptionbudgets", true},

	// apiextensions.k8s.io/v1
	"CustomResourceDefinition": {"apiextensions.k8s.io", "v1", "customresourcedefinitions", false},

	// storage.k8s.io/v1
	"StorageClass": {"storage.k8s.io", "v1", "storageclasses", false},

	// scheduling.k8s.io/v1
	"PriorityClass": {"scheduling.k8s.io", "v1", "priorityclasses", false},

	// admissionregistration.k8s.io/v1
	"ValidatingWebhookConfiguration": {"admissionregistration.k8s.io", "v1", "validatingwebhookconfigurations", false},
	"MutatingWebhookConfiguration":   {"admissionregistration.k8s.io", "v1", "mutatingwebhookconfigurations", false},

	// apiregistration.k8s.io/v1
	"APIService": {"apiregistration.k8s.io", "v1", "apiservices", false},

	// node.k8s.io/v1
	"RuntimeClass": {"node.k8s.io", "v1", "runtimeclasses", false},

	// storage.k8s.io/v1 (cluster-scoped)
	"VolumeAttachment": {"storage.k8s.io", "v1", "volumeattachments", false},
	"CSIDriver":        {"storage.k8s.io", "v1", "csidrivers", false},
	"CSINode":          {"storage.k8s.io", "v1", "csinodes", false},
}

// Load builds a static cluster.Cluster from a single manifest stream. The
// pipeline is Parse → assignNamespaces → synthesizeWorkloads →
// synthesizeEndpointSlices; the resulting objects are upserted into a client-less
// Store (dual-keyed under their namespace and the all-namespaces "" bucket) and
// the per-kind GVRs are seeded into a Discovery index. The returned Cluster
// reports Connected() == false because it carries no client.
//
// Warnings from Parse are propagated, and any Kind whose plural had to be
// guessed adds its own warning.
//
// The returned Cluster is always named ContextName ("manifests"): there is
// exactly one pinned manifest pseudo-context by design, so the name is not
// parameterized.
func Load(r io.Reader, defaultNS string) (*cluster.Cluster, []Warning, error) {
	objs, warns, err := Parse(r)
	if err != nil {
		return nil, warns, err
	}

	// Numbers from the YAML decoder land as float64 (it round-trips through
	// encoding/json into map[string]any). Real cluster objects use int64, and the
	// synthesis step reads integer fields like spec.replicas via NestedInt64,
	// which rejects float64. Re-decode each object with the K8s JSON decoder so
	// integers become int64 — matching genuine unstructured objects.
	for _, o := range objs {
		normalizeNumbers(o)
	}

	objs = assignNamespaces(objs, defaultNS)
	objs = synthesizeWorkloads(objs)
	objs = synthesizeEndpointSlices(objs)

	store := k8s.NewStore(nil, ContextName, nil)
	// The manifest store is static and client-less: its cache is the only copy of
	// the synthesized objects, so teardown (Unsubscribe/UnsubscribeAll) must never
	// clear it — otherwise switching namespaces away and back empties the view.
	store.MarkStatic()
	disc := k8s.NewDiscovery()

	// Collect one APIResource per distinct GVR seen so Discovery can resolve
	// apiVersion+Kind back to the bucket each object was upserted under.
	seen := map[schema.GroupVersionResource]bool{}
	var resources []k8s.APIResource

	for _, o := range objs {
		gvr, namespaced, guessed := resolveGVR(o)
		if guessed {
			warns = append(warns, Warning{
				Reason: fmt.Sprintf("guessed plural %q for unknown kind %q (apiVersion %q)",
					gvr.Resource, o.GetKind(), o.GetAPIVersion()),
			})
		}

		ns := o.GetNamespace()
		store.CacheUpsert(gvr, ns, o)
		// Dual-key namespaced objects into the all-namespaces bucket too, so the
		// "All Namespaces" view sees them. Cluster-scoped objects already have an
		// empty namespace, so this would be a redundant re-upsert; skip it.
		if ns != "" {
			store.CacheUpsert(gvr, "", o)
		}

		if !seen[gvr] {
			seen[gvr] = true
			resources = append(resources, apiResourceFor(o, gvr, namespaced))
		}
	}

	disc.Populate(resources)

	cl := cluster.New(ContextName, "", nil, store, disc, nil)
	return cl, warns, nil
}

// LoadFiles builds a static cluster.Cluster from the concatenation of several
// manifest files. Each path is expanded: a directory is walked recursively for
// *.yaml/*.yml files (other files are ignored silently), and a regular file is
// read as-is. A path that cannot be read is recorded as a Warning and skipped
// rather than failing the whole load. The combined object set runs through the
// same synthesis pipeline as Load.
func LoadFiles(paths []string, defaultNS string) (*cluster.Cluster, []Warning, error) {
	var sb strings.Builder
	var warns []Warning

	files, expandWarns := expandPaths(paths)
	warns = append(warns, expandWarns...)

	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			warns = append(warns, Warning{Reason: fmt.Sprintf("read %s: %v", p, err)})
			continue
		}
		sb.Write(data)
		// Ensure a document boundary between files so the last document of one
		// file never merges with the first of the next.
		if len(data) > 0 && data[len(data)-1] != '\n' {
			sb.WriteByte('\n')
		}
		sb.WriteString("---\n")
	}

	cl, loadWarns, err := Load(strings.NewReader(sb.String()), defaultNS)
	warns = append(warns, loadWarns...)
	return cl, warns, err
}

// isYAMLFile reports whether name has a .yaml or .yml extension (case-insensitive).
func isYAMLFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

// expandPaths resolves each input path to a stable, sorted list of files to
// read. A directory is walked recursively, collecting only *.yaml/*.yml files
// (other files are skipped silently). A regular file is included verbatim
// regardless of extension, matching the documented "-f file" behavior. A path
// that cannot be stat'd or walked is recorded as a Warning and skipped.
func expandPaths(paths []string) ([]string, []Warning) {
	var files []string
	var warns []Warning

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			warns = append(warns, Warning{Reason: fmt.Sprintf("stat %s: %v", p, err)})
			continue
		}
		if !info.IsDir() {
			files = append(files, p)
			continue
		}

		var dirFiles []string
		walkErr := filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				warns = append(warns, Warning{Reason: fmt.Sprintf("walk %s: %v", path, err)})
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if isYAMLFile(d.Name()) {
				dirFiles = append(dirFiles, path)
			}
			return nil
		})
		if walkErr != nil {
			warns = append(warns, Warning{Reason: fmt.Sprintf("walk %s: %v", p, walkErr)})
		}
		// Sort the per-directory files for deterministic ordering. WalkDir already
		// walks lexically, but sorting keeps the guarantee explicit and robust.
		slices.Sort(dirFiles)
		files = append(files, dirFiles...)
	}

	return files, warns
}

// resolveGVR derives an object's GroupVersionResource and namespaced flag. The
// group/version always come from the object's own apiVersion; the plural
// Resource comes from the builtin table when the Kind is known, otherwise it is
// guessed (and guessed is reported true). The namespaced flag prefers the
// builtin table, falling back to the cluster-scoped-kinds heuristic from
// synth.go.
func resolveGVR(o *unstructured.Unstructured) (gvr schema.GroupVersionResource, namespaced, guessed bool) {
	kind := o.GetKind()
	gv, err := schema.ParseGroupVersion(o.GetAPIVersion())
	if err != nil {
		gv = schema.GroupVersion{Version: "v1"}
	}

	if meta, ok := builtinKinds[kind]; ok {
		return schema.GroupVersionResource{
			Group:    gv.Group,
			Version:  gv.Version,
			Resource: meta.Resource,
		}, meta.Namespaced, false
	}

	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: guessPlural(kind),
	}, !isClusterScoped(kind), true
}

// apiResourceFor builds the Discovery APIResource record for an object's GVR.
func apiResourceFor(o *unstructured.Unstructured, gvr schema.GroupVersionResource, namespaced bool) k8s.APIResource {
	apiVersion := gvr.Version
	if gvr.Group != "" {
		apiVersion = gvr.Group + "/" + gvr.Version
	}
	return k8s.APIResource{
		Name:       gvr.Resource,
		APIVersion: apiVersion,
		Group:      gvr.Group,
		Version:    gvr.Version,
		Kind:       o.GetKind(),
		Namespaced: namespaced,
		GVR:        gvr,
	}
}

// normalizeNumbers rewrites an object's numeric fields from float64 (as produced
// by the YAML decoder via encoding/json) to the int64/JSON shapes the K8s
// unstructured accessors expect. It round-trips the object through the K8s JSON
// codec, which decodes whole numbers as int64. On any marshalling error the
// object is left as-is.
func normalizeNumbers(o *unstructured.Unstructured) {
	jsonBytes, err := json.Marshal(o.Object)
	if err != nil {
		return
	}
	var m map[string]any
	if err := utiljson.Unmarshal(jsonBytes, &m); err != nil {
		return
	}
	o.Object = m
}

// guessPlural derives a best-effort lowercase plural from a Kind for resources
// not in the builtin table. It handles the simple English suffix rules that
// cover most Kubernetes kinds: a trailing "y" becomes "ies", a trailing "s"
// becomes "ses", everything else just gains an "s".
func guessPlural(kind string) string {
	k := strings.ToLower(kind)
	switch {
	case strings.HasSuffix(k, "y"):
		return strings.TrimSuffix(k, "y") + "ies"
	case strings.HasSuffix(k, "s"):
		return k + "es"
	default:
		return k + "s"
	}
}
