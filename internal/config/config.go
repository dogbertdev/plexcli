package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/yosuke-furukawa/json5/encoding/json5"
)

const (
	filePermissions = 0600
	dirPermissions  = 0700
	configDir       = "plexcli"
	configFilename  = "config.json"
)

// Config represents the application configuration.
type Config struct {
	ServerURL     string `json:"server_url"`
	Token         string `json:"token"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	Timeout       int    `json:"timeout"`
	CacheTTL      int    `json:"cache_ttl"`
	CacheDisabled bool   `json:"-"`
	CacheRefresh  bool   `json:"-"`
}

func ConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}
	return filepath.Join(home, ".config", configDir, configFilename), nil
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
		if unmarshalErr := json5.Unmarshal(data, cfg); unmarshalErr != nil {
			return nil, fmt.Errorf("failed to parse config file: %w", unmarshalErr)
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
	if val := os.Getenv("PLEX_CACHE_TTL"); val != "" {
		ttl, parseErr := strconv.Atoi(strings.TrimSpace(val))
		if parseErr == nil {
			cfg.CacheTTL = ttl
		}
	}
	if val := os.Getenv("PLEX_NO_CACHE"); val != "" {
		disable, parseErr := strconv.ParseBool(strings.TrimSpace(val))
		if parseErr == nil {
			cfg.CacheDisabled = disable
		}
	}
	if val := os.Getenv("PLEX_REFRESH_CACHE"); val != "" {
		refresh, parseErr := strconv.ParseBool(strings.TrimSpace(val))
		if parseErr == nil {
			cfg.CacheRefresh = refresh
		}
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
	if mkdirErr := os.MkdirAll(dir, dirPermissions); mkdirErr != nil {
		return fmt.Errorf("failed to create config directory: %w", mkdirErr)
	}

	data, err := json5.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	tmpFile := path + ".tmp"
	if writeErr := os.WriteFile(tmpFile, data, filePermissions); writeErr != nil {
		return fmt.Errorf("failed to write temporary config file: %w", writeErr)
	}

	if renameErr := os.Rename(tmpFile, path); renameErr != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename temporary config file: %w", renameErr)
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
