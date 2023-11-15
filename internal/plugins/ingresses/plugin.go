package ingresses

import (
	"context"
	"fmt"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}

// Plugin implements plugin.ResourcePlugin for Kubernetes Ingresses.
type Plugin struct{}

// New creates a new Ingress plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "ingresses" }
func (p *Plugin) ShortName() string                { return "ing" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "CLASS", Width: 14},
		{Title: "HOSTS", Flex: true},
		{Title: "ADDRESS", Width: 16},
		{Title: "PORTS", Width: 10},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	className, _, _ := unstructured.NestedString(obj.Object, "spec", "ingressClassName")
	if className == "" {
		className = "<none>"
	}

	hosts := extractHosts(obj)
	address := extractAddress(obj)
	ports := extractPorts(obj)
	age := render.FormatAge(obj)

	return []string{name, className, hosts, address, ports, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	ing, err := toIngress(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("ingresses: decode: %w", err)
	}

	b := render.NewBuilder()

	// Basic metadata
	b.KV(render.LEVEL_0, "Name", ing.Name)
	b.KV(render.LEVEL_0, "Namespace", ing.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", ing.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", ing.Annotations)

	// Ingress Class
	if ing.Spec.IngressClassName != nil && *ing.Spec.IngressClassName != "" {
		b.KV(render.LEVEL_0, "IngressClass", *ing.Spec.IngressClassName)
	} else {
		b.KV(render.LEVEL_0, "IngressClass", "<none>")
	}

	// Default Backend
	if ing.Spec.DefaultBackend != nil {
		describeBackend(b, render.LEVEL_0, "Default Backend", ing.Spec.DefaultBackend)
	}

	// Rules
	if len(ing.Spec.Rules) > 0 {
		b.Section(render.LEVEL_0, "Rules")
		for _, rule := range ing.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			b.KV(render.LEVEL_1, "Host", host)
			if rule.HTTP != nil {
				for _, path := range rule.HTTP.Paths {
					pathStr := "/"
					if path.Path != "" {
						pathStr = path.Path
					}
					pathType := ""
					if path.PathType != nil {
						pathType = string(*path.PathType)
					}
					backendStr := formatIngressBackend(&path.Backend)
					b.KV(render.LEVEL_2, "Path", fmt.Sprintf("%s (%s) -> %s", pathStr, pathType, backendStr))
				}
			}
		}
	}

	// TLS
	if len(ing.Spec.TLS) > 0 {
		b.Section(render.LEVEL_0, "TLS")
		for _, tls := range ing.Spec.TLS {
			secretName := tls.SecretName
			if secretName == "" {
				secretName = "<none>"
			}
			b.KV(render.LEVEL_1, "Secret", secretName)
			if len(tls.Hosts) > 0 {
				b.KV(render.LEVEL_1, "Hosts", strings.Join(tls.Hosts, ", "))
			}
		}
	}

	return b.Build(), nil
}

// toIngress converts an unstructured object to a typed networkingv1.Ingress.
func toIngress(obj *unstructured.Unstructured) (*networkingv1.Ingress, error) {
	var ing networkingv1.Ingress
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &ing); err != nil {
		return nil, err
	}
	return &ing, nil
}

func describeBackend(b *render.Builder, level int, label string, backend *networkingv1.IngressBackend) {
	if backend.Service != nil {
		portStr := ""
		if backend.Service.Port.Number != 0 {
			portStr = fmt.Sprintf("%d", backend.Service.Port.Number)
		} else if backend.Service.Port.Name != "" {
			portStr = backend.Service.Port.Name
		}
		b.KV(level, label, fmt.Sprintf("%s:%s", backend.Service.Name, portStr))
	} else {
		b.KV(level, label, "<none>")
	}
}

func formatIngressBackend(backend *networkingv1.IngressBackend) string {
	if backend.Service != nil {
		portStr := ""
		if backend.Service.Port.Number != 0 {
			portStr = fmt.Sprintf("%d", backend.Service.Port.Number)
		} else if backend.Service.Port.Name != "" {
			portStr = backend.Service.Port.Name
		}
		return fmt.Sprintf("%s:%s", backend.Service.Name, portStr)
	}
	return "<none>"
}

// Helper functions for Row() which operates on *unstructured.Unstructured.

func extractHosts(obj *unstructured.Unstructured) string {
	rules, found, _ := unstructured.NestedSlice(obj.Object, "spec", "rules")
	if !found || len(rules) == 0 {
		return "*"
	}

	var hosts []string
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]any)
		if !ok {
			continue
		}
		host, ok := ruleMap["host"].(string)
		if ok && host != "" {
			hosts = append(hosts, host)
		}
	}

	if len(hosts) == 0 {
		return "*"
	}
	return strings.Join(hosts, ",")
}

func extractAddress(obj *unstructured.Unstructured) string {
	ingress, found, _ := unstructured.NestedSlice(obj.Object, "status", "loadBalancer", "ingress")
	if !found || len(ingress) == 0 {
		return ""
	}

	var addrs []string
	for _, entry := range ingress {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if ip, ok := entryMap["ip"].(string); ok && ip != "" {
			addrs = append(addrs, ip)
		} else if hostname, ok := entryMap["hostname"].(string); ok && hostname != "" {
			addrs = append(addrs, hostname)
		}
	}

	if len(addrs) == 0 {
		return ""
	}
	return strings.Join(addrs, ",")
}

func extractPorts(obj *unstructured.Unstructured) string {
	val, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", "tls")
	if found {
		if tls, ok := val.([]any); ok && len(tls) > 0 {
			return "80,443"
		}
	}
	return "80"
}
