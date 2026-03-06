package cmd

import (
	"bytes"
	"context"
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

func TestAuthTokenInfoRunUsesResolvedRuntimeToken(t *testing.T) {
	originalResolveToken := resolveToken
	originalInspectToken := inspectToken
	resolveToken = func(ctx context.Context, cfg config.Config) (string, error) {
		if cfg.Username != "paul" || cfg.Password != "secret" {
			t.Fatalf("unexpected config passed to resolver: %+v", cfg)
		}
		return "runtime-token", nil
	}
	inspectToken = func(ctx context.Context, token string) (*auth.TokenInfo, error) {
		if token != "runtime-token" {
			t.Fatalf("expected runtime token, got %q", token)
		}
		return &auth.TokenInfo{}, nil
	}
	defer func() {
		resolveToken = originalResolveToken
		inspectToken = originalInspectToken
	}()

	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	u := ui.New(ui.Options{Out: out, Err: errOut, ColorMode: ui.ColorNever})

	cmd := AuthTokenInfoCmd{Output: "json"}
	cfg := &config.Config{Username: "paul", Password: "secret"}
	if err := cmd.Run(nil, u, cfg); err != nil {
		t.Fatalf("Run returned error: %v", err)
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
	if strings.Contains(out.String(), "access_token") {
		t.Fatalf("expected JSON payload to omit access_token, got: %s", out.String())
	}
}

func TestFlattenTokenResourceItemsNoConnections(t *testing.T) {
	items := flattenTokenResourceItems([]auth.TokenResourceInfo{
		{
			Name:             "Offline Box",
			Product:          "Plex Media Server",
			ClientIdentifier: "server-123",
			ConnectionCount:  0,
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected 1 flattened item, got %d", len(items))
	}
	if items[0].ConnectionIndex != 0 {
		t.Fatalf("expected connection index 0 for resource without connections, got %d", items[0].ConnectionIndex)
	}
	if items[0].ConnectionCount != 0 {
		t.Fatalf("expected connection count 0, got %d", items[0].ConnectionCount)
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
