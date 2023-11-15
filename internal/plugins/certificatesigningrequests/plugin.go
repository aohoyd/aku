package certificatesigningrequests

import (
	"context"
	"fmt"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "certificates.k8s.io", Version: "v1", Resource: "certificatesigningrequests"}

// Plugin implements plugin.ResourcePlugin for Kubernetes CertificateSigningRequests.
type Plugin struct{}

// New creates a new CertificateSigningRequests plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "certificatesigningrequests" }
func (p *Plugin) ShortName() string                { return "csr" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "SIGNER", Flex: true},
		{Title: "REQUESTOR", Flex: true},
		{Title: "CONDITION", Width: 12},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	signerName, _, _ := unstructured.NestedString(obj.Object, "spec", "signerName")
	username, _, _ := unstructured.NestedString(obj.Object, "spec", "username")
	condition := extractLastCondition(obj)
	age := render.FormatAge(obj)

	return []string{name, signerName, username, condition, age}
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	csr, err := toCSR(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to CertificateSigningRequest: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", csr.Name)
	b.KVMulti(render.LEVEL_0, "Labels", csr.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", csr.Annotations)
	b.KV(render.LEVEL_0, "SignerName", csr.Spec.SignerName)
	b.KV(render.LEVEL_0, "Username", csr.Spec.Username)

	// Usages
	if len(csr.Spec.Usages) > 0 {
		usages := make([]string, len(csr.Spec.Usages))
		for i, u := range csr.Spec.Usages {
			usages[i] = string(u)
		}
		b.KV(render.LEVEL_0, "Usages", strings.Join(usages, ", "))
	} else {
		b.KV(render.LEVEL_0, "Usages", "<none>")
	}

	// ExpirationSeconds
	if csr.Spec.ExpirationSeconds != nil {
		b.KV(render.LEVEL_0, "ExpirationSeconds", fmt.Sprintf("%d", *csr.Spec.ExpirationSeconds))
	}

	// Conditions
	if len(csr.Status.Conditions) > 0 {
		b.Section(render.LEVEL_0, "Conditions")
		for _, cond := range csr.Status.Conditions {
			b.KVStyled(render.LEVEL_1, render.ConditionKind(string(cond.Status)), string(cond.Type), string(cond.Status))
			if cond.Reason != "" {
				b.KV(render.LEVEL_2, "Reason", cond.Reason)
			}
			if cond.Message != "" {
				b.KV(render.LEVEL_2, "Message", cond.Message)
			}
			if !cond.LastUpdateTime.IsZero() {
				b.KV(render.LEVEL_2, "LastUpdateTime", cond.LastUpdateTime.Format("2006-01-02T15:04:05Z"))
			}
		}
	}

	return b.Build(), nil
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

// toCSR converts an unstructured object to a typed certificatesv1.CertificateSigningRequest.
func toCSR(obj *unstructured.Unstructured) (*certificatesv1.CertificateSigningRequest, error) {
	var csr certificatesv1.CertificateSigningRequest
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &csr); err != nil {
		return nil, err
	}
	return &csr, nil
}

// extractLastCondition returns the type of the last condition, or "<none>" if there are no conditions.
func extractLastCondition(obj *unstructured.Unstructured) string {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found || len(conditions) == 0 {
		return "<none>"
	}

	last, ok := conditions[len(conditions)-1].(map[string]any)
	if !ok {
		return "<none>"
	}

	condType, _, _ := unstructured.NestedString(last, "type")
	if condType == "" {
		return "<none>"
	}
	return condType
}
