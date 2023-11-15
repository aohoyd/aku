package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}

// Plugin implements plugin.ResourcePlugin for Kubernetes Services.
type Plugin struct{}

// New creates a new Service plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "services" }
func (p *Plugin) ShortName() string                { return "svc" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "TYPE", Width: 12},
		{Title: "CLUSTER-IP", Width: 16},
		{Title: "EXTERNAL-IP", Width: 16},
		{Title: "PORTS", Width: 16},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	svcType, _, _ := unstructured.NestedString(obj.Object, "spec", "type")
	clusterIP, _, _ := unstructured.NestedString(obj.Object, "spec", "clusterIP")
	externalIP := extractExternalIP(obj)
	ports := extractPorts(obj)
	age := render.FormatAge(obj)

	return []string{name, svcType, clusterIP, externalIP, ports, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	svc, err := toService(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("services: decode: %w", err)
	}

	b := render.NewBuilder()

	// Basic metadata
	b.KV(render.LEVEL_0, "Name", svc.Name)
	b.KV(render.LEVEL_0, "Namespace", svc.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", svc.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", svc.Annotations)

	// Selector
	if len(svc.Spec.Selector) > 0 {
		b.KVMulti(render.LEVEL_0, "Selector", svc.Spec.Selector)
	} else {
		b.KV(render.LEVEL_0, "Selector", "<none>")
	}

	// Type
	svcType := string(svc.Spec.Type)
	if svcType == "" {
		svcType = "ClusterIP"
	}
	b.KV(render.LEVEL_0, "Type", svcType)

	// IP / ClusterIP
	b.KV(render.LEVEL_0, "IP", svc.Spec.ClusterIP)

	// IPs (if present)
	if len(svc.Spec.ClusterIPs) > 0 {
		b.KV(render.LEVEL_0, "IPs", strings.Join(svc.Spec.ClusterIPs, ","))
	}

	// External IPs
	if len(svc.Spec.ExternalIPs) > 0 {
		b.KV(render.LEVEL_0, "External IPs", strings.Join(svc.Spec.ExternalIPs, ","))
	}

	// LoadBalancer Ingress
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		if len(svc.Status.LoadBalancer.Ingress) > 0 {
			var lbAddrs []string
			for _, ing := range svc.Status.LoadBalancer.Ingress {
				if ing.IP != "" {
					lbAddrs = append(lbAddrs, ing.IP)
				} else if ing.Hostname != "" {
					lbAddrs = append(lbAddrs, ing.Hostname)
				}
			}
			if len(lbAddrs) > 0 {
				b.KV(render.LEVEL_0, "LoadBalancer Ingress", strings.Join(lbAddrs, ", "))
			}
		}
	}

	// Ports
	for _, port := range svc.Spec.Ports {
		protocol := string(port.Protocol)
		if protocol == "" {
			protocol = "TCP"
		}

		portValue := fmt.Sprintf("%d/%s", port.Port, protocol)
		if port.Name != "" {
			portValue = fmt.Sprintf("%s %d/%s", port.Name, port.Port, protocol)
		}
		b.KV(render.LEVEL_0, "Port", portValue)

		// TargetPort
		targetPort := port.TargetPort.String()
		if targetPort != "" && targetPort != "0" {
			b.KV(render.LEVEL_0, "TargetPort", targetPort)
		}

		// NodePort (only if > 0)
		if port.NodePort > 0 {
			npValue := fmt.Sprintf("%d/%s", port.NodePort, protocol)
			if port.Name != "" {
				npValue = fmt.Sprintf("%s %d/%s", port.Name, port.NodePort, protocol)
			}
			b.KV(render.LEVEL_0, "NodePort", npValue)
		}
	}

	// Session Affinity
	sessionAffinity := string(svc.Spec.SessionAffinity)
	if sessionAffinity == "" {
		sessionAffinity = "None"
	}
	b.KV(render.LEVEL_0, "Session Affinity", sessionAffinity)

	// External Traffic Policy
	if svc.Spec.ExternalTrafficPolicy != "" {
		b.KV(render.LEVEL_0, "External Traffic Policy", string(svc.Spec.ExternalTrafficPolicy))
	}

	// Internal Traffic Policy
	if svc.Spec.InternalTrafficPolicy != nil && *svc.Spec.InternalTrafficPolicy != "" {
		b.KV(render.LEVEL_0, "Internal Traffic Policy", string(*svc.Spec.InternalTrafficPolicy))
	}

	return b.Build(), nil
}

// toService converts an unstructured object to a typed corev1.Service.
func toService(obj *unstructured.Unstructured) (*corev1.Service, error) {
	var svc corev1.Service
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &svc); err != nil {
		return nil, err
	}
	return &svc, nil
}

func extractExternalIP(obj *unstructured.Unstructured) string {
	ingress, found, _ := unstructured.NestedSlice(obj.Object, "status", "loadBalancer", "ingress")
	if !found || len(ingress) == 0 {
		return "<none>"
	}

	var ips []string
	for _, entry := range ingress {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if ip, ok := entryMap["ip"].(string); ok && ip != "" {
			ips = append(ips, ip)
		} else if hostname, ok := entryMap["hostname"].(string); ok && hostname != "" {
			ips = append(ips, hostname)
		}
	}

	if len(ips) == 0 {
		return "<none>"
	}
	return strings.Join(ips, ",")
}

func extractPorts(obj *unstructured.Unstructured) string {
	ports, found, _ := unstructured.NestedSlice(obj.Object, "spec", "ports")
	if !found || len(ports) == 0 {
		return "<none>"
	}

	var parts []string
	for _, p := range ports {
		pMap, ok := p.(map[string]any)
		if !ok {
			continue
		}
		port, _, _ := unstructured.NestedInt64(pMap, "port")
		protocol, _, _ := unstructured.NestedString(pMap, "protocol")
		if protocol == "" {
			protocol = "TCP"
		}
		parts = append(parts, fmt.Sprintf("%d/%s", port, protocol))
	}

	return strings.Join(parts, ",")
}
