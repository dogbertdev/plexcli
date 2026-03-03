package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"empty", "", ""},
		{"no tilde", "/etc/config", "/etc/config"},
		{"only tilde", "~", home},
		{"tilde with slash", "~/config", filepath.Join(home, "config")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandPath(tt.path)
			if err != nil {
				t.Fatalf("ExpandPath(%q) error: %v", tt.path, err)
			}
			if got != tt.expected {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestReadWriteConfig(t *testing.T) {
	// Mock HOME directory
	tmpHome, err := os.MkdirTemp("", "plexcli-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	cfg := &Config{
		ServerURL: "http://localhost:32400",
		Token:     "test-token",
		Username:  "test-user",
		Password:  "test-pass",
	}

	// Test WriteConfig
	if writeErr := WriteConfig(cfg); writeErr != nil {
		t.Fatalf("WriteConfig failed: %v", writeErr)
	}

	// Test ReadConfig
	got, readErr := ReadConfig()
	if readErr != nil {
		t.Fatalf("ReadConfig failed: %v", readErr)
	}

	if got.ServerURL != cfg.ServerURL || got.Token != cfg.Token || got.Username != cfg.Username || got.Password != cfg.Password {
		t.Errorf("ReadConfig() = %+v, want %+v", got, cfg)
	}
}

func TestConfigEnvOverride(t *testing.T) {
	// Mock HOME directory
	tmpHome, err := os.MkdirTemp("", "plexcli-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	originalHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	cfg := &Config{
		ServerURL: "http://localhost:32400",
		Token:     "test-token",
	}

	if writeErr := WriteConfig(cfg); writeErr != nil {
		t.Fatal(writeErr)
	}

	// Set env vars
	os.Setenv("PLEX_SERVER", "http://env-server:32400")
	os.Setenv("PLEX_TOKEN", "env-token")
	defer os.Unsetenv("PLEX_SERVER")
	defer os.Unsetenv("PLEX_TOKEN")

	got, readErr := ReadConfig()
	if readErr != nil {
		t.Fatal(readErr)
	}

	if got.ServerURL != "http://env-server:32400" {
		t.Errorf("ServerURL not overridden: got %q", got.ServerURL)
	}
	if got.Token != "env-token" {
		t.Errorf("Token not overridden: got %q", got.Token)
	}
}

func TestReadConfigFileOnly_IgnoresEnvironmentOverrides(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "plexcli-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	originalHome := os.Getenv("HOME")
	if setErr := os.Setenv("HOME", tmpHome); setErr != nil {
		t.Fatal(setErr)
	}
	defer os.Setenv("HOME", originalHome)

	cfg := &Config{
		ServerURL: "http://localhost:32400",
		Token:     "file-token",
		Username:  "file-user",
		Password:  "file-pass",
	}
	if writeErr := WriteConfig(cfg); writeErr != nil {
		t.Fatal(writeErr)
	}

	os.Setenv("PLEX_SERVER", "http://env-server:32400")
	os.Setenv("PLEX_TOKEN", "env-token")
	os.Setenv("PLEX_USERNAME", "env-user")
	os.Setenv("PLEX_PASSWORD", "env-pass")
	defer os.Unsetenv("PLEX_SERVER")
	defer os.Unsetenv("PLEX_TOKEN")
	defer os.Unsetenv("PLEX_USERNAME")
	defer os.Unsetenv("PLEX_PASSWORD")

	got, readErr := ReadConfigFileOnly()
	if readErr != nil {
		t.Fatal(readErr)
	}

	if got.ServerURL != cfg.ServerURL {
		t.Errorf("ServerURL should come from file: got %q want %q", got.ServerURL, cfg.ServerURL)
	}
	if got.Token != cfg.Token {
		t.Errorf("Token should come from file: got %q want %q", got.Token, cfg.Token)
	}
	if got.Username != cfg.Username {
		t.Errorf("Username should come from file: got %q want %q", got.Username, cfg.Username)
	}
	if got.Password != cfg.Password {
		t.Errorf("Password should come from file: got %q want %q", got.Password, cfg.Password)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"valid with token", Config{ServerURL: "http://localhost", Token: "abc"}, false},
		{"valid with user/pass", Config{ServerURL: "http://localhost", Username: "u", Password: "p"}, false},
		{"missing server", Config{Token: "abc"}, true},
		{"missing auth", Config{ServerURL: "http://localhost"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
