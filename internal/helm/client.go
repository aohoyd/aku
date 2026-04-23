package helm

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"helm.sh/helm/v4/pkg/action"
	"helm.sh/helm/v4/pkg/kube"
	release "helm.sh/helm/v4/pkg/release/v1"
	releaseiface "helm.sh/helm/v4/pkg/release"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

// ReleaseInfo holds the fields ktui needs from a Helm release.
type ReleaseInfo struct {
	Name       string
	Namespace  string
	Revision   int
	Chart      string
	AppVersion string
	Status     string
	Updated    time.Time
	Manifest   string
}

// RevisionInfo holds a single history entry.
type RevisionInfo struct {
	Revision    int
	Updated     time.Time
	Status      string
	Chart       string
	AppVersion  string
	Description string
}

// Client abstracts Helm operations for testability.
type Client interface {
	ListReleases(namespace string) ([]ReleaseInfo, error)
	GetRelease(name, namespace string) (*ReleaseInfo, error)
	// GetValues returns the values for the named release in the given
	// namespace. When all is false, only user-supplied overrides are
	// returned (equivalent to `helm get values <release>`). When all is
	// true, the full coalesced set — chart defaults merged with user
	// overrides — is returned (equivalent to `helm get values <release>
	// --all`).
	GetValues(name, namespace string, all bool) (map[string]any, error)
	History(name, namespace string) ([]RevisionInfo, error)
	Upgrade(name, namespace string, values map[string]any) error
	Rollback(name, namespace string, revision int) error
	Uninstall(name, namespace string) error
}

// ReleaseToUnstructured converts a ReleaseInfo to an unstructured object
// suitable for the plugin table view.
func ReleaseToUnstructured(r ReleaseInfo) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              r.Name,
				"namespace":         r.Namespace,
				"creationTimestamp": r.Updated.Format(time.RFC3339),
			},
			"revision":   fmt.Sprintf("%d", r.Revision),
			"chart":      r.Chart,
			"appVersion": r.AppVersion,
			"status":     r.Status,
			"updated":    r.Updated.Format(time.RFC3339),
			"_manifest":  r.Manifest,
		},
	}
}

// liveClient implements Client using the Helm Go SDK.
type liveClient struct {
	restConfig    *rest.Config
	chartResolver ChartResolver
}

// NewClient creates a new Helm client backed by the Helm Go SDK.
func NewClient(cfg *rest.Config, resolver ChartResolver) Client {
	return &liveClient{restConfig: cfg, chartResolver: resolver}
}

func (c *liveClient) newActionConfig(namespace string) (*action.Configuration, error) {
	flags := &genericclioptions.ConfigFlags{
		WrapConfigFn: func(_ *rest.Config) *rest.Config {
			return c.restConfig
		},
	}
	cfg := new(action.Configuration)
	if err := cfg.Init(flags, namespace, ""); err != nil {
		return nil, fmt.Errorf("helm config: %w", err)
	}
	return cfg, nil
}

func toRelease(r releaseiface.Releaser) (*release.Release, error) {
	rel, ok := r.(*release.Release)
	if !ok {
		return nil, fmt.Errorf("unexpected release type %T", r)
	}
	return rel, nil
}

func releaseToInfo(r *release.Release) ReleaseInfo {
	info := ReleaseInfo{
		Name:      r.Name,
		Namespace: r.Namespace,
		Revision:  r.Version,
		Manifest:  r.Manifest,
	}
	if r.Chart != nil && r.Chart.Metadata != nil {
		info.Chart = r.Chart.Metadata.Name + "-" + r.Chart.Metadata.Version
		info.AppVersion = r.Chart.Metadata.AppVersion
	}
	if r.Info != nil {
		info.Status = string(r.Info.Status)
		info.Updated = r.Info.LastDeployed
	}
	return info
}

func (c *liveClient) ListReleases(namespace string) ([]ReleaseInfo, error) {
	cfg, err := c.newActionConfig(namespace)
	if err != nil {
		return nil, err
	}
	list := action.NewList(cfg)
	list.StateMask = action.ListAll
	if namespace == "" {
		list.AllNamespaces = true
	}
	releases, err := list.Run()
	if err != nil {
		return nil, fmt.Errorf("helm list: %w", err)
	}
	result := make([]ReleaseInfo, 0, len(releases))
	for _, r := range releases {
		rel, err := toRelease(r)
		if err != nil {
			return nil, fmt.Errorf("helm list: %w", err)
		}
		result = append(result, releaseToInfo(rel))
	}
	return result, nil
}

