package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Toast level int values, matching notify.Level's iota order. config_test must
// not import internal/notify (it would create an import cycle), so the tests
// reference the exported config.ToastLevel* constants directly (in-package,
// hence unqualified) rather than redefining the magic numbers.
const (
	testLevelInfo    = ToastLevelInfo
	testLevelWarning = ToastLevelWarning
	testLevelError   = ToastLevelError
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

func TestLoadConfigWithMouseEnabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("mouse:\n  enabled: true\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Mouse.Enabled {
		t.Fatalf("expected cfg.Mouse.Enabled to be true, got false")
	}
}

func TestLoadConfigWithMouseExplicitlyDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("mouse:\n  enabled: false\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mouse.Enabled {
		t.Fatalf("expected cfg.Mouse.Enabled to be false after explicit 'mouse.enabled: false'")
	}
}

// TestDefaultConfigMouseDisabled verifies that mouse support is opt-in: the
// zero-value Config returned by DefaultConfig has Mouse.Enabled=false without
// touching the filesystem. This is the contract LoadConfig falls back to when
// the mouse key is absent from the YAML, so a single assertion on
// DefaultConfig replaces the redundant file-I/O flavor of this test.
func TestDefaultConfigMouseDisabled(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Mouse.Enabled {
		t.Fatalf("expected DefaultConfig().Mouse.Enabled to be false, got true")
	}
}

func TestLoadConfigWithContextDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("contexts:\n  directories:\n    - ~/.kube/configs\n    - /work/kubeconfigs\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.ContextDirectories()
	want := []string{"~/.kube/configs", "/work/kubeconfigs"}
	if len(got) != len(want) {
		t.Fatalf("expected %d directories, got %v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ContextDirectories()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestContextDirectoriesEmpty(t *testing.T) {
	cfg := &Config{}
	if got := cfg.ContextDirectories(); got != nil {
		t.Fatalf("expected nil for absent contexts config, got %v", got)
	}
}

func TestContextDirectoriesNilConfig(t *testing.T) {
	var cfg *Config
	if got := cfg.ContextDirectories(); got != nil {
		t.Fatalf("expected nil for nil config, got %v", got)
	}
}

func TestTerminalPrefixDefault(t *testing.T) {
	c := &Config{}
	if got := c.TerminalPrefix(); got != "ctrl+a" {
		t.Fatalf("default terminal prefix should be ctrl+a, got %q", got)
	}
}

func TestTerminalPrefixCustom(t *testing.T) {
	c := &Config{}
	c.Terminal.Prefix = "ctrl+b"
	if got := c.TerminalPrefix(); got != "ctrl+b" {
		t.Fatalf("custom terminal prefix should be ctrl+b, got %q", got)
	}
}

func TestTerminalScrollbackDefault(t *testing.T) {
	c := &Config{}
	if got := c.TerminalScrollback(); got != 5000 {
		t.Fatalf("default terminal scrollback should be 5000, got %d", got)
	}
}

func TestTerminalScrollbackCustom(t *testing.T) {
	c := &Config{}
	c.Terminal.Scrollback = 200
	if got := c.TerminalScrollback(); got != 200 {
		t.Fatalf("custom terminal scrollback should be 200, got %d", got)
	}
}

func TestTerminalScrollbackNonPositive(t *testing.T) {
	c := &Config{}
	c.Terminal.Scrollback = -10
	if got := c.TerminalScrollback(); got != 5000 {
		t.Fatalf("negative terminal scrollback should fall back to 5000, got %d", got)
	}
}

func TestLoadConfigWithTerminal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("terminal:\n  prefix: ctrl+b\n  scrollback: 1234\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.TerminalPrefix(); got != "ctrl+b" {
		t.Fatalf("loaded terminal prefix = %q, want ctrl+b", got)
	}
	if got := cfg.TerminalScrollback(); got != 1234 {
		t.Fatalf("loaded terminal scrollback = %d, want 1234", got)
	}
}

func TestLoadConfigTerminalOmittedUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// A config without a terminal block must yield the defaults.
	data := []byte("mouse:\n  enabled: true\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.TerminalPrefix(); got != "ctrl+a" {
		t.Fatalf("omitted terminal prefix should default to ctrl+a, got %q", got)
	}
	if got := cfg.TerminalScrollback(); got != 5000 {
		t.Fatalf("omitted terminal scrollback should default to 5000, got %d", got)
	}
}

func TestLoadConfigWithTheme(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("theme: midnight-commander\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "midnight-commander" {
		t.Fatalf("loaded theme = %q, want midnight-commander", cfg.Theme)
	}
}

func TestLoadConfigThemeOmitted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	// A config without a theme key must yield an empty Theme (backward compatible).
	data := []byte("mouse:\n  enabled: true\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Theme != "" {
		t.Fatalf("omitted theme should yield empty string, got %q", cfg.Theme)
	}
}

