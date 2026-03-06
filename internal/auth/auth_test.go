package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LukeHagar/plexgo/models/components"

	"github.com/dogbertdev/plexcli/internal/config"
)

type fakePlexTVService struct {
	tokenDetails      *components.UserPlexAccount
	tokenDetailsErr   error
	serverResources   []components.PlexDevice
	serverResourceErr error
}

func (f *fakePlexTVService) TokenDetails(ctx context.Context) (*components.UserPlexAccount, error) {
	return f.tokenDetails, f.tokenDetailsErr
}

func (f *fakePlexTVService) ServerResources(ctx context.Context) ([]components.PlexDevice, error) {
	return f.serverResources, f.serverResourceErr
}

func TestTokenAuth_Name(t *testing.T) {
	auth := NewTokenAuth("test-token")
	if auth.Name() != "token" {
		t.Errorf("expected name 'token', got '%s'", auth.Name())
	}
}

func TestTokenAuth_Authenticate_EmptyToken(t *testing.T) {
	auth := NewTokenAuth("")
	ctx := context.Background()
	_, err := auth.Authenticate(ctx)
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("expected ErrNoCredentials, got %v", err)
	}
}

func TestTokenAuth_Authenticate_InvalidToken(t *testing.T) {
	original := newPlexTVService
	newPlexTVService = func(token string) plexTVService {
		return &fakePlexTVService{tokenDetailsErr: errors.New("unauthorized")}
	}
	defer func() { newPlexTVService = original }()

	auth := NewTokenAuth("invalid-token-that-will-fail")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := auth.Authenticate(ctx)
	if err == nil {
		t.Error("expected error for invalid token, got nil")
	}
}

func TestPasswordAuth_Name(t *testing.T) {
	auth := NewPasswordAuth("user", "pass")
	if auth.Name() != "password" {
		t.Errorf("expected name 'password', got '%s'", auth.Name())
	}
}

func TestPasswordAuth_Authenticate_EmptyCredentials(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
	}{
		{"empty username", "", "password"},
		{"empty password", "username", ""},
		{"both empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := NewPasswordAuth(tt.username, tt.password)
			ctx := context.Background()
			_, err := auth.Authenticate(ctx)
			if !errors.Is(err, ErrNoCredentials) {
				t.Errorf("expected ErrNoCredentials, got %v", err)
			}
		})
	}
}

func TestPasswordAuth_Authenticate_InvalidCredentials(t *testing.T) {
	auth := NewPasswordAuth("invalid_user_12345", "invalid_pass_12345")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := auth.Authenticate(ctx)
	if err == nil {
		t.Error("expected error for invalid credentials, got nil")
	}
}

func TestPasswordAuthWithClientID(t *testing.T) {
	auth := NewPasswordAuthWithClientID("user", "pass", "custom-client-id")
	if auth.clientID != "custom-client-id" {
		t.Errorf("expected clientID 'custom-client-id', got '%s'", auth.clientID)
	}
}

func TestAutoAuth_Name(t *testing.T) {
	cfg := config.Config{}
	auth := NewAutoAuth(cfg)
	if auth.Name() != "auto" {
		t.Errorf("expected name 'auto', got '%s'", auth.Name())
	}
}

func TestAutoAuth_Authenticate_NoCredentials(t *testing.T) {
	cfg := config.Config{}
	auth := NewAutoAuth(cfg)
	ctx := context.Background()

	_, err := auth.Authenticate(ctx)
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("expected ErrNoCredentials, got %v", err)
	}
}

