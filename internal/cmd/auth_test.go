package cmd

import (
	"bytes"
	"os"
	"testing"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/ui"
)

func TestAuthLoginRunRequiresCredentials(t *testing.T) {
	originalUser := os.Getenv("PLEX_USERNAME")
	originalPass := os.Getenv("PLEX_PASSWORD")
	_ = os.Unsetenv("PLEX_USERNAME")
	_ = os.Unsetenv("PLEX_PASSWORD")
	defer os.Setenv("PLEX_USERNAME", originalUser)
	defer os.Setenv("PLEX_PASSWORD", originalPass)

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	u := ui.New(ui.Options{Out: out, Err: errOut, ColorMode: ui.ColorNever})

	cmd := AuthLoginCmd{}
	cfg := &config.Config{}
	err := cmd.Run(nil, u, cfg)
	if err == nil {
		t.Fatal("expected error when username/password are missing")
	}
}

func TestAuthLogoutRunClearsStoredCredentials(t *testing.T) {
	tmpHome, err := os.MkdirTemp("", "plexcli-auth-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpHome)

	originalHome := os.Getenv("HOME")
	if setErr := os.Setenv("HOME", tmpHome); setErr != nil {
		t.Fatalf("failed to set HOME: %v", setErr)
	}
	defer os.Setenv("HOME", originalHome)

	if writeErr := config.WriteConfig(&config.Config{
		ServerURL: "http://localhost:32400",
		Token:     "test-token",
		Username:  "test-user",
		Password:  "test-pass",
	}); writeErr != nil {
		t.Fatalf("failed to seed config: %v", writeErr)
	}

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	u := ui.New(ui.Options{Out: out, Err: errOut, ColorMode: ui.ColorNever})

	cmd := AuthLogoutCmd{}
	if runErr := cmd.Run(nil, u, nil); runErr != nil {
		t.Fatalf("logout failed: %v", runErr)
	}

	got, readErr := config.ReadConfig()
	if readErr != nil {
		t.Fatalf("failed to read config: %v", readErr)
	}

	if got.Token != "" {
		t.Errorf("expected token to be cleared, got %q", got.Token)
	}
	if got.Username != "" {
		t.Errorf("expected username to be cleared, got %q", got.Username)
	}
	if got.Password != "" {
		t.Errorf("expected password to be cleared, got %q", got.Password)
	}
	if got.ServerURL != "http://localhost:32400" {
		t.Errorf("expected server URL to be preserved, got %q", got.ServerURL)
	}
}

func TestAuthServersRunRequiresToken(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	u := ui.New(ui.Options{Out: out, Err: errOut, ColorMode: ui.ColorNever})

	cmd := AuthServersCmd{}
	cfg := &config.Config{}
	err := cmd.Run(nil, u, cfg)
	if err == nil {
		t.Fatal("expected error when token is missing")
	}
}
