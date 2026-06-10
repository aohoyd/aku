package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// DebugConfig holds configuration for ephemeral debug containers.
type DebugConfig struct {
	Image   string   `yaml:"image,omitempty"`
	Command []string `yaml:"command,omitempty"`
}

// ExecConfig holds configuration for exec into containers.
type ExecConfig struct {
	Command []string `yaml:"command,omitempty"`
}

// APIConfig holds configuration for Kubernetes API calls.
type APIConfig struct {
	TimeoutSeconds   int `yaml:"timeout_seconds,omitempty"`
	HeartbeatSeconds int `yaml:"heartbeat_seconds,omitempty"`
}

// LogsConfig holds configuration for log viewing.
type LogsConfig struct {
	BufferSize       int    `yaml:"buffer_size,omitempty"`
	DefaultTimeRange string `yaml:"default_time_range,omitempty"`
}

// MouseConfig holds configuration for mouse support.
type MouseConfig struct {
	Enabled bool `yaml:"enabled,omitempty"`
}

// TerminalConfig holds configuration for embedded terminal panes.
type TerminalConfig struct {
	// Prefix is the tmux-style keystroke that flips a focused terminal pane
	// from typing mode into nav mode (e.g. "ctrl+a"). Empty means the default.
	Prefix string `yaml:"prefix,omitempty"`
	// Scrollback is the number of off-screen lines the terminal emulator
	// retains for scrollback. Zero or negative means the default.
	Scrollback int `yaml:"scrollback,omitempty"`
}

// ContextsConfig holds configuration for multi-cluster context discovery.
type ContextsConfig struct {
	Directories []string `yaml:"directories,omitempty"`
}

// NotificationsConfig holds configuration for the toast overlay and the backing
// notify store.
//
// The timeout_* fields are per-level auto-hide durations IN SECONDS. Because the
// YAML tags use omitempty, a value of 0 is indistinguishable from "unset", so
// BOTH 0 and an omitted field yield the per-level DEFAULT (info 3s, warning 5s,
// error 8s) — a 0-second (instant-hide) timeout is therefore NOT configurable.
// Use a NEGATIVE value (e.g. timeout_error: -1) to make a level STICKY (never
// auto-hide). See ToastTTL for the exact mapping.
type NotificationsConfig struct {
	BufferSize     int `yaml:"buffer_size,omitempty"`
	MaxVisible     int `yaml:"max_visible,omitempty"`
	TimeoutInfo    int `yaml:"timeout_info,omitempty"`
	TimeoutWarning int `yaml:"timeout_warning,omitempty"`
	TimeoutError   int `yaml:"timeout_error,omitempty"`
}

// Config holds the application configuration.
type Config struct {
	// Theme names a color preset loaded from <config>/themes/<name>.yaml. It is
	// applied over the built-in defaults; theme.yaml (if present) is layered on
	// top and overrides it.
	Theme         string                       `yaml:"theme,omitempty"`
	Charts        map[string]map[string]string `yaml:"charts,omitempty"`
	API           APIConfig                    `yaml:"api,omitempty"`
	Debug         DebugConfig                  `yaml:"debug,omitempty"`
	Exec          ExecConfig                   `yaml:"exec,omitempty"`
	Logs          LogsConfig                   `yaml:"logs,omitempty"`
	Mouse         MouseConfig                  `yaml:"mouse,omitempty"`
	Terminal      TerminalConfig               `yaml:"terminal,omitempty"`
	Contexts      ContextsConfig               `yaml:"contexts,omitempty"`
	Notifications NotificationsConfig          `yaml:"notifications,omitempty"`
}

// Default terminal settings, returned by the accessors when the corresponding
// config field is empty/zero.
const (
	defaultTerminalPrefix     = "ctrl+a"
	defaultTerminalScrollback = 5000
)

// Default notification settings, returned by the accessors when the
// corresponding config field is empty/zero.
const (
	// defaultNotifyBufferSize: keep in sync with notify.defaultCapacity (the
	// store's own defensive fallback for non-positive capacities).
	defaultNotifyBufferSize    = 1000
	defaultNotifyMaxVisible    = 5
	defaultToastTimeoutInfo    = 3 * time.Second
	defaultToastTimeoutWarning = 5 * time.Second
	defaultToastTimeoutError   = 8 * time.Second
)

// DefaultConfig returns a config with default settings.
func DefaultConfig() *Config {
	return &Config{}
}

// LoadConfig loads config from a YAML file, falling back to defaults if not found.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// ConfigPath returns the default config file path.
func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// ChartRef returns the chart reference for a release in a namespace, or "" if not found.
func (c *Config) ChartRef(namespace, release string) string {
	if c.Charts == nil {
		return ""
	}
	nsCharts, ok := c.Charts[namespace]
	if !ok {
		return ""
	}
	return nsCharts[release]
}

// DebugImage returns the configured debug image, defaulting to "busybox:latest".
func (c *Config) DebugImage() string {
	if c.Debug.Image != "" {
		return c.Debug.Image
	}
	return "busybox:latest"
}

// DebugCommand returns the configured debug command, defaulting to ["sh"].
func (c *Config) DebugCommand() []string {
	if len(c.Debug.Command) > 0 {
		return c.Debug.Command
	}
	return []string{"sh"}
}

