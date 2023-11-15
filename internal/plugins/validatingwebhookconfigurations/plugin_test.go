package validatingwebhookconfigurations

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestValidatingWebhookConfigurationPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
}

func TestValidatingWebhookConfigurationPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "ValidatingWebhookConfiguration",
			"metadata": map[string]any{
				"name":              "test-vwc",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"webhooks": []any{
				map[string]any{
					"name":          "validate.example.com",
					"failurePolicy": "Ignore",
				},
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "test-vwc" {
		t.Fatalf("expected name 'test-vwc', got '%s'", row[0])
	}
	if row[1] != "1" {
		t.Fatalf("expected webhooks '1', got '%s'", row[1])
	}
	if row[2] != "Ignore" {
		t.Fatalf("expected failure policy 'Ignore', got '%s'", row[2])
	}
	if row[3] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestValidatingWebhookConfigurationPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "ValidatingWebhookConfiguration",
			"metadata": map[string]any{
				"name":              "test-vwc",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "webhook"},
				"annotations":       map[string]any{"note": "test"},
			},
			"webhooks": []any{
				map[string]any{
					"name":                    "validate.example.com",
					"failurePolicy":           "Ignore",
					"matchPolicy":             "Exact",
					"sideEffects":             "None",
					"timeoutSeconds":          int64(5),
					"admissionReviewVersions": []any{"v1"},
					"clientConfig": map[string]any{
						"url": "https://webhook.example.com/validate",
					},
					"objectSelector": map[string]any{
						"matchLabels": map[string]any{"validate": "true"},
					},
					"rules": []any{
						map[string]any{
							"apiGroups":   []any{""},
							"apiVersions": []any{"v1"},
							"operations":  []any{"CREATE"},
							"resources":   []any{"pods"},
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
		"test-vwc",
		"validate.example.com",
		"Ignore",
		"Exact",
		"None",
		"5s",
		"v1",
		"https://webhook.example.com/validate",
		"validate=true",
		"CREATE",
		"pods",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
