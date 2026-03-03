package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
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
	DefaultPINPoll  = 1 * time.Second
)

var (
	ErrNoCredentials      = fmt.Errorf("no authentication credentials provided")
	ErrInvalidToken       = fmt.Errorf("invalid or expired token")
	ErrInvalidCredentials = fmt.Errorf("invalid username or password")
	ErrAuthFailed         = fmt.Errorf("authentication failed")
	ErrPINExpired         = fmt.Errorf("authentication PIN expired")
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

type pinResponse struct {
	ID        int64  `json:"id"`
	Code      string `json:"code"`
	AuthToken string `json:"authToken"`
	ExpiresIn int    `json:"expiresIn"`
}

type ServerConnection struct {
	Protocol string `json:"protocol"`
	URI      string `json:"uri"`
	Local    bool   `json:"local"`
	Relay    bool   `json:"relay"`
}

type ServerResource struct {
	Name             string             `json:"name"`
	Product          string             `json:"product"`
	ClientIdentifier string             `json:"clientIdentifier"`
	Owned            bool               `json:"owned"`
	Presence         bool               `json:"presence"`
	AccessToken      string             `json:"accessToken"`
	Connections      []ServerConnection `json:"connections"`
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

func CreatePIN(ctx context.Context, clientID, product string) (*pinResponse, error) {
	if clientID == "" {
		clientID = DefaultClientID
	}
	if product == "" {
		product = DefaultProduct
	}

	req, err := http.NewRequestWithContext(ctx, "POST", PlexTVURL+"/pins?strong=true", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create PIN request: %v", ErrAuthFailed, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Client-Identifier", clientID)
	req.Header.Set("X-Plex-Product", product)
	req.Header.Set("X-Plex-Version", DefaultVersion)
	req.Header.Set("X-Plex-Platform", DefaultPlatform)

	resp, err := (&http.Client{Timeout: DefaultTimeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create PIN: %v", ErrAuthFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusCreated && resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("%w: failed to create PIN (status %d)", ErrAuthFailed, resp.StatusCode)
	}

	var pin pinResponse
	if err := json.NewDecoder(resp.Body).Decode(&pin); err != nil {
		return nil, fmt.Errorf("%w: failed to decode PIN response: %v", ErrAuthFailed, err)
	}
	if pin.ID == 0 || pin.Code == "" {
		return nil, fmt.Errorf("%w: invalid PIN response", ErrAuthFailed)
	}

	return &pin, nil
}

func BuildAuthURL(clientID, code, product string) string {
	if clientID == "" {
		clientID = DefaultClientID
	}
	if product == "" {
		product = DefaultProduct
	}

	values := url.Values{}
	values.Set("clientID", clientID)
	values.Set("code", code)
	values.Set("context[device][product]", product)
	return "https://app.plex.tv/auth#?" + values.Encode()
}

func PollPINToken(ctx context.Context, pinID int64, clientID string, pollInterval time.Duration) (string, error) {
	if pinID == 0 {
		return "", fmt.Errorf("%w: invalid PIN id", ErrAuthFailed)
	}
	if clientID == "" {
		clientID = DefaultClientID
	}
	if pollInterval <= 0 {
		pollInterval = DefaultPINPoll
	}

	client := &http.Client{Timeout: DefaultTimeout}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		req, err := http.NewRequestWithContext(ctx, "GET", PlexTVURL+"/pins/"+strconv.FormatInt(pinID, 10), nil)
		if err != nil {
			return "", fmt.Errorf("%w: failed to poll PIN: %v", ErrAuthFailed, err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Plex-Client-Identifier", clientID)
		req.Header.Set("X-Plex-Product", DefaultProduct)
		req.Header.Set("X-Plex-Version", DefaultVersion)
		req.Header.Set("X-Plex-Platform", DefaultPlatform)

		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return "", fmt.Errorf("%w: %v", ErrPINExpired, ctx.Err())
			}
			return "", fmt.Errorf("%w: failed to poll PIN: %v", ErrAuthFailed, err)
		}

		var pin pinResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&pin)
		resp.Body.Close()
		if resp.StatusCode != StatusOK {
			return "", fmt.Errorf("%w: failed to poll PIN (status %d)", ErrAuthFailed, resp.StatusCode)
		}
		if decodeErr != nil {
			return "", fmt.Errorf("%w: failed to decode PIN poll response: %v", ErrAuthFailed, decodeErr)
		}
		if pin.AuthToken != "" {
			return pin.AuthToken, nil
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("%w: %v", ErrPINExpired, ctx.Err())
		case <-ticker.C:
		}
	}
}

func DiscoverServers(ctx context.Context, token string) ([]ServerResource, error) {
	if token == "" {
		return nil, ErrNoCredentials
	}

	req, err := http.NewRequestWithContext(ctx, "GET", PlexTVURL+"/resources?includeHttps=1", nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create discover request: %v", ErrAuthFailed, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Token", token)
	req.Header.Set("X-Plex-Client-Identifier", DefaultClientID)
	req.Header.Set("X-Plex-Product", DefaultProduct)
	req.Header.Set("X-Plex-Version", DefaultVersion)
	req.Header.Set("X-Plex-Platform", DefaultPlatform)

	resp, err := (&http.Client{Timeout: DefaultTimeout}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to discover servers: %v", ErrAuthFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("%w: failed to discover servers (status %d)", ErrAuthFailed, resp.StatusCode)
	}

	var resources []ServerResource
	if err := json.NewDecoder(resp.Body).Decode(&resources); err != nil {
		return nil, fmt.Errorf("%w: failed to decode server resources: %v", ErrAuthFailed, err)
	}

	servers := make([]ServerResource, 0, len(resources))
	for _, resource := range resources {
		if strings.Contains(resource.Product, "Plex Media Server") {
			servers = append(servers, resource)
		}
	}

	return servers, nil
}

func PreferredServerURI(server ServerResource, preferLocal bool) (string, bool) {
	if len(server.Connections) == 0 {
		return "", false
	}

	type matcher struct {
		localOnly bool
		protocol  string
	}
	orders := []matcher{
		{localOnly: preferLocal, protocol: "https"},
		{localOnly: preferLocal, protocol: "http"},
		{localOnly: false, protocol: "https"},
		{localOnly: false, protocol: "http"},
	}

	for _, order := range orders {
		for _, c := range server.Connections {
			if order.localOnly && !c.Local {
				continue
			}
			if c.URI == "" || c.Protocol != order.protocol {
				continue
			}
			return c.URI, true
		}
	}

	return "", false
}