func TestNotifyDefaults(t *testing.T) {
	c := &Config{}
	if got := c.NotifyBufferSize(); got != 1000 {
		t.Fatalf("default NotifyBufferSize should be 1000, got %d", got)
	}
	if got := c.NotifyMaxVisible(); got != 5 {
		t.Fatalf("default NotifyMaxVisible should be 5, got %d", got)
	}
	if got := c.ToastTTL(testLevelInfo); got != 3*time.Second {
		t.Fatalf("default info TTL should be 3s, got %v", got)
	}
	if got := c.ToastTTL(testLevelWarning); got != 5*time.Second {
		t.Fatalf("default warning TTL should be 5s, got %v", got)
	}
	if got := c.ToastTTL(testLevelError); got != 8*time.Second {
		t.Fatalf("default error TTL should be 8s, got %v", got)
	}
}

func TestNotifyOverrides(t *testing.T) {
	c := &Config{}
	c.Notifications.BufferSize = 50
	c.Notifications.MaxVisible = 2
	c.Notifications.TimeoutInfo = 1
	c.Notifications.TimeoutWarning = 2
	c.Notifications.TimeoutError = 4
	if got := c.NotifyBufferSize(); got != 50 {
		t.Fatalf("custom NotifyBufferSize should be 50, got %d", got)
	}
	if got := c.NotifyMaxVisible(); got != 2 {
		t.Fatalf("custom NotifyMaxVisible should be 2, got %d", got)
	}
	if got := c.ToastTTL(testLevelInfo); got != 1*time.Second {
		t.Fatalf("custom info TTL should be 1s, got %v", got)
	}
	if got := c.ToastTTL(testLevelWarning); got != 2*time.Second {
		t.Fatalf("custom warning TTL should be 2s, got %v", got)
	}
	if got := c.ToastTTL(testLevelError); got != 4*time.Second {
		t.Fatalf("custom error TTL should be 4s, got %v", got)
	}
}

func TestNotifyNonPositiveBufferAndVisible(t *testing.T) {
	c := &Config{}
	c.Notifications.BufferSize = -5
	c.Notifications.MaxVisible = -1
	if got := c.NotifyBufferSize(); got != 1000 {
		t.Fatalf("negative NotifyBufferSize should fall back to 1000, got %d", got)
	}
	if got := c.NotifyMaxVisible(); got != 5 {
		t.Fatalf("negative NotifyMaxVisible should fall back to 5, got %d", got)
	}
}

// TestToastTTLSticky documents the sticky semantics: a NEGATIVE timeout (e.g.
// -1) yields a zero Duration, which notify.Store.Live treats as never
// auto-hiding. A configured 0 is indistinguishable from unset (omitempty) and so
// maps to the per-level default, not sticky.
func TestToastTTLSticky(t *testing.T) {
	c := &Config{}
	c.Notifications.TimeoutError = -1
	if got := c.ToastTTL(testLevelError); got != 0 {
		t.Fatalf("TimeoutError=-1 should yield 0 (sticky), got %v", got)
	}

	// A zero (== unset under omitempty) falls back to the default, not sticky.
	c.Notifications.TimeoutError = 0
	if got := c.ToastTTL(testLevelError); got != 8*time.Second {
		t.Fatalf("TimeoutError=0 should fall back to default 8s, got %v", got)
	}
}

func TestLoadConfigWithNotifications(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("notifications:\n  buffer_size: 250\n  max_visible: 3\n  timeout_info: 2\n  timeout_warning: 6\n  timeout_error: -1\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.NotifyBufferSize(); got != 250 {
		t.Fatalf("loaded NotifyBufferSize = %d, want 250", got)
	}
	if got := cfg.NotifyMaxVisible(); got != 3 {
		t.Fatalf("loaded NotifyMaxVisible = %d, want 3", got)
	}
	if got := cfg.ToastTTL(testLevelInfo); got != 2*time.Second {
		t.Fatalf("loaded info TTL = %v, want 2s", got)
	}
	if got := cfg.ToastTTL(testLevelWarning); got != 6*time.Second {
		t.Fatalf("loaded warning TTL = %v, want 6s", got)
	}
	if got := cfg.ToastTTL(testLevelError); got != 0 {
		t.Fatalf("loaded error TTL = %v, want 0 (sticky)", got)
	}
}

func TestLoadConfigNotificationsOmittedUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("mouse:\n  enabled: true\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.NotifyBufferSize(); got != 1000 {
		t.Fatalf("omitted NotifyBufferSize should default to 1000, got %d", got)
	}
	if got := cfg.NotifyMaxVisible(); got != 5 {
		t.Fatalf("omitted NotifyMaxVisible should default to 5, got %d", got)
	}
	if got := cfg.ToastTTL(testLevelInfo); got != 3*time.Second {
		t.Fatalf("omitted info TTL should default to 3s, got %v", got)
	}
	if got := cfg.ToastTTL(testLevelError); got != 8*time.Second {
		t.Fatalf("omitted error TTL should default to 8s, got %v", got)
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
