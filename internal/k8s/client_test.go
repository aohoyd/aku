package k8s

import (
	"context"
	"testing"

	"k8s.io/client-go/kubernetes/fake"
)

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

func TestCheckHealthSuccess(t *testing.T) {
	c := &Client{Typed: fake.NewSimpleClientset()}
	if !CheckHealth(context.Background(), c) {
		t.Fatal("expected CheckHealth to return true with fake clientset")
	}
}

func TestCheckHealthNilClient(t *testing.T) {
	if CheckHealth(context.Background(), nil) {
		t.Fatal("expected CheckHealth to return false for nil client")
	}
}

func TestCheckHealthNilTyped(t *testing.T) {
	c := &Client{Typed: nil}
	if CheckHealth(context.Background(), c) {
		t.Fatal("expected CheckHealth to return false when Typed is nil")
	}
}
