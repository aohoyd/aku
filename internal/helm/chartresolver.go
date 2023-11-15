package helm

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
)

// ChartResolver resolves a chart reference to a fully-loaded chart with dependencies.
// Returns (nil, nil) if no chart reference is configured (fallback to stored chart).
type ChartResolver interface {
	Resolve(namespace, releaseName, version string) (*chart.Chart, error)
}

type configChartResolver struct {
	charts map[string]map[string]string
}

// NewConfigChartResolver creates a ChartResolver backed by the config's charts map.
// The map is read live — mutations via Config.SetChartRef are immediately visible.
func NewConfigChartResolver(charts map[string]map[string]string) ChartResolver {
	return &configChartResolver{charts: charts}
}

func (r *configChartResolver) Resolve(namespace, releaseName, version string) (*chart.Chart, error) {
	if r.charts == nil {
		return nil, nil
	}
	nsCharts, ok := r.charts[namespace]
	if !ok {
		return nil, nil
	}
	ref, ok := nsCharts[releaseName]
	if !ok || ref == "" {
		return nil, nil
	}

	if strings.HasPrefix(ref, "oci://") {
		return r.loadOCI(ref, version)
	}
	if strings.HasPrefix(ref, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir: %w", err)
		}
		ref = filepath.Join(home, ref[2:])
	}
	return loader.Load(ref)
}

// stripOCITag removes the tag suffix (e.g. ":1.0.0") from an OCI reference,
// preserving any port number in the registry host.
func stripOCITag(ref string) string {
	base := strings.TrimPrefix(ref, "oci://")
	lastSlash := strings.LastIndex(base, "/")
	lastColon := strings.LastIndex(base, ":")
	// A colon is a tag separator only if it appears after the last slash
	// (or if there is no slash at all, e.g. "registry.example.com:latest").
	if lastColon >= 0 && lastColon > lastSlash {
		base = base[:lastColon]
	}
	return "oci://" + base
}

func (r *configChartResolver) loadOCI(ref, version string) (*chart.Chart, error) {
	client, err := registry.NewClient()
	if err != nil {
		return nil, fmt.Errorf("registry client: %w", err)
	}
	// Append version as tag, stripping any existing tag first
	pullRef := ref
	if version != "" {
		pullRef = stripOCITag(ref) + ":" + version
	}
	result, err := client.Pull(pullRef, registry.PullOptWithChart(true))
	if err != nil {
		return nil, fmt.Errorf("registry pull %s: %w", pullRef, err)
	}
	if result.Chart == nil {
		return nil, fmt.Errorf("registry pull returned no chart data for %s", pullRef)
	}
	return loader.LoadArchive(bytes.NewBuffer(result.Chart.Data))
}
