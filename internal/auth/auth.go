package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/dogbertdev/plexcli/internal/config"
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

type userResponse struct {
	AuthToken string `json:"authToken"`
	Username  string `json:"username"`
	Email     string `json:"email"`
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

	client := &http.Client{Timeout: DefaultTimeout}
	req, err := http.NewRequestWithContext(ctx, "GET", PlexTVURL+"/user", nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", t.token)
	req.Header.Set("X-Plex-Client-Identifier", DefaultClientID)
	req.Header.Set("X-Plex-Product", DefaultProduct)
	req.Header.Set("X-Plex-Version", DefaultVersion)
	req.Header.Set("X-Plex-Platform", DefaultPlatform)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		return "", fmt.Errorf("%w: received status %d", ErrInvalidToken, resp.StatusCode)
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

	client := &http.Client{Timeout: DefaultTimeout}

	data := url.Values{}
	data.Set("login", p.username)
	data.Set("password", p.password)

	req, err := http.NewRequestWithContext(ctx, "POST", PlexTVURL+"/users/signin", strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Plex-Client-Identifier", p.clientID)
	req.Header.Set("X-Plex-Product", DefaultProduct)
	req.Header.Set("X-Plex-Version", DefaultVersion)
	req.Header.Set("X-Plex-Platform", DefaultPlatform)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCredentials, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusCreated {
		return "", fmt.Errorf("%w: received status %d", ErrAuthFailed, resp.StatusCode)
	}

	var user userResponse
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", fmt.Errorf("%w: failed to decode response: %v", ErrAuthFailed, err)
	}

	if user.AuthToken == "" {
		return "", fmt.Errorf("%w: no auth token in response", ErrAuthFailed)
	}

	return user.AuthToken, nil
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
