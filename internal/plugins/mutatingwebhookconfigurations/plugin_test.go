package mutatingwebhookconfigurations

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestMutatingWebhookConfigurationPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
}

func TestMutatingWebhookConfigurationPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "MutatingWebhookConfiguration",
			"metadata": map[string]any{
				"name":              "test-mwc",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"webhooks": []any{
				map[string]any{
					"name":          "webhook1.example.com",
					"failurePolicy": "Fail",
				},
				map[string]any{
					"name":          "webhook2.example.com",
					"failurePolicy": "Ignore",
				},
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "test-mwc" {
		t.Fatalf("expected name 'test-mwc', got '%s'", row[0])
	}
	if row[1] != "2" {
		t.Fatalf("expected webhooks '2', got '%s'", row[1])
	}
	if row[2] != "Fail" {
		t.Fatalf("expected failure policy 'Fail', got '%s'", row[2])
	}
	if row[3] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestMutatingWebhookConfigurationPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "MutatingWebhookConfiguration",
			"metadata": map[string]any{
				"name":              "test-mwc",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "webhook"},
				"annotations":       map[string]any{"note": "test"},
			},
			"webhooks": []any{
				map[string]any{
					"name":                    "webhook1.example.com",
					"failurePolicy":           "Fail",
					"matchPolicy":             "Equivalent",
					"sideEffects":             "None",
					"timeoutSeconds":          int64(10),
					"reinvocationPolicy":      "Never",
					"admissionReviewVersions": []any{"v1", "v1beta1"},
					"clientConfig": map[string]any{
						"service": map[string]any{
							"namespace": "webhook-ns",
							"name":      "webhook-svc",
							"path":      "/validate",
							"port":      int64(443),
						},
						"caBundle": "dGVzdA==",
					},
					"namespaceSelector": map[string]any{
						"matchLabels": map[string]any{"env": "production"},
					},
					"rules": []any{
						map[string]any{
							"apiGroups":   []any{"apps"},
							"apiVersions": []any{"v1"},
							"operations":  []any{"CREATE", "UPDATE"},
							"resources":   []any{"deployments"},
						},
					},
					"matchConditions": []any{
						map[string]any{
							"name":       "exclude-leases",
							"expression": "!(request.resource.group == 'coordination.k8s.io')",
						},
					},
				},
			},
		},
	}

	c, err := p.Describe(t.Context(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Display == "" {
		t.Fatal("display output should not be empty")
	}
	if stripped := ansi.Strip(c.Display); stripped != c.Raw {
		t.Errorf("strip invariant violated: ansi.Strip(c.Display) != raw\nstripped: %q\nraw:      %q", stripped, c.Raw)
	}

	checks := []string{
		"test-mwc",
		"webhook1.example.com",
		"Fail",
		"Equivalent",
		"None",
		"10s",
		"Never",
		"v1, v1beta1",
		"webhook-ns",
		"webhook-svc",
		"/validate",
		"443",
		"production",
		"CREATE, UPDATE",
		"deployments",
		"exclude-leases",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
