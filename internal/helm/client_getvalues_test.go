package helm

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	chartcommon "helm.sh/helm/v4/pkg/chart/common"
	kubefake "helm.sh/helm/v4/pkg/kube/fake"
	"helm.sh/helm/v4/pkg/registry"
	releasecommon "helm.sh/helm/v4/pkg/release/common"
	release "helm.sh/helm/v4/pkg/release/v1"
	"helm.sh/helm/v4/pkg/storage"
	"helm.sh/helm/v4/pkg/storage/driver"
)

// newTestActionConfig builds an *action.Configuration backed by an in-memory
// release store and a no-op kube client. It's a unit-test-friendly stand-in
// for the live cluster wiring in liveClient.newActionConfig.
func newTestActionConfig(t *testing.T) *action.Configuration {
	t.Helper()
	regClient, err := registry.NewClient()
	if err != nil {
		t.Fatalf("registry client: %v", err)
	}
	return &action.Configuration{
		Releases:       storage.Init(driver.NewMemory()),
		KubeClient:     &kubefake.PrintingKubeClient{Out: io.Discard},
		Capabilities:   chartcommon.DefaultCapabilities,
		RegistryClient: regClient,
	}
}

func seedTestRelease(t *testing.T, cfg *action.Configuration, name string) {
	t.Helper()
	rel := &release.Release{
		Name: name,
		Info: &release.Info{
			Status: releasecommon.StatusDeployed,
		},
		Chart: &chart.Chart{
			Metadata: &chart.Metadata{Name: "test-chart", Version: "1.0.0"},
			Values: map[string]any{
				"defaultKey": "defaultValue",
				"replicaCount": 1,
			},
		},
		Config: map[string]any{
			"replicaCount": 5,
		},
		Version:   1,
		Namespace: "default",
	}
	if err := cfg.Releases.Create(rel); err != nil {
		t.Fatalf("seed release: %v", err)
	}
}

// TestRunGetValues_MissingReleaseError verifies the error wrapping path of
// runGetValues. If the underlying helm action returns an error (for example,
// a release that isn't in the store), the wrapper must surface it with the
// `helm get values:` prefix so the caller can identify the source.
func TestRunGetValues_MissingReleaseError(t *testing.T) {
	cfg := newTestActionConfig(t)
	// Note: no release seeded.

	vals, err := runGetValues(cfg, "does-not-exist", false)
	if err == nil {
		t.Fatalf("expected error for missing release, got vals=%v", vals)
	}
	if vals != nil {
		t.Errorf("expected nil values on error, got %v", vals)
	}
	if msg := err.Error(); !strings.Contains(msg, "helm get values:") {
		t.Errorf("expected wrapped error to include 'helm get values:', got %q", msg)
	}
}

// TestRunGetValues_EmptyUserValues verifies the user-values path for a
// release that has no user-supplied overrides. With all=false, runGetValues
// should return an empty (non-nil) map. This pins the behaviour the panel
// renderer relies on (it forwards through helm.MarshalValues which surfaces
// a friendly placeholder for the empty case).
func TestRunGetValues_EmptyUserValues(t *testing.T) {
	cfg := newTestActionConfig(t)
	rel := &release.Release{
		Name: "emptyrel",
		Info: &release.Info{Status: releasecommon.StatusDeployed},
		Chart: &chart.Chart{
			Metadata: &chart.Metadata{Name: "test-chart", Version: "1.0.0"},
			Values:   map[string]any{"defaultKey": "defaultValue"},
		},
		// No Config => no user values.
		Version:   1,
		Namespace: "default",
	}
	if err := cfg.Releases.Create(rel); err != nil {
		t.Fatalf("seed release: %v", err)
	}

	vals, err := runGetValues(cfg, "emptyrel", false)
	if err != nil {
		t.Fatalf("runGetValues: %v", err)
	}
	if len(vals) != 0 {
		t.Errorf("expected empty user values, got %v", vals)
	}
}

func TestRunGetValues_PropagatesAllFlag(t *testing.T) {
	tests := []struct {
		name             string
		all              bool
		wantDefaultKey   bool
		wantReplicaCount int
	}{
		{
			name:             "user values only",
			all:              false,
			wantDefaultKey:   false,
			wantReplicaCount: 5,
		},
		{
			name:             "all coalesced values",
			all:              true,
			wantDefaultKey:   true,
			wantReplicaCount: 5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := newTestActionConfig(t)
			seedTestRelease(t, cfg, "myrelease")

			vals, err := runGetValues(cfg, "myrelease", tc.all)
			if err != nil {
				t.Fatalf("runGetValues: %v", err)
			}

			_, hasDefault := vals["defaultKey"]
			if hasDefault != tc.wantDefaultKey {
				t.Errorf("defaultKey present=%v, want %v (vals=%v)", hasDefault, tc.wantDefaultKey, vals)
			}

			rc, ok := vals["replicaCount"]
			if !ok {
				t.Fatalf("expected replicaCount in values, got %v", vals)
			}
			// Compare via fmt.Sprint to avoid relying on the in-memory store
			// preserving Go's int type — a JSON-roundtripping driver would
			// surface a float64 here, which would silently fail an `any !=
			// int` comparison.
			if got, want := fmt.Sprint(rc), fmt.Sprint(tc.wantReplicaCount); got != want {
				t.Errorf("replicaCount=%v, want %v", got, want)
			}
		})
	}
}
