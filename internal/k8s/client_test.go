package k8s

import "testing"

func TestNewClientErrorsOnInvalidKubeconfig(t *testing.T) {
	_, err := NewClient("/nonexistent/kubeconfig", "", "")
	if err == nil {
		t.Fatal("expected error for nonexistent kubeconfig")
	}
}

func TestClientWithNamespace(t *testing.T) {
	wh := &warningHandler{}
	c := &Client{Namespace: "default", Context: "test", WarningHandler: wh}
	updated := c.WithNamespace("kube-system")
	if updated.Namespace != "kube-system" {
		t.Fatalf("expected 'kube-system', got '%s'", updated.Namespace)
	}
	if c.Namespace != "default" {
		t.Fatal("original client namespace should be unchanged")
	}
	if updated.Context != "test" {
		t.Fatal("context should be preserved")
	}
	if updated.WarningHandler != wh {
		t.Fatal("WarningHandler should be preserved")
	}
}
