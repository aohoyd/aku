package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aohoyd/aku/internal/app"
	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/notify"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/apiresources"
	"github.com/aohoyd/aku/internal/plugins/certificatesigningrequests"
	"github.com/aohoyd/aku/internal/plugins/clusterrolebindings"
	"github.com/aohoyd/aku/internal/plugins/clusterroles"
	"github.com/aohoyd/aku/internal/plugins/configmaps"
	"github.com/aohoyd/aku/internal/plugins/containers"
	"github.com/aohoyd/aku/internal/plugins/contexts"
	"github.com/aohoyd/aku/internal/plugins/cronjobs"
	"github.com/aohoyd/aku/internal/plugins/customresourcedefinitions"
	"github.com/aohoyd/aku/internal/plugins/daemonsets"
	"github.com/aohoyd/aku/internal/plugins/deployments"
	"github.com/aohoyd/aku/internal/plugins/endpoints"
	"github.com/aohoyd/aku/internal/plugins/endpointslices"
	"github.com/aohoyd/aku/internal/plugins/events"
	"github.com/aohoyd/aku/internal/plugins/gatewayclasses"
	"github.com/aohoyd/aku/internal/plugins/gateways"
	"github.com/aohoyd/aku/internal/plugins/grpcroutes"
	"github.com/aohoyd/aku/internal/plugins/helmreleases"
	"github.com/aohoyd/aku/internal/plugins/horizontalpodautoscalers"
	"github.com/aohoyd/aku/internal/plugins/httproutes"
	"github.com/aohoyd/aku/internal/plugins/ingressclasses"
	"github.com/aohoyd/aku/internal/plugins/ingresses"
	"github.com/aohoyd/aku/internal/plugins/jobs"
	"github.com/aohoyd/aku/internal/plugins/leases"
	"github.com/aohoyd/aku/internal/plugins/limitranges"
	"github.com/aohoyd/aku/internal/plugins/mutatingwebhookconfigurations"
	"github.com/aohoyd/aku/internal/plugins/namespaces"
	"github.com/aohoyd/aku/internal/plugins/networkpolicies"
	"github.com/aohoyd/aku/internal/plugins/nodes"
	"github.com/aohoyd/aku/internal/plugins/notifications"
	"github.com/aohoyd/aku/internal/plugins/persistentvolumeclaims"
	"github.com/aohoyd/aku/internal/plugins/persistentvolumes"
	"github.com/aohoyd/aku/internal/plugins/poddisruptionbudgets"
	"github.com/aohoyd/aku/internal/plugins/pods"
	"github.com/aohoyd/aku/internal/plugins/portforwards"
	"github.com/aohoyd/aku/internal/plugins/priorityclasses"
	"github.com/aohoyd/aku/internal/plugins/referencegrants"
	"github.com/aohoyd/aku/internal/plugins/replicasets"
	"github.com/aohoyd/aku/internal/plugins/resourcequotas"
	"github.com/aohoyd/aku/internal/plugins/rolebindings"
	"github.com/aohoyd/aku/internal/plugins/roles"
	"github.com/aohoyd/aku/internal/plugins/runtimeclasses"
	"github.com/aohoyd/aku/internal/plugins/secrets"
	"github.com/aohoyd/aku/internal/plugins/serviceaccounts"
	"github.com/aohoyd/aku/internal/plugins/services"
	"github.com/aohoyd/aku/internal/plugins/statefulsets"
	"github.com/aohoyd/aku/internal/plugins/storageclasses"
	"github.com/aohoyd/aku/internal/plugins/validatingadmissionpolicies"
	"github.com/aohoyd/aku/internal/plugins/validatingadmissionpolicybindings"
	"github.com/aohoyd/aku/internal/plugins/validatingwebhookconfigurations"
	"github.com/aohoyd/aku/internal/portforward"
	"github.com/aohoyd/aku/internal/theme"
	"github.com/aohoyd/aku/pkg/build"

	tea "charm.land/bubbletea/v2"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

var (
	kubeconfig string
	context_   string
	namespace  string
	resources  []string
	details    string
	layoutFlag string
)

