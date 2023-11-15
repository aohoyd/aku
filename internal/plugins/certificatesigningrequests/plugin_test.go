package certificatesigningrequests

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCSRPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
}

func TestCSRPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makeCSR("my-csr", "kubernetes.io/kube-apiserver-client", "system:admin", "Approved")
	row := p.Row(obj)

	if row[0] != "my-csr" {
		t.Fatalf("expected name 'my-csr', got '%s'", row[0])
	}
	if row[1] != "kubernetes.io/kube-apiserver-client" {
		t.Fatalf("expected signerName 'kubernetes.io/kube-apiserver-client', got '%s'", row[1])
	}
	if row[2] != "system:admin" {
		t.Fatalf("expected requestor 'system:admin', got '%s'", row[2])
	}
	if row[3] != "Approved" {
		t.Fatalf("expected condition 'Approved', got '%s'", row[3])
	}
}

func TestCSRPluginRowNoCondition(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "certificates.k8s.io/v1",
			"kind":       "CertificateSigningRequest",
			"metadata": map[string]any{
				"name":              "no-cond-csr",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"signerName": "example.com/signer",
				"username":   "user1",
			},
		},
	}
	row := p.Row(obj)
	if row[3] != "<none>" {
		t.Fatalf("expected condition '<none>', got '%s'", row[3])
	}
}

func TestCSRPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "certificates.k8s.io/v1",
			"kind":       "CertificateSigningRequest",
			"metadata": map[string]any{
				"name":              "test-csr",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":           map[string]any{"app": "test"},
			},
			"spec": map[string]any{
				"signerName":        "kubernetes.io/kube-apiserver-client",
				"username":          "system:admin",
				"usages":            []any{"digital signature", "key encipherment"},
				"expirationSeconds": int64(3600),
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":           "Approved",
						"status":         "True",
						"reason":         "AutoApproved",
						"message":        "Auto approved by controller",
						"lastUpdateTime": "2026-02-24T10:01:00Z",
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
		"test-csr",
		"kubernetes.io/kube-apiserver-client",
		"system:admin",
		"digital signature, key encipherment",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func makeCSR(name, signerName, username, condition string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "certificates.k8s.io/v1",
			"kind":       "CertificateSigningRequest",
			"metadata": map[string]any{
				"name":              name,
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"signerName": signerName,
				"username":   username,
			},
		},
	}

	if condition != "" {
		obj.Object["status"] = map[string]any{
			"conditions": []any{
				map[string]any{
					"type":           condition,
					"status":         "True",
					"lastUpdateTime": "2024-01-01T00:01:00Z",
				},
			},
		}
	}

	return obj
}
