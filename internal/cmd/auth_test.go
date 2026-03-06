package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dogbertdev/plexcli/internal/auth"
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

	got, readErr := config.ReadConfigFileOnly()
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

func TestAuthTokenInfoRunRequiresToken(t *testing.T) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	u := ui.New(ui.Options{Out: out, Err: errOut, ColorMode: ui.ColorNever})

	cmd := AuthTokenInfoCmd{}
	cfg := &config.Config{}
	err := cmd.Run(nil, u, cfg)
	if err == nil {
		t.Fatal("expected error when token is missing")
	}
}

func TestAuthTokenInfoOutputTable(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := AuthTokenInfoCmd{Output: "table"}

	info := &auth.TokenInfo{
		Account: auth.TokenAccountInfo{
			Title:              "Paul",
			Username:           "paul",
			Email:              "paul@example.com",
			FriendlyName:       "Paul Mansfield",
			Home:               true,
			HomeAdmin:          true,
			Confirmed:          true,
			HasPassword:        true,
			TwoFactorEnabled:   true,
			SubscriptionActive: true,
			SubscriptionStatus: "Active",
			SubscriptionPlan:   "Lifetime Plex Pass",
		},
		Resources: []auth.TokenResourceInfo{
			{
				Name:             "MediaBox",
				Product:          "Plex Media Server",
				Provides:         "server",
				ClientIdentifier: "server-123",
				Owned:            true,
				Home:             true,
				Presence:         true,
				ConnectionCount:  1,
				Connections: []auth.TokenConnectionInfo{
					{
						Protocol: "https",
						URI:      "https://10.0.0.5:32400",
						Local:    true,
					},
				},
			},
		},
	}

	if err := cmd.output(out, info); err != nil {
		t.Fatalf("output returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"TITLE\tPaul", "EMAIL\tpaul@example.com", "MediaBox", "server-123", "https://10.0.0.5:32400"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected output to contain %q, got:\n%s", want, got)
		}
	}
}

func TestAuthTokenInfoOutputJSON(t *testing.T) {
	out := &bytes.Buffer{}
	cmd := AuthTokenInfoCmd{Output: "json"}

	info := &auth.TokenInfo{
		Account: auth.TokenAccountInfo{
			Title:    "Paul",
			Username: "paul",
			Email:    "paul@example.com",
		},
		Resources: []auth.TokenResourceInfo{
			{Name: "MediaBox", ClientIdentifier: "server-123"},
		},
	}

	if err := cmd.output(out, info); err != nil {
		t.Fatalf("output returned error: %v", err)
	}

	var decoded auth.TokenInfo
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("failed to decode JSON output: %v", err)
	}
	if decoded.Account.Title != "Paul" || len(decoded.Resources) != 1 || decoded.Resources[0].ClientIdentifier != "server-123" {
		t.Fatalf("unexpected JSON payload: %+v", decoded)
	}
}

func TestApplySelectedServer_UsesServerAccessTokenWhenPresent(t *testing.T) {
	cfg := &config.Config{Token: "account-token"}
	server := auth.ServerResource{
		AccessToken: "server-token",
	}

	applySelectedServer(cfg, server, "https://example:32400", "account-token")

	if cfg.ServerURL != "https://example:32400" {
		t.Fatalf("expected server URL to be set, got %q", cfg.ServerURL)
	}
	if cfg.Token != "server-token" {
		t.Fatalf("expected server token to be used, got %q", cfg.Token)
	}
}

func TestApplySelectedServer_KeepsExistingTokenWhenServerTokenMissing(t *testing.T) {
	cfg := &config.Config{Token: "account-token"}
	server := auth.ServerResource{}

	applySelectedServer(cfg, server, "https://example:32400", "account-token")

	if cfg.Token != "account-token" {
		t.Fatalf("expected existing token to be preserved, got %q", cfg.Token)
	}
}

func TestApplySelectedServer_UsesFallbackTokenWhenConfigTokenEmpty(t *testing.T) {
	cfg := &config.Config{Token: ""}
	server := auth.ServerResource{}

	applySelectedServer(cfg, server, "https://example:32400", "runtime-token")

	if cfg.Token != "runtime-token" {
		t.Fatalf("expected fallback token to be used, got %q", cfg.Token)
	}
}