var rootCmd = &cobra.Command{
	Use:   "aku",
	Short: "Kubernetes TUI",
	RunE:  run,
}

func init() {
	v := build.Version
	if build.Commit != "" {
		v += " (" + build.Commit + ")"
	}
	rootCmd.Version = v

	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "path to the kubeconfig file")
	rootCmd.PersistentFlags().StringVar(&context_, "context", "", "the kubeconfig context to use")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "kubernetes namespace")
	rootCmd.PersistentFlags().StringSliceVarP(&resources, "resource", "r", nil, "resources to display (e.g. pods,deploy or -r svc -r deploy)")
	rootCmd.PersistentFlags().StringVarP(&details, "details", "d", "", "open detail panel on startup (y/yaml, d/describe, l/logs)")
	rootCmd.PersistentFlags().StringVarP(&layoutFlag, "layout", "l", "", "layout orientation (v/vertical, h/horizontal)")

	// Register flag completion functions
	rootCmd.RegisterFlagCompletionFunc("context", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeContexts(kubeconfig)
	})
	rootCmd.RegisterFlagCompletionFunc("namespace", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeNamespaces(kubeconfig, context_)
	})
	rootCmd.RegisterFlagCompletionFunc("resource", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeResources()
	})

	rootCmd.RegisterFlagCompletionFunc("details", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"y\tShow resource YAML",
			"yaml\tShow resource YAML",
			"d\tShow resource description",
			"describe\tShow resource description",
			"l\tShow resource logs",
			"logs\tShow resource logs",
		}, cobra.ShellCompDirectiveNoFileComp
	})
	rootCmd.RegisterFlagCompletionFunc("layout", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{
			"v\tVertical (resources left, details right)",
			"vertical\tVertical (resources left, details right)",
			"h\tHorizontal (resources top, details bottom)",
			"horizontal\tHorizontal (resources top, details bottom)",
		}, cobra.ShellCompDirectiveNoFileComp
	})

	// Add completion subcommand
	rootCmd.AddCommand(completionCmd)
}

// completionCmd generates shell completion scripts.
var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion script for aku.

To load completions:

Bash:
  $ source <(aku completion bash)

Zsh:
  $ source <(aku completion zsh)

Fish:
  $ aku completion fish | source

PowerShell:
  PS> aku completion powershell | Out-String | Invoke-Expression
