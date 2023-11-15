package serviceaccounts

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}

// Plugin implements plugin.ResourcePlugin for Kubernetes ServiceAccounts.
type Plugin struct{}

// New creates a new ServiceAccount plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "serviceaccounts" }
func (p *Plugin) ShortName() string                { return "sa" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "SECRETS", Width: 10},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	secretsCount := 0
	secrets, found, _ := unstructured.NestedSlice(obj.Object, "secrets")
	if found {
		secretsCount = len(secrets)
	}

	age := render.FormatAge(obj)

	return []string{name, fmt.Sprintf("%d", secretsCount), age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	sa, err := toServiceAccount(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to ServiceAccount: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", sa.Name)
	b.KV(render.LEVEL_0, "Namespace", sa.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", sa.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", sa.Annotations)

	// Secrets
	b.Section(render.LEVEL_0, "Mountable secrets")
	if len(sa.Secrets) > 0 {
		for _, s := range sa.Secrets {
			b.RawLine(render.LEVEL_1, s.Name)
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	return b.Build(), nil
}

// toServiceAccount converts an unstructured object to a typed corev1.ServiceAccount.
func toServiceAccount(obj *unstructured.Unstructured) (*corev1.ServiceAccount, error) {
	var sa corev1.ServiceAccount
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &sa); err != nil {
		return nil, err
	}
	return &sa, nil
}
