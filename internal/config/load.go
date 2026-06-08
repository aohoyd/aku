package config

import (
	"os"
	"path/filepath"
)

// ConfigDir returns the aku config directory: $XDG_CONFIG_HOME/aku, or
// ~/.config/aku when XDG_CONFIG_HOME is unset.
func ConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aku")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "aku")
}

// KeymapPath returns the default keymap file path.
func KeymapPath() string {
	return filepath.Join(ConfigDir(), "keymap.yaml")
}

// Load loads both keymap and config from their default paths.
// Each file is optional; missing files fall back to defaults.
func Load() (*Keymap, *Config, error) {
	km, err := LoadKeymap(KeymapPath())
	if err != nil {
		return nil, nil, err
	}
	cfg, err := LoadConfig(ConfigPath())
	if err != nil {
		return nil, nil, err
	}
	return km, cfg, nil
}