// ExecCommand returns the configured exec command, defaulting to ["sh", "-c", "clear; (bash || ash || sh)"].
func (c *Config) ExecCommand() []string {
	if len(c.Exec.Command) > 0 {
		return c.Exec.Command
	}
	return []string{"sh", "-c", "clear; (bash || ash || sh)"}
}

// TerminalPrefix returns the configured tmux-style terminal prefix keystroke,
// defaulting to "ctrl+a" when empty.
func (c *Config) TerminalPrefix() string {
	if c.Terminal.Prefix != "" {
		return c.Terminal.Prefix
	}
	return defaultTerminalPrefix
}

// TerminalScrollback returns the configured terminal scrollback line count,
// defaulting to 5000 when zero or negative.
func (c *Config) TerminalScrollback() int {
	if c.Terminal.Scrollback > 0 {
		return c.Terminal.Scrollback
	}
	return defaultTerminalScrollback
}

// LogDefaultTimeRange returns the configured default time range label, defaulting to "15m".
func (c *Config) LogDefaultTimeRange() string {
	if c.Logs.DefaultTimeRange != "" {
		return c.Logs.DefaultTimeRange
	}
	return "15m"
}

// LogBufferSize returns the configured log buffer size, defaulting to 10000.
func (c *Config) LogBufferSize() int {
	if c.Logs.BufferSize > 0 {
		return c.Logs.BufferSize
	}
	return 10000
}

// APITimeout returns the configured API timeout duration, defaulting to 5s.
func (c *Config) APITimeout() time.Duration {
	if c.API.TimeoutSeconds > 0 {
		return time.Duration(c.API.TimeoutSeconds) * time.Second
	}
	return 5 * time.Second
}

// HeartbeatInterval returns the configured heartbeat interval duration, defaulting to 5s.
func (c *Config) HeartbeatInterval() time.Duration {
	if c.API.HeartbeatSeconds > 0 {
		return time.Duration(c.API.HeartbeatSeconds) * time.Second
	}
	return 5 * time.Second
}

// ContextDirectories returns the configured directories to scan for kubeconfig
// files. Returns nil if the config is nil or no directories are configured.
func (c *Config) ContextDirectories() []string {
	if c == nil {
		return nil
	}
	return c.Contexts.Directories
}

// NotifyBufferSize returns the configured notify store capacity, defaulting to
// 1000 when zero or negative.
func (c *Config) NotifyBufferSize() int {
	if c.Notifications.BufferSize > 0 {
		return c.Notifications.BufferSize
	}
	return defaultNotifyBufferSize
}

// NotifyMaxVisible returns the maximum number of toasts shown at once,
// defaulting to 5 when zero or negative.
func (c *Config) NotifyMaxVisible() int {
	if c.Notifications.MaxVisible > 0 {
		return c.Notifications.MaxVisible
	}
	return defaultNotifyMaxVisible
}

// Toast severity levels, matching the iota order of notify.Level
// (LevelInfo=0, LevelWarning=1, LevelError=2). ToastTTL takes the int level
// rather than notify.Level to avoid an import cycle: config is imported
// (transitively, via theme) by the notify dependency graph, so config cannot
// import notify. Callers in the app pass int(notify.Level).
//
// These are exported (as plain ints, not the notify.Level type) so tests and
// callers can reference them without redefining the magic numbers. They MUST
// stay in sync with notify.Level's iota order; the import cycle only blocks
// importing the notify TYPE, not exposing matching int constants here.
const (
	ToastLevelInfo = iota
	ToastLevelWarning
	ToastLevelError
)

// ToastTTL returns the auto-hide duration for a toast of the given level. The
// level is the int value of notify.Level (0=info, 1=warning, 2=error); see the
// ToastLevel* constants for why this is an int rather than notify.Level.
//
// Per-level timeouts are configured in seconds. Because the YAML tags use
// omitempty, a configured 0 is indistinguishable from "unset", so the zero/unset
// value always maps to the per-level default (info 3s, warning 5s, error 8s). A
// POSITIVE value is used verbatim (in seconds), and a NEGATIVE value (e.g. -1)
// is the explicit, unambiguous way to make a level STICKY: ToastTTL returns 0,
// which notify.Store.Live treats as "never auto-hide". To pin error toasts on
// screen, set `timeout_error: -1`.
func (c *Config) ToastTTL(level int) time.Duration {
	var (
		secs int
		def  time.Duration
	)
	switch level {
	case ToastLevelWarning:
		secs, def = c.Notifications.TimeoutWarning, defaultToastTimeoutWarning
	case ToastLevelError:
		secs, def = c.Notifications.TimeoutError, defaultToastTimeoutError
	default:
		secs, def = c.Notifications.TimeoutInfo, defaultToastTimeoutInfo
	}
	switch {
	case secs > 0:
		return time.Duration(secs) * time.Second
	case secs < 0:
		return 0 // sticky
	default:
		return def
	}
}

// SetChartRef sets the chart reference for a release in a namespace.
func (c *Config) SetChartRef(namespace, release, ref string) {
	if c.Charts == nil {
		c.Charts = make(map[string]map[string]string)
	}
	if c.Charts[namespace] == nil {
		c.Charts[namespace] = make(map[string]string)
	}
	c.Charts[namespace][release] = ref
}
