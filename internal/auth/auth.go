package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/LukeHagar/plexgo"
	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/user/plexcli/internal/config"
)

const (
	PlexTVURL       = "https://plex.tv/api/v2"
	DefaultTimeout  = 30 * time.Second
	DefaultClientID = "plexcli"
	DefaultProduct  = "plexcli"
	DefaultVersion  = "1.0.0"
	DefaultPlatform = "Go"
	StatusOK        = 200
	StatusCreated   = 201
)

var (
	ErrNoCredentials      = fmt.Errorf("no authentication credentials provided")
	ErrInvalidToken       = fmt.Errorf("invalid or expired token")
	ErrInvalidCredentials = fmt.Errorf("invalid username or password")
	ErrAuthFailed         = fmt.Errorf("authentication failed")
)

type AuthMethod interface {
	Authenticate(ctx context.Context) (string, error)
	Name() string
}

type TokenAuth struct {
	token string
}

func NewTokenAuth(token string) *TokenAuth {
	return &TokenAuth{token: token}
}

func (t *TokenAuth) Name() string {
	return "token"
}

func (t *TokenAuth) Authenticate(ctx context.Context) (string, error) {
	if t.token == "" {
		return "", ErrNoCredentials
	}

	sdk := plexgo.New(
		plexgo.WithServerURL(PlexTVURL),
		plexgo.WithSecurity(t.token),
		plexgo.WithClientIdentifier(DefaultClientID),
		plexgo.WithProduct(DefaultProduct),
		plexgo.WithVersion(DefaultVersion),
		plexgo.WithPlatform(DefaultPlatform),
	)

	req := operations.GetTokenDetailsRequest{}
	resp, err := sdk.Authentication.GetTokenDetails(ctx, req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	if resp.StatusCode != StatusOK {
		return "", fmt.Errorf("%w: received status %d", ErrInvalidToken, resp.StatusCode)
	}

	if resp.UserPlexAccount == nil {
		return "", fmt.Errorf("%w: no user account in response", ErrInvalidToken)
	}

	return t.token, nil
}

type PasswordAuth struct {
	username string
	password string
	clientID string
}

func NewPasswordAuth(username, password string) *PasswordAuth {
	return &PasswordAuth{
		username: username,
		password: password,
		clientID: DefaultClientID,
	}
}

func NewPasswordAuthWithClientID(username, password, clientID string) *PasswordAuth {
	return &PasswordAuth{
		username: username,
		password: password,
		clientID: clientID,
	}
}

func (p *PasswordAuth) Name() string {
	return "password"
}

func (p *PasswordAuth) Authenticate(ctx context.Context) (string, error) {
	if p.username == "" || p.password == "" {
		return "", ErrNoCredentials
	}

	sdk := plexgo.New(
		plexgo.WithServerURL(PlexTVURL),
		plexgo.WithClientIdentifier(p.clientID),
		plexgo.WithProduct(DefaultProduct),
		plexgo.WithVersion(DefaultVersion),
		plexgo.WithPlatform(DefaultPlatform),
	)

	req := operations.PostUsersSignInDataRequest{
		Accepts:          components.AcceptsApplicationJSON.ToPointer(),
		ClientIdentifier: &p.clientID,
		Product:          stringPtr(DefaultProduct),
		Version:          stringPtr(DefaultVersion),
		Platform:         stringPtr(DefaultPlatform),
		RequestBody: &operations.PostUsersSignInDataRequestBody{
			Login:    p.username,
			Password: p.password,
		},
	}

	resp, err := sdk.Authentication.PostUsersSignInData(ctx, req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
	}

	if resp.StatusCode != StatusCreated {
		return "", fmt.Errorf("%w: received status %d", ErrAuthFailed, resp.StatusCode)
	}

	if resp.UserPlexAccount == nil {
		return "", fmt.Errorf("%w: no user account in response", ErrAuthFailed)
	}

	token := resp.UserPlexAccount.GetAuthToken()
	if token == "" {
		return "", fmt.Errorf("%w: no auth token in response", ErrAuthFailed)
	}

	return token, nil
}

type AutoAuth struct {
	config   config.Config
	clientID string
}

func NewAutoAuth(cfg config.Config) *AutoAuth {
	return &AutoAuth{
		config:   cfg,
		clientID: DefaultClientID,
	}
}

func NewAutoAuthWithClientID(cfg config.Config, clientID string) *AutoAuth {
	return &AutoAuth{
		config:   cfg,
		clientID: clientID,
	}
}

func (a *AutoAuth) Name() string {
	return "auto"
}

func (a *AutoAuth) Authenticate(ctx context.Context) (string, error) {
	if a.config.Token != "" {
		tokenAuth := NewTokenAuth(a.config.Token)
		token, err := tokenAuth.Authenticate(ctx)
		if err == nil {
			return token, nil
		}
	}

	if a.config.Username != "" && a.config.Password != "" {
		passAuth := NewPasswordAuthWithClientID(a.config.Username, a.config.Password, a.clientID)
		token, err := passAuth.Authenticate(ctx)
		if err == nil {
			return token, nil
		}
		return "", fmt.Errorf("auto-auth failed: token auth failed and password auth failed: %w", err)
	}

	return "", ErrNoCredentials
}

func GetToken(ctx context.Context, cfg config.Config) (string, error) {
	auth := NewAutoAuth(cfg)
	return auth.Authenticate(ctx)
}

func GetTokenAndStore(ctx context.Context, cfg config.Config) (string, error) {
	token, err := GetToken(ctx, cfg)
	if err != nil {
		return "", err
	}

	if token != cfg.Token {
		cfg.Token = token
		cfg.Password = ""
		if err := config.WriteConfig(&cfg); err != nil {
			return token, nil
		}
	}

	return token, nil
}

func stringPtr(s string) *string {
	return &s
}
