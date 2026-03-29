package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChartRefLookup(t *testing.T) {
	cfg := &Config{
		Charts: map[string]map[string]string{
			"production": {
				"my-app": "oci://ghcr.io/org/my-app",
				"legacy": "/home/user/charts/legacy",
			},
		},
	}

	if ref := cfg.ChartRef("production", "my-app"); ref != "oci://ghcr.io/org/my-app" {
		t.Fatalf("expected OCI ref, got %q", ref)
	}
	if ref := cfg.ChartRef("production", "legacy"); ref != "/home/user/charts/legacy" {
		t.Fatalf("expected local path, got %q", ref)
	}
	if ref := cfg.ChartRef("production", "unknown"); ref != "" {
		t.Fatalf("expected empty for unknown release, got %q", ref)
	}
	if ref := cfg.ChartRef("staging", "my-app"); ref != "" {
		t.Fatalf("expected empty for unknown namespace, got %q", ref)
	}
}

func TestChartRefNilCharts(t *testing.T) {
	cfg := &Config{}
	if ref := cfg.ChartRef("any", "release"); ref != "" {
		t.Fatalf("expected empty for nil charts, got %q", ref)
	}
}

func TestSetChartRef(t *testing.T) {
	cfg := &Config{}
	cfg.SetChartRef("production", "my-app", "oci://ghcr.io/org/my-app")
	if ref := cfg.ChartRef("production", "my-app"); ref != "oci://ghcr.io/org/my-app" {
		t.Fatalf("expected OCI ref after set, got %q", ref)
	}
}

func TestDebugCommand(t *testing.T) {
	c := &Config{}
	if cmd := c.DebugCommand(); len(cmd) != 1 || cmd[0] != "sh" {
		t.Fatalf("default should be [sh], got %v", cmd)
	}
	c.Debug.Command = []string{"bash", "-l"}
	cmd := c.DebugCommand()
	if len(cmd) != 2 || cmd[0] != "bash" || cmd[1] != "-l" {
		t.Fatalf("custom command not returned: %v", cmd)
	}
}

func TestExecCommandDefault(t *testing.T) {
	c := &Config{}
	cmd := c.ExecCommand()
	expected := []string{"sh", "-c", "clear; (bash || ash || sh)"}
	if len(cmd) != len(expected) {
		t.Fatalf("default exec command length mismatch: got %v, want %v", cmd, expected)
	}
	for i := range expected {
		if cmd[i] != expected[i] {
			t.Fatalf("default exec command[%d] = %q, want %q", i, cmd[i], expected[i])
		}
	}
}

func TestExecCommandCustom(t *testing.T) {
	c := &Config{}
	c.Exec.Command = []string{"/bin/bash", "-l"}
	cmd := c.ExecCommand()
	if len(cmd) != 2 || cmd[0] != "/bin/bash" || cmd[1] != "-l" {
		t.Fatalf("custom exec command not returned: got %v", cmd)
	}
}

func TestAPITimeoutDefault(t *testing.T) {
	c := &Config{}
	if got := c.APITimeout(); got != 5*time.Second {
		t.Fatalf("default API timeout should be 5s, got %v", got)
	}
}

func TestAPITimeoutCustom(t *testing.T) {
	c := &Config{}
	c.API.TimeoutSeconds = 10
	if got := c.APITimeout(); got != 10*time.Second {
		t.Fatalf("custom API timeout should be 10s, got %v", got)
	}
}

func TestAPITimeoutNegative(t *testing.T) {
	c := &Config{}
	c.API.TimeoutSeconds = -5
	if got := c.APITimeout(); got != 5*time.Second {
		t.Fatalf("negative API timeout should fall back to default 5s, got %v", got)
	}
}

func TestHeartbeatIntervalDefault(t *testing.T) {
	c := &Config{}
	if got := c.HeartbeatInterval(); got != 5*time.Second {
		t.Fatalf("default heartbeat interval should be 5s, got %v", got)
	}
}

func TestHeartbeatIntervalCustom(t *testing.T) {
	c := &Config{}
	c.API.HeartbeatSeconds = 10
	if got := c.HeartbeatInterval(); got != 10*time.Second {
		t.Fatalf("custom heartbeat interval should be 10s, got %v", got)
	}
}

func TestHeartbeatIntervalNegative(t *testing.T) {
	c := &Config{}
	c.API.HeartbeatSeconds = -1
	if got := c.HeartbeatInterval(); got != 5*time.Second {
		t.Fatalf("negative heartbeat interval should fall back to default 5s, got %v", got)
	}
}

func TestLoadConfigWithCharts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("charts:\n  prod:\n    my-app: oci://ghcr.io/org/my-app\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if ref := cfg.ChartRef("prod", "my-app"); ref != "oci://ghcr.io/org/my-app" {
		t.Fatalf("expected OCI ref from loaded config, got %q", ref)
	}
}
