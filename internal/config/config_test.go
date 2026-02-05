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
	if err := WriteConfig(cfg); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	// Test ReadConfig
	got, err := ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
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

	if err := WriteConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// Set env vars
	os.Setenv("PLEX_SERVER", "http://env-server:32400")
	os.Setenv("PLEX_TOKEN", "env-token")
	defer os.Unsetenv("PLEX_SERVER")
	defer os.Unsetenv("PLEX_TOKEN")

	got, err := ReadConfig()
	if err != nil {
		t.Fatal(err)
	}

	if got.ServerURL != "http://env-server:32400" {
		t.Errorf("ServerURL not overridden: got %q", got.ServerURL)
	}
	if got.Token != "env-token" {
		t.Errorf("Token not overridden: got %q", got.Token)
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
