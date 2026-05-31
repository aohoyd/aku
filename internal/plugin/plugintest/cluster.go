// Package plugintest provides test-only helpers shared across the many plugin
// test packages. It is a normal (non-_test) package only because Go cannot
// share a _test.go helper across packages; nothing in production code imports
// it, and it is meant to be imported only from _test.go files.
package plugintest

import "github.com/aohoyd/aku/internal/k8s"

// FakeCluster is a trivial plugin.Cluster implementation for tests. It lets
// plugin unit tests inject a known store (and optionally a discovery index)
// into DescribeUncovered / DrillDown call sites without depending on the
// internal/cluster package.
type FakeCluster struct {
	StoreVal     *k8s.Store
	DiscoveryVal *k8s.Discovery
}

// NewFakeCluster builds a FakeCluster wrapping the given store. Discovery is
// left nil; use NewFakeClusterWithDiscovery when a resolver is needed.
func NewFakeCluster(store *k8s.Store) *FakeCluster {
	return &FakeCluster{StoreVal: store}
}

// NewFakeClusterWithDiscovery builds a FakeCluster wrapping both a store and a
// discovery index.
func NewFakeClusterWithDiscovery(store *k8s.Store, discovery *k8s.Discovery) *FakeCluster {
	return &FakeCluster{StoreVal: store, DiscoveryVal: discovery}
}

func (c *FakeCluster) Store() *k8s.Store         { return c.StoreVal }
func (c *FakeCluster) Discovery() *k8s.Discovery { return c.DiscoveryVal }