`,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		default:
			return fmt.Errorf("unsupported shell: %s", args[0])
		}
	},
}

// completeContexts returns context names from the kubeconfig for shell completion.
func completeContexts(kubeconfigFlag string) ([]string, cobra.ShellCompDirective) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigFlag != "" {
		loadingRules.ExplicitPath = kubeconfigFlag
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{},
	).RawConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, 0, len(config.Contexts))
	for name := range config.Contexts {
		names = append(names, name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeNamespaces returns namespace names from the cluster for shell completion.
func completeNamespaces(kubeconfigFlag, contextFlag string) ([]string, cobra.ShellCompDirective) {
	kubeconfigPath := kubeconfigFlag
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}
	if kubeconfigPath == "" {
		home, _ := os.UserHomeDir()
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}
	client, err := k8s.NewClient(kubeconfigPath, contextFlag, "")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names, err := client.ListNamespaces(context.Background())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeResources returns resource names and short names from the plugin registry
// for shell completion.
func completeResources() ([]string, cobra.ShellCompDirective) {
	var completions []string
	allPlugins := plugin.All()
	for _, p := range allPlugins {
		completions = append(completions, fmt.Sprintf("%s\t%s", p.ShortName(), p.Name()))
		if p.Name() != p.ShortName() {
			completions = append(completions, p.Name())
		}
	}

	// For colliding names, add qualified completions.
	seen := make(map[string]bool)
	for _, p := range allPlugins {
		name := p.Name()
		if seen[name] || !plugin.HasNameCollision(name) {
			continue
		}
		seen[name] = true
		for _, pp := range plugin.AllByName(name) {
			gvr := pp.GVR()
			qualified := gvr.Resource + "." + gvr.Group + "/" + gvr.Version
			completions = append(completions, qualified)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// parseResourceSpecs converts raw -r flag values into app.ResourceSpec slice.
// Each raw value may contain an optional namespace prefix: "namespace/resource".
// The resource name is looked up via plugin.ByName which supports both full and short names.
func parseResourceSpecs(rawSpecs []string) ([]app.ResourceSpec, error) {
	if len(rawSpecs) == 0 {
		return nil, nil
	}

	var specs []app.ResourceSpec
	for _, raw := range rawSpecs {
		// Detect qualified names (e.g. "certificates.cert-manager.io/v1"):
		// if the part before the first "/" contains a ".", it's a qualified name.
		// Otherwise "namespace/resource" is a namespaced bare name.
		ns, name, found := strings.Cut(raw, "/")
		if !found || strings.ContainsRune(ns, '.') {
			ns = ""
			name = raw
		}

		var p plugin.ResourcePlugin
		var ok bool
		if strings.Contains(name, "/") {
			p, ok = plugin.ByQualifiedName(name)
		}
		if !ok {
			p, ok = plugin.ByName(name)
		}
		if !ok {
			return nil, fmt.Errorf("unknown resource %q", name)
		}
		specs = append(specs, app.ResourceSpec{Plugin: p, Namespace: ns})
	}
	return specs, nil
}

// parseDetailMode converts the --details flag value into a *msgs.DetailMode.
func parseDetailMode(s string) (*msgs.DetailMode, error) {
	if s == "" {
		return nil, nil
	}
	var mode msgs.DetailMode
	switch s {
	case "y", "yaml":
		mode = msgs.DetailYAML
	case "d", "describe":
		mode = msgs.DetailDescribe
	case "l", "logs":
		mode = msgs.DetailLogs
	default:
		return nil, fmt.Errorf("unknown detail mode %q, valid: y/yaml, d/describe, l/logs", s)
	}
	return &mode, nil
}

// parseLayoutOrientation converts the --layout flag value into a layout.Orientation.
func parseLayoutOrientation(s string) (layout.Orientation, error) {
	switch s {
	case "", "v", "vertical":
		return layout.OrientationVertical, nil
	case "h", "horizontal":
		return layout.OrientationHorizontal, nil
	default:
		return 0, fmt.Errorf("unknown layout %q, valid: v/vertical, h/horizontal", s)
	}
}

// warnThemeInit writes a non-fatal theme initialization warning to w. It is a
// no-op when warning is nil. It is a small wrapper (rather than an inline
// fmt.Fprintf like the k8s-connect warning below) so the formatting can be
// unit-tested without exercising the full run() path.
func warnThemeInit(w io.Writer, warning error) {
	if warning != nil {
		fmt.Fprintf(w, "Warning: %v\n", warning)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Suppress klog stderr output that breaks TUI display
	klog.SetOutput(io.Discard)
	klog.LogToStderr(false)
	klog.SetLogger(logr.Discard())

	// Load config
	km, cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Surface any non-fatal theme resolution warning (missing named theme or
	// unparseable theme file) before the TUI takes over the screen.
	warnThemeInit(os.Stderr, theme.InitWarning())

	// Resolve kubeconfig path: flag > env > default
	kubeconfigPath := kubeconfig
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}
	if kubeconfigPath == "" {
		home, _ := os.UserHomeDir()
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	// Scan kubeconfig files for contexts (configured dirs + the resolved
	// default kubeconfig). The scan can never fail as a whole — every per-file
	// failure is skipped silently — so there is no error to handle; a fully
	// failed scan just yields no extra entries and the global cluster still
	// connects via the base path.
	entries := cluster.ScanKubeconfigs(cfg.ContextDirectories(), cluster.DefaultKubeconfigFiles(kubeconfig))

	// Build the Cluster Session Manager. GetOrCreate eagerly creates and connects
	// the startup cluster (context_ == "" means the kubeconfig current-context);
	// the resulting cluster is cached under its resolved context name, which seeds
	// the initial pane(s).
	mgr := cluster.NewManager(entries, kubeconfigPath, cfg.APITimeout())
	startupCluster, connectErr := mgr.GetOrCreate(context_)
	if connectErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not connect to Kubernetes: %v\n", connectErr)
		// Continue without k8s - app will show empty lists (degraded startup).
	}

	// The explicit startup context the App seeds initial panes with. Prefer the
	// connected cluster's resolved name (an empty context_ resolves to the
	// kubeconfig current-context). On a degraded startup (connect failed, so the
	// cluster carries an empty context) fall back to a meaningful name so initial
	// panes are not stamped with "" — which would empty paneContexts() and let a
	// later SyncRefs tear down clusters. Prefer the explicit --context flag, then
	// the kubeconfig current-context name.
	startupContext := startupCluster.Context()
	if startupContext == "" {
		if context_ != "" {
			startupContext = context_
		} else {
			startupContext = cluster.CurrentContextName(kubeconfigPath)
		}
	}

	// Pull the startup cluster's client for plugin registration. Built-in plugins
	// are metadata-only and ignore it; helmreleases is the exception — it builds
	// its helm client from the startup client's config (see below).
	var k8sClient *k8s.Client
	if startupCluster != nil {
		k8sClient = startupCluster.Client()
	}

	// Register built-in plugins. These constructors are metadata-only and take no
	// args; the cluster (store/discovery) reaches them at call time via
	// plugin.Cluster. helmreleases is the exception (registered below) — it still
	// builds its Helm client from the global client.
	for _, fn := range []func() plugin.ResourcePlugin{
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
		plugin.Register(fn())
	}

	// API Resources (synthetic view)
	plugin.Register(apiresources.New())

	// Contexts (synthetic view over the kubeconfig contexts the Manager knows).
	plugin.Register(contexts.New(mgr))

	// Helm -- always create the resolver so runtime SetChartRef mutations are visible.
	if cfg.Charts == nil {
		cfg.Charts = make(map[string]map[string]string)
	}
	chartResolver := helm.NewConfigChartResolver(cfg.Charts)
	hrPlugin := helmreleases.New(k8sClient, chartResolver)
	plugin.Register(hrPlugin)

	// Port-forward registry and plugin
	pfRegistry := portforward.NewRegistry()
	plugin.Register(portforwards.New(pfRegistry))

	// Shared notify store for aku's own messages, created here so the
	// aku-messages plugin and the App share the SAME instance. SetSend
	// (program.Send) is wired after the program is created, below.
	notifyStore := notify.NewStore(cfg.NotifyBufferSize())
	plugin.Register(notifications.New(notifyStore))

	// Parse resource specs from -r flag
	specs, err := parseResourceSpecs(resources)
	if err != nil {
		return err
	}

	// Parse detail mode from -d flag
	detailMode, err := parseDetailMode(details)
	if err != nil {
		return err
	}

	// Parse layout orientation from -l flag
	orientation, err := parseLayoutOrientation(layoutFlag)
	if err != nil {
		return err
	}

	// Create app. The Manager is the single source of client/store/discovery;
	// chartResolver lets the app build per-cluster helm clients lazily.
	application := app.New(mgr, km, cfg, notifyStore, pfRegistry, chartResolver, specs, detailMode, orientation, startupContext)

	// Create program
	p := tea.NewProgram(application)

	// Wire the send function through the Manager. SetSend propagates to every
	// already-created cluster's store + warning handler (the startup cluster was
	// created by Connect above) and is recorded for clusters created later.
	mgr.SetSend(p.Send)

	// Wire the notify store's send so that Add emits MessageAddedMsg into the
	// Bubble Tea loop (driving the toast overlay). Must run before p.Run.
	notifyStore.SetSend(p.Send)

	teardown := func() {
		mgr.ForEach(func(c *cluster.Cluster) {
			if s := c.Store(); s != nil {
				s.UnsubscribeAll()
			}
		})
		pfRegistry.StopAll()
	}

	if _, err := p.Run(); err != nil {
		teardown()
		return fmt.Errorf("error: %w", err)
	}
	teardown()
	return nil
}
