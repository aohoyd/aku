package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestNsPickerSetNamespaces(t *testing.T) {
	np := NewNsPicker(40, 20)
	np.SetNamespaces([]string{"default", "kube-system", "production"})
	np.Open()
	if !np.Active() {
		t.Fatal("ns picker should be active after Open")
	}
}

func TestNsPickerSelectsNamespace(t *testing.T) {
	np := NewNsPicker(40, 20)
	np.SetNamespaces([]string{"default", "kube-system"})
	np.Open()
	updated, cmd := np.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.Active() {
		t.Fatal("ns picker should close after selection")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
}

func TestNsPickerEscCancels(t *testing.T) {
	np := NewNsPicker(40, 20)
	np.SetNamespaces([]string{"default", "kube-system"})
	np.Open()
	updated, _ := np.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.Active() {
		t.Fatal("ns picker should close after Esc")
	}
}

func TestNsPickerAllNamespacesEntry(t *testing.T) {
	np := NewNsPicker(40, 20)
	np.SetNamespaces([]string{"default", "kube-system"})
	np.Open()

	filtered := np.Filtered()
	if len(filtered) != 3 {
		t.Fatalf("expected 3 items (All Namespaces + 2 real), got %d", len(filtered))
	}
	if filtered[0] != "All Namespaces" {
		t.Fatalf("expected first item 'All Namespaces', got %q", filtered[0])
	}
}

func TestNsPickerSelectAllNamespacesEmitsEmpty(t *testing.T) {
	np := NewNsPicker(40, 20)
	np.SetNamespaces([]string{"default", "kube-system"})
	np.Open()

	updated, cmd := np.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.Active() {
		t.Fatal("picker should close after selection")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	nsMsg, ok := msg.(msgs.NamespaceSelectedMsg)
	if !ok {
		t.Fatalf("expected NamespaceSelectedMsg, got %T", msg)
	}
	if nsMsg.Namespace != "" {
		t.Fatalf("expected empty string for all-namespaces, got %q", nsMsg.Namespace)
	}
}

func TestNsPickerFilterExcludesAllNamespaces(t *testing.T) {
	np := NewNsPicker(40, 20)
	np.SetNamespaces([]string{"default", "kube-system"})
	np.Open()

	np, _ = np.Update(tea.KeyPressMsg{Code: -1, Text: "k"})
	np, _ = np.Update(tea.KeyPressMsg{Code: -1, Text: "u"})
	np, _ = np.Update(tea.KeyPressMsg{Code: -1, Text: "b"})
	np, _ = np.Update(tea.KeyPressMsg{Code: -1, Text: "e"})

	filtered := np.Filtered()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered item, got %d: %v", len(filtered), filtered)
	}
	if filtered[0] != "kube-system" {
		t.Fatalf("expected 'kube-system', got %q", filtered[0])
	}
}

func TestNsPickerFixedHeight(t *testing.T) {
	np := NewNsPicker(50, 20)
	np.SetNamespaces([]string{"default", "kube-system", "production", "staging"})
	np.Open()

	fullView := np.View()
	fullLines := strings.Count(fullView, "\n")

	np, _ = np.Update(tea.KeyPressMsg{Code: -1, Text: "d"})
	np, _ = np.Update(tea.KeyPressMsg{Code: -1, Text: "e"})
	np, _ = np.Update(tea.KeyPressMsg{Code: -1, Text: "f"})

	filteredView := np.View()
	filteredLines := strings.Count(filteredView, "\n")

	if fullLines != filteredLines {
		t.Fatalf("picker height should be stable: full=%d lines, filtered=%d lines", fullLines, filteredLines)
	}
}
