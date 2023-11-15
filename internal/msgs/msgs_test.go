package msgs

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestErrorMsgErrorString(t *testing.T) {
	msg := ErrMsg{Err: fmt.Errorf("test error")}
	if msg.Error() != "test error" {
		t.Fatalf("expected 'test error', got '%s'", msg.Error())
	}
}

func TestDetailModeConstants(t *testing.T) {
	if DetailYAML != 0 || DetailDescribe != 1 || DetailLogs != 2 {
		t.Fatal("DetailMode constants have unexpected values")
	}
}

func TestSearchModeConstants(t *testing.T) {
	if SearchModeSearch != 0 {
		t.Fatal("SearchModeSearch should be 0")
	}
	if SearchModeFilter != 1 {
		t.Fatal("SearchModeFilter should be 1")
	}
}

func TestSearchSubmittedMsg(t *testing.T) {
	msg := SearchSubmittedMsg{Pattern: "test", Mode: SearchModeSearch}
	if msg.Pattern != "test" {
		t.Fatal("Pattern should be 'test'")
	}
}

func TestSetImageRequestedMsg(t *testing.T) {
	msg := SetImageRequestedMsg{
		ResourceName: "nginx",
		Namespace:    "default",
		PluginName:   "deployments",
		Images: []ContainerImageChange{
			{Name: "nginx", Image: "nginx:1.26", Init: false},
			{Name: "init-db", Image: "busybox:latest", Init: true},
		},
	}
	if len(msg.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(msg.Images))
	}
	if msg.Images[1].Init != true {
		t.Fatal("expected init container to be marked as Init")
	}
}

func TestAllMsgTypes(t *testing.T) {
	// Verify all message types can be used as tea.Msg
	messages := []tea.Msg{
		NamespaceSelectedMsg{Namespace: "default"},
		ActionResultMsg{ActionID: "delete"},
		ResourcePickedMsg{Command: "ns kube-system"},
		ConfirmResultMsg{Action: ConfirmYes},
		ErrMsg{Err: fmt.Errorf("err")},
		SetImageRequestedMsg{},
		LogLineMsg{Line: "test", Gen: 1},
		LogStreamEndedMsg{Gen: 1},
	}
	for i, msg := range messages {
		if msg == nil {
			t.Fatalf("message %d should not be nil", i)
		}
	}
}