func TestAutoAuth_Authenticate_TokenFirst(t *testing.T) {
	original := newPlexTVService
	newPlexTVService = func(token string) plexTVService {
		return &fakePlexTVService{tokenDetailsErr: errors.New("unauthorized")}
	}
	defer func() { newPlexTVService = original }()

	cfg := config.Config{
		Token:    "invalid-token-for-testing",
		Username: "user",
		Password: "pass",
	}
	auth := NewAutoAuth(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := auth.Authenticate(ctx)
	if err == nil {
		t.Error("expected error when both token and password fail")
	}
}

func TestAutoAuthWithClientID(t *testing.T) {
	cfg := config.Config{}
	auth := NewAutoAuthWithClientID(cfg, "custom-id")
	if auth.clientID != "custom-id" {
		t.Errorf("expected clientID 'custom-id', got '%s'", auth.clientID)
	}
}

func TestGetToken_NoCredentials(t *testing.T) {
	cfg := config.Config{}
	ctx := context.Background()

	_, err := GetToken(ctx, cfg)
	if !errors.Is(err, ErrNoCredentials) {
		t.Errorf("expected ErrNoCredentials, got %v", err)
	}
}

func TestConstants(t *testing.T) {
	if PlexTVURL != "https://plex.tv/api/v2" {
		t.Errorf("unexpected PlexTVURL: %s", PlexTVURL)
	}
	if DefaultClientID != "plexcli" {
		t.Errorf("unexpected DefaultClientID: %s", DefaultClientID)
	}
	if DefaultProduct != "plexcli" {
		t.Errorf("unexpected DefaultProduct: %s", DefaultProduct)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"ErrNoCredentials", ErrNoCredentials},
		{"ErrInvalidToken", ErrInvalidToken},
		{"ErrInvalidCredentials", ErrInvalidCredentials},
		{"ErrAuthFailed", ErrAuthFailed},
		{"ErrPINExpired", ErrPINExpired},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Error("expected non-nil error")
			}
		})
	}
}

func TestBuildAuthURL(t *testing.T) {
	got := BuildAuthURL("client-123", "code-abc", "plexcli")
	if !strings.HasPrefix(got, "https://app.plex.tv/auth#?") {
		t.Fatalf("unexpected auth URL prefix: %s", got)
	}
	if !strings.Contains(got, "clientID=client-123") {
		t.Fatalf("auth URL missing clientID: %s", got)
	}
	if !strings.Contains(got, "code=code-abc") {
		t.Fatalf("auth URL missing code: %s", got)
	}
}

func TestPollPINToken_InvalidPINID(t *testing.T) {
	ctx := context.Background()
	_, err := PollPINToken(ctx, 0, DefaultClientID, DefaultPINPoll)
	if err == nil {
		t.Fatal("expected error for invalid PIN id")
	}
}

func TestPreferredServerURI(t *testing.T) {
	server := ServerResource{
		Connections: []ServerConnection{
			{Protocol: "http", URI: "http://public.example:32400", Local: false},
			{Protocol: "https", URI: "https://local.example:32400", Local: true},
		},
	}

	got, ok := PreferredServerURI(server, true)
	if !ok {
		t.Fatal("expected preferred URI to be found")
	}
	if got != "https://local.example:32400" {
		t.Fatalf("unexpected preferred URI: %s", got)
	}
}

func TestNormalizeTokenAccountInfo(t *testing.T) {
	active := true
	confirmed := true
	home := true
	homeAdmin := true
	twoFactor := true
	status := components.UserPlexAccountSubscriptionStatusActive
	plan := "Lifetime Plex Pass"

	info := normalizeTokenAccountInfo(&components.UserPlexAccount{
		ID:               42,
		Title:            "Paul",
		Username:         "paul",
		Email:            "paul@example.com",
		FriendlyName:     "Paul Mansfield",
		UUID:             "uuid-123",
		Confirmed:        &confirmed,
		Home:             &home,
		HomeAdmin:        &homeAdmin,
		HasPassword:      &active,
		TwoFactorEnabled: &twoFactor,
		Roles:            []string{"plexpass", "admin"},
		Entitlements:     []string{"sync"},
		Subscription: &components.Subscription{
			Active: &active,
			Status: &status,
			Plan:   &plan,
		},
	})

	if info.Title != "Paul" || info.Username != "paul" || info.Email != "paul@example.com" {
		t.Fatalf("unexpected normalized account info: %+v", info)
	}
	if !info.Home || !info.HomeAdmin || !info.Confirmed || !info.SubscriptionActive || !info.TwoFactorEnabled {
		t.Fatalf("expected account flags to be preserved: %+v", info)
	}
	if info.SubscriptionStatus != "Active" || info.SubscriptionPlan != "Lifetime Plex Pass" {
		t.Fatalf("unexpected subscription info: %+v", info)
	}
}

