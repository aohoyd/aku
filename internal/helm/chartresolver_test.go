package helm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigChartResolverNoEntry(t *testing.T) {
	resolver := NewConfigChartResolver(nil)
	ch, err := resolver.Resolve("ns", "release", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch != nil {
		t.Fatal("expected nil chart for no config entry")
	}
}

func TestConfigChartResolverEmptyMap(t *testing.T) {
	charts := map[string]map[string]string{}
	resolver := NewConfigChartResolver(charts)
	ch, err := resolver.Resolve("ns", "release", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ch != nil {
		t.Fatal("expected nil chart for empty config")
	}
}

func TestConfigChartResolverLocalPath(t *testing.T) {
	charts := map[string]map[string]string{
		"default": {"my-app": "/nonexistent/path/to/chart"},
	}
	resolver := NewConfigChartResolver(charts)
	_, err := resolver.Resolve("default", "my-app", "1.0.0")
	if err == nil {
		t.Fatal("expected error for nonexistent local path")
	}
}

func TestConfigChartResolverTildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("cannot get home dir: %v", err)
	}

	charts := map[string]map[string]string{
		"default": {"my-app": "~/charts/my-app"},
	}
	resolver := NewConfigChartResolver(charts)
	_, err = resolver.Resolve("default", "my-app", "1.0.0")
	if err == nil {
		t.Fatal("expected error for nonexistent expanded path")
	}

	expanded := filepath.Join(home, "charts/my-app")
	if !strings.Contains(err.Error(), expanded) {
		t.Fatalf("expected error to contain expanded path %q, got: %v", expanded, err)
	}
	if strings.Contains(err.Error(), "~/") {
		t.Fatalf("expected error to not contain unexpanded ~/: %v", err)
	}
}

func TestStripOCITag(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"port with tag", "oci://registry.example.com:5000/charts/myapp:1.0.0", "oci://registry.example.com:5000/charts/myapp"},
		{"port no tag", "oci://registry.example.com:5000/charts/myapp", "oci://registry.example.com:5000/charts/myapp"},
		{"no port with tag", "oci://registry.example.com/charts/myapp:1.0.0", "oci://registry.example.com/charts/myapp"},
		{"no port no tag", "oci://registry.example.com/charts/myapp", "oci://registry.example.com/charts/myapp"},
		{"tag only no path", "oci://registry.example.com:latest", "oci://registry.example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripOCITag(tt.ref); got != tt.want {
				t.Errorf("stripOCITag(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestConfigChartResolverLiveConfig(t *testing.T) {
	charts := map[string]map[string]string{}
	resolver := NewConfigChartResolver(charts)

	ch, err := resolver.Resolve("ns", "rel", "1.0.0")
	if err != nil || ch != nil {
		t.Fatal("expected nil for missing entry")
	}

	charts["ns"] = map[string]string{"rel": "/still/nonexistent"}
	_, err = resolver.Resolve("ns", "rel", "1.0.0")
	if err == nil {
		t.Fatal("expected error for nonexistent path after live update")
	}
}
