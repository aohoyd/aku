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

// Config holds the application configuration.
type Config struct {
	Charts map[string]map[string]string `yaml:"charts,omitempty"`
	API    APIConfig                    `yaml:"api,omitempty"`
	Debug  DebugConfig                  `yaml:"debug,omitempty"`
	Exec   ExecConfig                   `yaml:"exec,omitempty"`
	Logs   LogsConfig                   `yaml:"logs,omitempty"`
}

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
	return filepath.Join(configDir(), "config.yaml")
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
