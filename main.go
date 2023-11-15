package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aohoyd/aku/internal/app"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/configmaps"
	"github.com/aohoyd/aku/internal/plugins/containers"
	"github.com/aohoyd/aku/internal/plugins/cronjobs"
	"github.com/aohoyd/aku/internal/plugins/daemonsets"
	"github.com/aohoyd/aku/internal/plugins/deployments"
	"github.com/aohoyd/aku/internal/plugins/endpoints"
	"github.com/aohoyd/aku/internal/plugins/events"
	"github.com/aohoyd/aku/internal/plugins/horizontalpodautoscalers"
	"github.com/aohoyd/aku/internal/plugins/ingresses"
	"github.com/aohoyd/aku/internal/plugins/jobs"
	"github.com/aohoyd/aku/internal/plugins/limitranges"
	"github.com/aohoyd/aku/internal/plugins/namespaces"
	"github.com/aohoyd/aku/internal/plugins/networkpolicies"
	"github.com/aohoyd/aku/internal/plugins/nodes"
	"github.com/aohoyd/aku/internal/plugins/persistentvolumeclaims"
	"github.com/aohoyd/aku/internal/plugins/persistentvolumes"
	"github.com/aohoyd/aku/internal/plugins/poddisruptionbudgets"
	"github.com/aohoyd/aku/internal/plugins/pods"
	"github.com/aohoyd/aku/internal/plugins/replicasets"
	"github.com/aohoyd/aku/internal/plugins/resourcequotas"
	"github.com/aohoyd/aku/internal/plugins/secrets"
	"github.com/aohoyd/aku/internal/plugins/serviceaccounts"
	"github.com/aohoyd/aku/internal/plugins/services"
	"github.com/aohoyd/aku/internal/plugins/statefulsets"

	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/plugins/apiresources"
	"github.com/aohoyd/aku/internal/plugins/helmreleases"
	"github.com/aohoyd/aku/internal/plugins/portforwards"
	"github.com/aohoyd/aku/internal/portforward"
	"github.com/aohoyd/aku/internal/plugins/certificatesigningrequests"
	"github.com/aohoyd/aku/internal/plugins/clusterrolebindings"
	"github.com/aohoyd/aku/internal/plugins/clusterroles"
	"github.com/aohoyd/aku/internal/plugins/customresourcedefinitions"
	"github.com/aohoyd/aku/internal/plugins/endpointslices"
	"github.com/aohoyd/aku/internal/plugins/gatewayclasses"
	"github.com/aohoyd/aku/internal/plugins/gateways"
	"github.com/aohoyd/aku/internal/plugins/grpcroutes"
	"github.com/aohoyd/aku/internal/plugins/httproutes"
	"github.com/aohoyd/aku/internal/plugins/ingressclasses"
	"github.com/aohoyd/aku/internal/plugins/leases"
	"github.com/aohoyd/aku/internal/plugins/mutatingwebhookconfigurations"
	"github.com/aohoyd/aku/internal/plugins/priorityclasses"
	"github.com/aohoyd/aku/internal/plugins/referencegrants"
	"github.com/aohoyd/aku/internal/plugins/validatingadmissionpolicies"
	"github.com/aohoyd/aku/internal/plugins/validatingadmissionpolicybindings"
	"github.com/aohoyd/aku/internal/plugins/validatingwebhookconfigurations"
	"github.com/aohoyd/aku/internal/plugins/rolebindings"
	"github.com/aohoyd/aku/internal/plugins/roles"
	"github.com/aohoyd/aku/internal/plugins/runtimeclasses"
	"github.com/aohoyd/aku/internal/plugins/storageclasses"

	tea "charm.land/bubbletea/v2"
	"github.com/go-logr/logr"
	"k8s.io/klog/v2"
)

func main() {
	// Suppress klog stderr output that breaks TUI display
	klog.SetOutput(io.Discard)
	klog.LogToStderr(false)
	klog.SetLogger(logr.Discard())

	// Load config
	km, cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Try to create k8s client
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		home, _ := os.UserHomeDir()
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	var k8sClient *k8s.Client
	var store *k8s.Store

	k8sClient, err = k8s.NewClient(kubeconfigPath, "", "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not connect to Kubernetes: %v\n", err)
		// Continue without k8s - app will show empty lists
	}

	// Create store
	if k8sClient != nil {
		store = k8s.NewStore(k8sClient.Dynamic, nil) // send func set after program creation
	}

	// Register built-in plugins
	for _, fn := range []func(*k8s.Client, *k8s.Store) plugin.ResourcePlugin{
		pods.New,
		deployments.New,
		statefulsets.New,
		daemonsets.New,
		replicasets.New,
		services.New,
		configmaps.New,
		namespaces.New,
		containers.New,
		nodes.New,
		secrets.New,
		jobs.New,
		cronjobs.New,
		persistentvolumeclaims.New,
		persistentvolumes.New,
		events.New,
		serviceaccounts.New,
		endpoints.New,
		resourcequotas.New,
		limitranges.New,
		ingresses.New,
		horizontalpodautoscalers.New,
		networkpolicies.New,
		poddisruptionbudgets.New,
		// RBAC
		roles.New,
		rolebindings.New,
		clusterroles.New,
		clusterrolebindings.New,
		// Storage
		storageclasses.New,
		// Networking
		ingressclasses.New,
		// API Extensions
		customresourcedefinitions.New,
		// Discovery
		endpointslices.New,
		// Coordination
		leases.New,
		// Scheduling
		priorityclasses.New,
		// Node
		runtimeclasses.New,
		// Certificates
		certificatesigningrequests.New,
		// Gateway API
		gatewayclasses.New,
		gateways.New,
		httproutes.New,
		grpcroutes.New,
		referencegrants.New,
		// Admission Control
		mutatingwebhookconfigurations.New,
		validatingwebhookconfigurations.New,
		validatingadmissionpolicies.New,
		validatingadmissionpolicybindings.New,
	} {
		plugin.Register(fn(k8sClient, store))
	}

	// API Resources (synthetic view)
	plugin.Register(apiresources.New())

	// Helm — always create the resolver so runtime SetChartRef mutations are visible.
	if cfg.Charts == nil {
		cfg.Charts = make(map[string]map[string]string)
	}
	chartResolver := helm.NewConfigChartResolver(cfg.Charts)
	hrPlugin := helmreleases.New(k8sClient, store, chartResolver)
	plugin.Register(hrPlugin)

	// Port-forward registry and plugin
	pfRegistry := portforward.NewRegistry()
	plugin.Register(portforwards.New(pfRegistry))

	// Create app
	application := app.New(k8sClient, store, km, cfg, pfRegistry, hrPlugin.HelmClient())

	// Create program
	p := tea.NewProgram(application)

	// Wire send functions
	if store != nil {
		store.SetSend(p.Send)
	}
	if k8sClient != nil {
		k8sClient.WarningHandler.SetSend(p.Send)
	}
	if _, err := p.Run(); err != nil {
		pfRegistry.StopAll()
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	pfRegistry.StopAll()
}
