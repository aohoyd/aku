package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aohoyd/aku/internal/app"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/apiresources"
	"github.com/aohoyd/aku/internal/plugins/certificatesigningrequests"
	"github.com/aohoyd/aku/internal/plugins/clusterrolebindings"
	"github.com/aohoyd/aku/internal/plugins/clusterroles"
	"github.com/aohoyd/aku/internal/plugins/configmaps"
	"github.com/aohoyd/aku/internal/plugins/containers"
	"github.com/aohoyd/aku/internal/plugins/cronjobs"
	"github.com/aohoyd/aku/internal/plugins/customresourcedefinitions"
	"github.com/aohoyd/aku/internal/plugins/daemonsets"
	"github.com/aohoyd/aku/internal/plugins/deployments"
	"github.com/aohoyd/aku/internal/plugins/endpointslices"
	"github.com/aohoyd/aku/internal/plugins/endpoints"
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
)

var rootCmd = &cobra.Command{
	Use:   "aku",
	Short: "Kubernetes TUI",
	RunE:  run,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "path to the kubeconfig file")
	rootCmd.PersistentFlags().StringVar(&context_, "context", "", "the kubeconfig context to use")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "kubernetes namespace")
	rootCmd.PersistentFlags().StringSliceVarP(&resources, "resource", "r", nil, "resources to display (e.g. pods,deploy or -r svc -r deploy)")
	rootCmd.PersistentFlags().StringVarP(&details, "details", "d", "", "open detail panel on startup (y/yaml, d/describe, l/logs)")

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
	for _, p := range plugin.All() {
		completions = append(completions, fmt.Sprintf("%s\t%s", p.ShortName(), p.Name()))
		if p.Name() != p.ShortName() {
			completions = append(completions, p.Name())
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
		var ns, name string
		if idx := strings.IndexByte(raw, '/'); idx >= 0 {
			ns = raw[:idx]
			name = raw[idx+1:]
		} else {
			name = raw
		}
		p, ok := plugin.ByName(name)
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

	// Resolve kubeconfig path: flag > env > default
	kubeconfigPath := kubeconfig
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}
	if kubeconfigPath == "" {
		home, _ := os.UserHomeDir()
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	}

	// Create k8s client
	var k8sClient *k8s.Client
	var store *k8s.Store

	k8sClient, err = k8s.NewClient(kubeconfigPath, context_, namespace)
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

	// Helm -- always create the resolver so runtime SetChartRef mutations are visible.
	if cfg.Charts == nil {
		cfg.Charts = make(map[string]map[string]string)
	}
	chartResolver := helm.NewConfigChartResolver(cfg.Charts)
	hrPlugin := helmreleases.New(k8sClient, store, chartResolver)
	plugin.Register(hrPlugin)

	// Port-forward registry and plugin
	pfRegistry := portforward.NewRegistry()
	plugin.Register(portforwards.New(pfRegistry))

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

	// Create app
	application := app.New(k8sClient, store, km, cfg, pfRegistry, hrPlugin.HelmClient(), specs, detailMode)

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
		return fmt.Errorf("error: %w", err)
	}
	pfRegistry.StopAll()
	return nil
}
