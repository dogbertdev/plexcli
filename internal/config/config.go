package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/yosuke-furukawa/json5/encoding/json5"
)

// Config represents the application configuration.
type Config struct {
	ServerURL string `json:"server_url"`
	Token     string `json:"token"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Timeout   int    `json:"timeout"`
}

// ConfigPath returns the default configuration file path: ~/.config/plexcli/config.json
func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "plexcli", "config.json"), nil
}

// ExpandPath expands the tilde (~) in a path to the user's home directory.
func ExpandPath(path string) (string, error) {
	if len(path) == 0 || path[0] != '~' {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	if len(path) == 1 {
		return home, nil
	}

	if path[1] == '/' || (runtime.GOOS == "windows" && path[1] == '\\') {
		return filepath.Join(home, path[2:]), nil
	}

	return path, nil
}

// ReadConfig reads the configuration from the default path, overriding with environment variables.
func ReadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := &Config{}

	data, err := os.ReadFile(path)
	if err == nil {
		if err := json5.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Override with environment variables
	if val := os.Getenv("PLEX_SERVER"); val != "" {
		cfg.ServerURL = val
	}
	if val := os.Getenv("PLEX_TOKEN"); val != "" {
		cfg.Token = val
	}
	if val := os.Getenv("PLEX_USERNAME"); val != "" {
		cfg.Username = val
	}
	if val := os.Getenv("PLEX_PASSWORD"); val != "" {
		cfg.Password = val
	}

	return cfg, nil
}

// WriteConfig writes the configuration atomically to the default path.
func WriteConfig(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json5.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Atomic write using a temporary file
	tmpFile := path + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temporary config file: %w", err)
	}

	if err := os.Rename(tmpFile, path); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename temporary config file: %w", err)
	}

	return nil
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if c.ServerURL == "" {
		return fmt.Errorf("server URL is required")
	}
	if c.Token == "" && (c.Username == "" || c.Password == "") {
		return fmt.Errorf("either token or username/password must be provided")
	}
	return nil
}
