package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/user/plexcli/internal/config"
)

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

func TestStringPtr(t *testing.T) {
	s := "test"
	ptr := stringPtr(s)
	if ptr == nil {
		t.Error("expected non-nil pointer")
		return
	}
	if *ptr != s {
		t.Errorf("expected '%s', got '%s'", s, *ptr)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Error("expected non-nil error")
			}
		})
	}
}
