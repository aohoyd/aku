package cmd

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mockPlugin implements plugin.ResourcePlugin for testing.
type mockPlugin struct {
	name      string
	shortName string
	gvr       schema.GroupVersionResource
}

func (m *mockPlugin) Name() string                              { return m.name }
func (m *mockPlugin) ShortName() string                         { return m.shortName }
func (m *mockPlugin) GVR() schema.GroupVersionResource          { return m.gvr }
func (m *mockPlugin) IsClusterScoped() bool                     { return false }
func (m *mockPlugin) Columns() []plugin.Column                  { return nil }
func (m *mockPlugin) Row(_ *unstructured.Unstructured) []string { return nil }
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func registerTestPlugins() {
	plugin.Reset()
	plugin.Register(&mockPlugin{
		name:      "pods",
		shortName: "po",
		gvr:       schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
	})
	plugin.Register(&mockPlugin{
		name:      "deployments",
		shortName: "deploy",
		gvr:       schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	})
	plugin.Register(&mockPlugin{
		name:      "secrets",
		shortName: "sec",
		gvr:       schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"},
	})
	plugin.Register(&mockPlugin{
		name:      "services",
		shortName: "svc",
		gvr:       schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"},
	})
}

func TestParseResourceSpecs_CommaSplitting(t *testing.T) {
	registerTestPlugins()

	specs, err := parseResourceSpecs([]string{"po", "deploy"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].Plugin.Name() != "pods" {
		t.Errorf("expected first spec plugin name 'pods', got %q", specs[0].Plugin.Name())
	}
	if specs[1].Plugin.Name() != "deployments" {
		t.Errorf("expected second spec plugin name 'deployments', got %q", specs[1].Plugin.Name())
	}
}

func TestParseResourceSpecs_NamespacePrefix(t *testing.T) {
	registerTestPlugins()

	specs, err := parseResourceSpecs([]string{"kube-system/sec"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Namespace != "kube-system" {
		t.Errorf("expected namespace 'kube-system', got %q", specs[0].Namespace)
	}
	if specs[0].Plugin.Name() != "secrets" {
		t.Errorf("expected plugin name 'secrets', got %q", specs[0].Plugin.Name())
	}
}

func TestParseResourceSpecs_InvalidName(t *testing.T) {
	registerTestPlugins()

	_, err := parseResourceSpecs([]string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for unknown resource, got nil")
	}
	expected := `unknown resource "nonexistent"`
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestParseResourceSpecs_EmptyList(t *testing.T) {
	registerTestPlugins()

	specs, err := parseResourceSpecs(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if specs != nil {
		t.Errorf("expected nil specs, got %v", specs)
	}

	specs, err = parseResourceSpecs([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if specs != nil {
		t.Errorf("expected nil specs for empty slice, got %v", specs)
	}
}

// writeTempKubeconfig creates a temporary kubeconfig file with the given context names
// and returns the path. The caller should defer os.Remove on the returned path.
func writeTempKubeconfig(t *testing.T, contexts []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")

	var contextEntries, contextRefs string
	for _, name := range contexts {
		contextEntries += "- context:\n    cluster: test-cluster\n    user: test-user\n  name: " + name + "\n"
		if contextRefs == "" {
			contextRefs = name // use first as current-context
		}
	}

	content := `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test-cluster
contexts:
` + contextEntries + `current-context: ` + contextRefs + `
users:
- name: test-user
  user:
    token: fake-token
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write temp kubeconfig: %v", err)
	}
	return path
}

func TestCompleteContexts(t *testing.T) {
	kubeconfig := writeTempKubeconfig(t, []string{"dev", "staging", "production"})

	names, directive := completeContexts(kubeconfig)
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	sort.Strings(names)
	expected := []string{"dev", "production", "staging"}
	if len(names) != len(expected) {
		t.Fatalf("expected %d contexts, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("expected context %q at index %d, got %q", expected[i], i, name)
		}
	}
}

func TestCompleteContexts_InvalidKubeconfig(t *testing.T) {
	names, directive := completeContexts("/nonexistent/path/kubeconfig")
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(names) != 0 {
		t.Errorf("expected empty names for invalid kubeconfig, got %v", names)
	}
}

func TestCompleteContexts_EmptyKubeconfig(t *testing.T) {
	kubeconfig := writeTempKubeconfig(t, nil)

	names, directive := completeContexts(kubeconfig)
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 contexts, got %d: %v", len(names), names)
	}
}

func TestCompleteResources(t *testing.T) {
	registerTestPlugins()

	completions, directive := completeResources()
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}

	// Each plugin should produce a short-name completion (with description) and a full-name completion
	// when short name != full name.
	expectedShort := map[string]bool{
		"po\tpods":            true,
		"deploy\tdeployments": true,
		"sec\tsecrets":        true,
		"svc\tservices":       true,
	}
	expectedFull := map[string]bool{
		"pods":        true,
		"deployments": true,
		"secrets":     true,
		"services":    true,
	}

	for _, c := range completions {
		if expectedShort[c] {
			delete(expectedShort, c)
		} else if expectedFull[c] {
			delete(expectedFull, c)
		}
	}
	if len(expectedShort) != 0 {
		t.Errorf("missing short-name completions: %v", expectedShort)
	}
	if len(expectedFull) != 0 {
		t.Errorf("missing full-name completions: %v", expectedFull)
	}
}

func TestCompleteResources_EmptyRegistry(t *testing.T) {
	plugin.Reset()

	completions, directive := completeResources()
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}
	if len(completions) != 0 {
		t.Errorf("expected 0 completions for empty registry, got %d: %v", len(completions), completions)
	}
}

func TestParseResourceSpecs_QualifiedName(t *testing.T) {
	plugin.Reset()
	// Register two plugins with the same name but different groups.
	plugin.Register(&mockPlugin{
		name:      "certificates",
		shortName: "cert",
		gvr:       schema.GroupVersionResource{Group: "certificates.k8s.io", Version: "v1", Resource: "certificates"},
	})
	plugin.Register(&mockPlugin{
		name:      "certificates",
		shortName: "cert",
		gvr:       schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	})

	// Use qualified name to select the cert-manager.io variant.
	specs, err := parseResourceSpecs([]string{"certificates.cert-manager.io/v1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	got := specs[0].Plugin.GVR()
	if got.Group != "cert-manager.io" {
		t.Errorf("expected group 'cert-manager.io', got %q", got.Group)
	}
	if got.Version != "v1" {
		t.Errorf("expected version 'v1', got %q", got.Version)
	}
	if got.Resource != "certificates" {
		t.Errorf("expected resource 'certificates', got %q", got.Resource)
	}
	if specs[0].Namespace != "" {
		t.Errorf("expected empty namespace, got %q", specs[0].Namespace)
	}
}

func TestParseResourceSpecs_BareNameBackwardCompat(t *testing.T) {
	registerTestPlugins()

	// Bare name still works.
	specs, err := parseResourceSpecs([]string{"pods"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Plugin.Name() != "pods" {
		t.Errorf("expected plugin name 'pods', got %q", specs[0].Plugin.Name())
	}

	// Short name still works.
	specs, err = parseResourceSpecs([]string{"deploy"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Plugin.Name() != "deployments" {
		t.Errorf("expected plugin name 'deployments', got %q", specs[0].Plugin.Name())
	}

	// Namespace/name still works.
	specs, err = parseResourceSpecs([]string{"kube-system/pods"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].Namespace != "kube-system" {
		t.Errorf("expected namespace 'kube-system', got %q", specs[0].Namespace)
	}
	if specs[0].Plugin.Name() != "pods" {
		t.Errorf("expected plugin name 'pods', got %q", specs[0].Plugin.Name())
	}
}

func TestCompleteResources_QualifiedNamesOnCollision(t *testing.T) {
	plugin.Reset()
	// Register two plugins with the same name but different groups.
	plugin.Register(&mockPlugin{
		name:      "certificates",
		shortName: "cert",
		gvr:       schema.GroupVersionResource{Group: "certificates.k8s.io", Version: "v1", Resource: "certificates"},
	})
	plugin.Register(&mockPlugin{
		name:      "certificates",
		shortName: "cert",
		gvr:       schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	})

	completions, directive := completeResources()
	if directive != cobra.ShellCompDirectiveNoFileComp {
		t.Errorf("expected ShellCompDirectiveNoFileComp, got %v", directive)
	}

	// Should contain qualified names for both colliding resources.
	qualified1 := "certificates.certificates.k8s.io/v1"
	qualified2 := "certificates.cert-manager.io/v1"
	var found1, found2 bool
	for _, c := range completions {
		if c == qualified1 {
			found1 = true
		}
		if c == qualified2 {
			found2 = true
		}
	}
	if !found1 {
		t.Errorf("expected qualified completion %q, not found in %v", qualified1, completions)
	}
	if !found2 {
		t.Errorf("expected qualified completion %q, not found in %v", qualified2, completions)
	}
}

func TestParseDetailMode_ValidModes(t *testing.T) {
	tests := []struct {
		input string
		want  msgs.DetailMode
	}{
		{"y", msgs.DetailYAML},
		{"yaml", msgs.DetailYAML},
		{"d", msgs.DetailDescribe},
		{"describe", msgs.DetailDescribe},
		{"l", msgs.DetailLogs},
		{"logs", msgs.DetailLogs},
	}
	for _, tt := range tests {
		mode, err := parseDetailMode(tt.input)
		if err != nil {
			t.Fatalf("parseDetailMode(%q): unexpected error: %v", tt.input, err)
		}
		if mode == nil {
			t.Fatalf("parseDetailMode(%q): expected non-nil mode", tt.input)
		}
		if *mode != tt.want {
			t.Errorf("parseDetailMode(%q) = %v, want %v", tt.input, *mode, tt.want)
		}
	}
}

func TestParseDetailMode_Empty(t *testing.T) {
	mode, err := parseDetailMode("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mode != nil {
		t.Errorf("expected nil for empty string, got %v", *mode)
	}
}

func TestParseDetailMode_Invalid(t *testing.T) {
	_, err := parseDetailMode("foo")
	if err == nil {
		t.Fatal("expected error for invalid detail mode")
	}
}