func TestNormalizeTokenResourceInfo(t *testing.T) {
	platform := "linux"
	device := "server"
	platformVersion := "6.8"
	sourceTitle := "Shared Server"

	info := normalizeTokenResourceInfo(components.PlexDevice{
		Name:             "MediaBox",
		Product:          "Plex Media Server",
		ProductVersion:   "1.0.0",
		Platform:         &platform,
		PlatformVersion:  &platformVersion,
		Device:           &device,
		ClientIdentifier: "server-123",
		Provides:         "server",
		PublicAddress:    "203.0.113.5",
		AccessToken:      "server-token",
		Owned:            true,
		Home:             true,
		Presence:         true,
		SourceTitle:      &sourceTitle,
		Connections: []components.Connections{
			{
				Protocol: components.PlexDeviceProtocolHTTPS,
				Address:  "10.0.0.5",
				Port:     32400,
				URI:      "https://10.0.0.5:32400",
				Local:    true,
				Relay:    false,
				IPv6:     false,
			},
		},
	})

	if info.Name != "MediaBox" || info.Product != "Plex Media Server" || info.ConnectionCount != 1 {
		t.Fatalf("unexpected normalized resource info: %+v", info)
	}
	if len(info.Connections) != 1 || info.Connections[0].URI != "https://10.0.0.5:32400" {
		t.Fatalf("unexpected normalized connections: %+v", info.Connections)
	}
}

func TestInspectToken(t *testing.T) {
	original := newPlexTVService
	newPlexTVService = func(token string) plexTVService {
		confirmed := true
		return &fakePlexTVService{
			tokenDetails: &components.UserPlexAccount{
				Title:     "Paul",
				Username:  "paul",
				Email:     "paul@example.com",
				Confirmed: &confirmed,
			},
			serverResources: []components.PlexDevice{
				{
					Name:             "MediaBox",
					Product:          "Plex Media Server",
					ClientIdentifier: "server-123",
					Provides:         "server",
				},
			},
		}
	}
	defer func() { newPlexTVService = original }()

	info, err := InspectToken(context.Background(), "token-123")
	if err != nil {
		t.Fatalf("InspectToken returned error: %v", err)
	}
	if info.Account.Title != "Paul" {
		t.Fatalf("expected account title to be preserved, got %+v", info.Account)
	}
	if len(info.Resources) != 1 || info.Resources[0].ClientIdentifier != "server-123" {
		t.Fatalf("unexpected resources: %+v", info.Resources)
	}
}

func TestDiscoverServers_UsesSDKResources(t *testing.T) {
	original := newPlexTVService
	newPlexTVService = func(token string) plexTVService {
		return &fakePlexTVService{
			serverResources: []components.PlexDevice{
				{
					Name:             "MediaBox",
					Product:          "Plex Media Server",
					ClientIdentifier: "server-123",
					Owned:            true,
					Presence:         true,
					AccessToken:      "server-token",
					Connections: []components.Connections{
						{
							Protocol: components.PlexDeviceProtocolHTTPS,
							URI:      "https://10.0.0.5:32400",
							Local:    true,
						},
					},
				},
				{
					Name:    "Mobile Client",
					Product: "Plex for iOS",
				},
			},
		}
	}
	defer func() { newPlexTVService = original }()

	servers, err := DiscoverServers(context.Background(), "token-123")
	if err != nil {
		t.Fatalf("DiscoverServers returned error: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected one Plex Media Server, got %d", len(servers))
	}
	if servers[0].Name != "MediaBox" || len(servers[0].Connections) != 1 {
		t.Fatalf("unexpected normalized server: %+v", servers[0])
	}
}
