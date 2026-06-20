package plugin

import "testing"

func TestWorkloadHealth(t *testing.T) {
	tests := []struct {
		name    string
		ready   int32
		desired int32
		want    Health
	}{
		{"desired zero", 0, 0, Healthy},
		{"ready equals desired", 3, 3, Healthy},
		{"ready below desired", 1, 3, Warning},
		{"ready above desired", 4, 3, Healthy},
		{"desired zero with ready", 2, 0, Healthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := WorkloadHealth(tt.ready, tt.desired); got != tt.want {
				t.Errorf("WorkloadHealth(%d, %d) = %v, want %v", tt.ready, tt.desired, got, tt.want)
			}
		})
	}
}

func TestIsFailedPhase(t *testing.T) {
	tests := []struct {
		phase string
		want  bool
	}{
		{"Failed", true},
		{"CrashLoopBackOff", true},
		{"Error", true},
		{"ImagePullBackOff", true},
		{"ErrImagePull", true},
		{"CreateContainerError", true},
		{"CreateContainerConfigError", true},
		{"InvalidImageName", true},
		{"RunContainerError", true},
		{"OOMKilled", true},
		{"Terminating", true},
		{"Init:0/2", true},
		{"Signal:9", true},
		{"ExitCode:1", true},
		{"Running", false},
		{"Pending", false},
		{"Completed", false},
	}
	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			if got := IsFailedPhase(tt.phase); got != tt.want {
				t.Errorf("IsFailedPhase(%q) = %v, want %v", tt.phase, got, tt.want)
			}
		})
	}
}

func TestIsPendingPhase(t *testing.T) {
	tests := []struct {
		phase string
		want  bool
	}{
		{"Pending", true},
		{"Waiting", true},
		{"ContainerCreating", true},
		{"NotReady", true},
		{"Running", false},
		{"CrashLoopBackOff", false},
		{"Completed", false},
	}
	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			if got := IsPendingPhase(tt.phase); got != tt.want {
				t.Errorf("IsPendingPhase(%q) = %v, want %v", tt.phase, got, tt.want)
			}
		})
	}
}
