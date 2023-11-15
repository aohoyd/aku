package render

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func makeEvent(kind, name, namespace, eventType, reason, message, source, ts string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"involvedObject": map[string]interface{}{
			"kind":      kind,
			"name":      name,
			"namespace": namespace,
		},
		"type":          eventType,
		"reason":        reason,
		"message":       message,
		"lastTimestamp":  ts,
		"source":        map[string]interface{}{"component": source},
	}}
}

func TestRenderEventsMatching(t *testing.T) {
	now := time.Now()
	events := []*unstructured.Unstructured{
		makeEvent("Pod", "my-pod", "default", "Normal", "Pulled", "Image pulled", "kubelet", now.Add(-5*time.Minute).Format(time.RFC3339)),
		makeEvent("Pod", "other-pod", "default", "Normal", "Pulled", "Other image", "kubelet", now.Add(-3*time.Minute).Format(time.RFC3339)),
		makeEvent("Pod", "my-pod", "default", "Warning", "Failed", "Back-off", "kubelet", now.Add(-1*time.Minute).Format(time.RFC3339)),
	}
	content := RenderEvents(events, "Pod", "my-pod", "default")
	if !strings.Contains(content.Raw, "Events:") {
		t.Error("expected Events: section header")
	}
	if !strings.Contains(content.Raw, "Image pulled") {
		t.Error("expected matching event message")
	}
	if !strings.Contains(content.Raw, "Back-off") {
		t.Error("expected matching warning event")
	}
	if strings.Contains(content.Raw, "Other image") {
		t.Error("should not contain non-matching event")
	}
}

func TestRenderEventsNewestFirst(t *testing.T) {
	now := time.Now()
	events := []*unstructured.Unstructured{
		makeEvent("Pod", "p", "ns", "Normal", "A", "first", "kubelet", now.Add(-10*time.Minute).Format(time.RFC3339)),
		makeEvent("Pod", "p", "ns", "Normal", "B", "second", "kubelet", now.Add(-1*time.Minute).Format(time.RFC3339)),
	}
	content := RenderEvents(events, "Pod", "p", "ns")
	secondIdx := strings.Index(content.Raw, "second")
	firstIdx := strings.Index(content.Raw, "first")
	if secondIdx > firstIdx {
		t.Errorf("expected newest first: second at %d, first at %d", secondIdx, firstIdx)
	}
}

func TestRenderEventsEmpty(t *testing.T) {
	content := RenderEvents(nil, "Pod", "my-pod", "default")
	if !strings.Contains(content.Raw, "Events:") {
		t.Error("expected Events: header even when empty")
	}
	if !strings.Contains(content.Raw, "<none>") {
		t.Error("expected <none> when no matching events")
	}
}

func TestRenderEventsNoMatch(t *testing.T) {
	events := []*unstructured.Unstructured{
		makeEvent("Deployment", "my-deploy", "default", "Normal", "Scaled", "Scaled up", "controller", time.Now().Format(time.RFC3339)),
	}
	content := RenderEvents(events, "Pod", "my-pod", "default")
	if !strings.Contains(content.Raw, "<none>") {
		t.Error("expected <none> when no events match")
	}
}

func TestRenderEventsStripInvariant(t *testing.T) {
	now := time.Now()
	events := []*unstructured.Unstructured{
		makeEvent("Pod", "p", "ns", "Normal", "Pulled", "Image pulled", "kubelet", now.Format(time.RFC3339)),
		makeEvent("Pod", "p", "ns", "Warning", "Failed", "Back-off", "kubelet", now.Add(-5*time.Minute).Format(time.RFC3339)),
	}
	content := RenderEvents(events, "Pod", "p", "ns")
	stripped := ansi.Strip(content.Display)
	if stripped != content.Raw {
		t.Errorf("strip invariant broken:\nstripped: %q\nraw:      %q", stripped, content.Raw)
	}
}

func TestRenderEventsAlignment(t *testing.T) {
	now := time.Now()
	events := []*unstructured.Unstructured{
		makeEvent("Pod", "p", "ns", "Normal", "Pulled", "Image pulled", "kubelet", now.Format(time.RFC3339)),
		makeEvent("Pod", "p", "ns", "Warning", "FailedToRetrieveImagePullSecret", "secret not found", "kubelet", now.Add(-1*time.Minute).Format(time.RFC3339)),
	}
	content := RenderEvents(events, "Pod", "p", "ns")

	// Every row (header, separator, data) must have Message starting at the same column.
	var positions []int
	for _, line := range strings.Split(content.Raw, "\n") {
		if idx := strings.Index(line, "Message"); idx != -1 {
			positions = append(positions, idx)
		}
		if idx := strings.Index(line, "-------"); idx != -1 {
			positions = append(positions, idx)
		}
		if idx := strings.Index(line, "Image pulled"); idx != -1 {
			positions = append(positions, idx)
		}
		if idx := strings.Index(line, "secret not found"); idx != -1 {
			positions = append(positions, idx)
		}
	}

	if len(positions) < 4 {
		t.Fatalf("expected at least 4 message-column entries, got %d in:\n%s", len(positions), content.Raw)
	}
	for i := 1; i < len(positions); i++ {
		if positions[i] != positions[0] {
			t.Errorf("message column misaligned: line %d at col %d, expected %d\noutput:\n%s", i, positions[i], positions[0], content.Raw)
		}
	}
}

func TestRenderEventsTableColumns(t *testing.T) {
	events := []*unstructured.Unstructured{
		makeEvent("Pod", "p", "ns", "Warning", "FailedScheduling", "0/3 nodes available", "default-scheduler", time.Now().Format(time.RFC3339)),
	}
	content := RenderEvents(events, "Pod", "p", "ns")
	if !strings.Contains(content.Raw, "Type") || !strings.Contains(content.Raw, "Reason") || !strings.Contains(content.Raw, "Message") {
		t.Errorf("expected table column headers, got:\n%s", content.Raw)
	}
}
