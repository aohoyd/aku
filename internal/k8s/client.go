package k8s

import (
	"context"
	"fmt"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// warningHandler captures K8s API warnings and forwards them to the TUI.
type warningHandler struct {
	mu   sync.Mutex
	send func(tea.Msg)
}

func (h *warningHandler) HandleWarningHeader(_ int, _ string, text string) {
	h.mu.Lock()
	send := h.send
	h.mu.Unlock()
	if send != nil {
		send(msgs.WarningMsg{Text: text})
	}
}

func (h *warningHandler) SetSend(fn func(tea.Msg)) {
	h.mu.Lock()
	h.send = fn
	h.mu.Unlock()
}

// Client bundles the typed and dynamic Kubernetes clients with context metadata.
type Client struct {
	Dynamic        dynamic.Interface
	Typed          kubernetes.Interface
	Config         *rest.Config
	Context        string
	Namespace      string
	WarningHandler *warningHandler
}

// NewClient creates a Kubernetes client from the given kubeconfig path.
// contextOverride and namespaceOverride can be empty to use defaults.
func NewClient(kubeconfigPath, contextOverride, namespaceOverride string) (*Client, error) {
	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	configOverrides := &clientcmd.ConfigOverrides{}

	if contextOverride != "" {
		configOverrides.CurrentContext = contextOverride
	}
	if namespaceOverride != "" {
		configOverrides.Context.Namespace = namespaceOverride
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to build rest config: %w", err)
	}

	wh := &warningHandler{}
	restConfig.WarningHandler = wh

	dynClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	typedClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create typed client: %w", err)
	}

	// Read current context and namespace from raw config
	rawConfig, err := kubeConfig.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to read raw config: %w", err)
	}

	currentCtx := rawConfig.CurrentContext
	if contextOverride != "" {
		currentCtx = contextOverride
	}

	namespace := "default"
	if namespaceOverride != "" {
		namespace = namespaceOverride
	} else if ctx, ok := rawConfig.Contexts[currentCtx]; ok && ctx.Namespace != "" {
		namespace = ctx.Namespace
	}

	return &Client{
		Dynamic:        dynClient,
		Typed:          typedClient,
		Config:         restConfig,
		Context:        currentCtx,
		Namespace:      namespace,
		WarningHandler: wh,
	}, nil
}

// WithNamespace returns a copy of the client with updated namespace.
func (c *Client) WithNamespace(ns string) *Client {
	return &Client{
		Dynamic:        c.Dynamic,
		Typed:          c.Typed,
		Config:         c.Config,
		Context:        c.Context,
		Namespace:      ns,
		WarningHandler: c.WarningHandler,
	}
}

// ListNamespaces returns all namespace names the user can see.
func (c *Client) ListNamespaces(ctx context.Context) ([]string, error) {
	nsList, err := c.Typed.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	names := make([]string, len(nsList.Items))
	for i, ns := range nsList.Items {
		names[i] = ns.Name
	}
	return names, nil
}