func (c *liveClient) GetRelease(name, namespace string) (*ReleaseInfo, error) {
	cfg, err := c.newActionConfig(namespace)
	if err != nil {
		return nil, err
	}
	get := action.NewGet(cfg)
	r, err := get.Run(name)
	if err != nil {
		return nil, fmt.Errorf("helm get: %w", err)
	}
	rel, err := toRelease(r)
	if err != nil {
		return nil, fmt.Errorf("helm get: %w", err)
	}
	info := releaseToInfo(rel)
	return &info, nil
}

func (c *liveClient) GetValues(name, namespace string, all bool) (map[string]any, error) {
	cfg, err := c.newActionConfig(namespace)
	if err != nil {
		return nil, err
	}
	return runGetValues(cfg, name, all)
}

// runGetValues is a thin seam around action.NewGetValues so the propagation
// of the `all` flag onto action.GetValues.AllValues can be unit-tested
// without instantiating a live Helm action configuration.
func runGetValues(cfg *action.Configuration, name string, all bool) (map[string]any, error) {
	gv := action.NewGetValues(cfg)
	gv.AllValues = all
	vals, err := gv.Run(name)
	if err != nil {
		return nil, fmt.Errorf("helm get values: %w", err)
	}
	return vals, nil
}

func (c *liveClient) History(name, namespace string) ([]RevisionInfo, error) {
	cfg, err := c.newActionConfig(namespace)
	if err != nil {
		return nil, err
	}
	hist := action.NewHistory(cfg)
	hist.Max = 256
	raw, err := hist.Run(name)
	if err != nil {
		return nil, fmt.Errorf("helm history: %w", err)
	}
	releases := make([]*release.Release, 0, len(raw))
	for _, r := range raw {
		rel, err := toRelease(r)
		if err != nil {
			return nil, fmt.Errorf("helm history: %w", err)
		}
		releases = append(releases, rel)
	}
	slices.SortFunc(releases, func(a, b *release.Release) int {
		return cmp.Compare(b.Version, a.Version)
	})
	result := make([]RevisionInfo, len(releases))
	for i, r := range releases {
		ri := RevisionInfo{
			Revision: r.Version,
		}
		if r.Chart != nil && r.Chart.Metadata != nil {
			ri.Chart = r.Chart.Metadata.Name + "-" + r.Chart.Metadata.Version
			ri.AppVersion = r.Chart.Metadata.AppVersion
		}
		if r.Info != nil {
			ri.Status = string(r.Info.Status)
			ri.Updated = r.Info.LastDeployed
			ri.Description = r.Info.Description
		}
		result[i] = ri
	}
	return result, nil
}

func (c *liveClient) Upgrade(name, namespace string, values map[string]any) error {
	cfg, err := c.newActionConfig(namespace)
	if err != nil {
		return err
	}
	get := action.NewGet(cfg)
	r, err := get.Run(name)
	if err != nil {
		return fmt.Errorf("helm get for upgrade: %w", err)
	}
	rel, err := toRelease(r)
	if err != nil {
		return fmt.Errorf("helm get for upgrade: %w", err)
	}

	ch := rel.Chart
	if c.chartResolver != nil {
		version := ""
		if rel.Chart != nil && rel.Chart.Metadata != nil {
			version = rel.Chart.Metadata.Version
		}
		resolved, resolveErr := c.chartResolver.Resolve(namespace, name, version)
		if resolveErr != nil {
			return fmt.Errorf("chart resolve: %w", resolveErr)
		}
		if resolved != nil {
			ch = resolved
		}
	}

	upgrade := action.NewUpgrade(cfg)
	upgrade.Namespace = namespace
	upgrade.ReuseValues = false
	upgrade.WaitStrategy = kube.StatusWatcherStrategy
	if _, err := upgrade.Run(name, ch, values); err != nil {
		return fmt.Errorf("helm upgrade: %w", err)
	}
	return nil
}

func (c *liveClient) Rollback(name, namespace string, revision int) error {
	cfg, err := c.newActionConfig(namespace)
	if err != nil {
		return err
	}
	rb := action.NewRollback(cfg)
	rb.Version = revision
	rb.WaitStrategy = kube.StatusWatcherStrategy
	if err := rb.Run(name); err != nil {
		return fmt.Errorf("helm rollback: %w", err)
	}
	return nil
}

func (c *liveClient) Uninstall(name, namespace string) error {
	cfg, err := c.newActionConfig(namespace)
	if err != nil {
		return err
	}
	uninstall := action.NewUninstall(cfg)
	uninstall.WaitStrategy = kube.StatusWatcherStrategy
	if _, err := uninstall.Run(name); err != nil {
		return fmt.Errorf("helm uninstall: %w", err)
	}
	return nil
}
