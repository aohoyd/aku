package render

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestValueKindConstants(t *testing.T) {
	kinds := []ValueKind{ValueDefault, ValueStatusOK, ValueStatusWarn, ValueStatusError, ValueStatusGray, ValueNumber}
	seen := make(map[ValueKind]bool)
	for _, k := range kinds {
		if seen[k] {
			t.Fatalf("duplicate ValueKind: %d", k)
		}
		seen[k] = true
	}
}

func TestStatusKind(t *testing.T) {
	tests := []struct {
		phase string
		want  ValueKind
	}{
		{"Running", ValueStatusOK},
		{"Succeeded", ValueStatusGray},
		{"Pending", ValueStatusWarn},
		{"ContainerCreating", ValueStatusWarn},
		{"Failed", ValueStatusError},
		{"CrashLoopBackOff", ValueStatusError},
		{"ImagePullBackOff", ValueStatusError},
		{"Unknown", ValueDefault},
		{"SomeOtherPhase", ValueDefault},
	}
	for _, tt := range tests {
		got := StatusKind(tt.phase)
		if got != tt.want {
			t.Errorf("StatusKind(%q) = %d, want %d", tt.phase, got, tt.want)
		}
	}
}

func TestConditionKind(t *testing.T) {
	if ConditionKind("True") != ValueStatusOK {
		t.Fatal("True should be OK")
	}
	if ConditionKind("False") != ValueStatusError {
		t.Fatal("False should be Error")
	}
	if ConditionKind("Unknown") != ValueStatusWarn {
		t.Fatal("Unknown should be Warn")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{3 * time.Hour, "3h"},
		{48 * time.Hour, "2d"},
		{0, "0s"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.d); got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatAge(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{}}
	if FormatAge(obj) != "?" {
		t.Fatal("zero timestamp should return ?")
	}
}
