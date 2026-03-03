package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/auth"
	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type AuthCmd struct {
	Login   AuthLoginCmd   `cmd:"" help:"Authenticate with Plex and store a token"`
	Logout  AuthLogoutCmd  `cmd:"" help:"Clear stored Plex authentication credentials"`
	Servers AuthServersCmd `cmd:"" help:"Discover Plex servers and optionally select one"`
}

type AuthLoginCmd struct {
	Username string `help:"Plex username or email" short:"u" env:"PLEX_USERNAME" default:""`
	Password string `help:"Plex password" short:"p" env:"PLEX_PASSWORD" default:""`
	Browser  bool   `help:"Use browser-based Plex login flow to fetch a token" default:"false"`
	NoOpen   bool   `help:"Do not open the browser automatically (prints login URL)" default:"false"`
	Wait     int    `help:"Maximum time to wait for browser login in seconds" default:"300"`
}

func (c *AuthLoginCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is required")
	}

	loginCfg := *cfg
	if c.Username != "" {
		loginCfg.Username = c.Username
	}
	if c.Password != "" {
		loginCfg.Password = c.Password
	}

	var token string
	if c.Browser {
		waitSeconds := c.Wait
		if waitSeconds <= 0 {
			waitSeconds = 300
		}

		authCtx, cancel := context.WithTimeout(context.Background(), time.Duration(waitSeconds)*time.Second)
		defer cancel()

		pin, err := auth.CreatePIN(authCtx, auth.DefaultClientID, auth.DefaultProduct)
		if err != nil {
			return fmt.Errorf("failed to initialize browser login: %w", err)
		}

		loginURL := auth.BuildAuthURL(auth.DefaultClientID, pin.Code, auth.DefaultProduct)
		fmt.Fprintln(u.Out(), "Open this URL to sign in to Plex:")
		fmt.Fprintln(u.Out(), loginURL)

		if !c.NoOpen {
			if browserErr := openBrowser(loginURL); browserErr != nil {
				fmt.Fprintf(u.Err(), "Could not auto-open browser: %v\n", browserErr)
			}
		}

		fmt.Fprintln(u.Out(), "Waiting for Plex login approval...")
		token, err = auth.PollPINToken(authCtx, pin.ID, auth.DefaultClientID, auth.DefaultPINPoll)
		if err != nil {
			return fmt.Errorf("browser login failed: %w", err)
		}
	} else {
		if loginCfg.Username == "" || loginCfg.Password == "" {
			return fmt.Errorf("username and password are required (use flags or PLEX_USERNAME/PLEX_PASSWORD), or use --browser")
		}

		authCtx, cancel := context.WithTimeout(context.Background(), auth.DefaultTimeout)
		defer cancel()

		passwordAuth := auth.NewPasswordAuth(loginCfg.Username, loginCfg.Password)
		var err error
		token, err = passwordAuth.Authenticate(authCtx)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	savedCfg, err := config.ReadConfigFileOnly()
	if err != nil {
		savedCfg = &config.Config{}
	}

	if loginCfg.ServerURL != "" {
		savedCfg.ServerURL = loginCfg.ServerURL
	}
	savedCfg.Token = token
	savedCfg.Username = loginCfg.Username
	savedCfg.Password = ""

	if writeErr := config.WriteConfig(savedCfg); writeErr != nil {
		return fmt.Errorf("authenticated, but failed to write config: %w", writeErr)
	}

	configPath, err := config.ConfigPath()
	if err != nil {
		fmt.Fprintln(u.Out(), "Authenticated with Plex. Token stored in config.")
		return nil
	}

	fmt.Fprintf(u.Out(), "Authenticated with Plex. Token stored at %s\n", configPath)
	return nil
}

type AuthLogoutCmd struct{}

func (c *AuthLogoutCmd) Run(ctx *kong.Context, u *ui.UI, _ *config.Config) error {
	cfg, err := config.ReadConfigFileOnly()
	if err != nil {
		cfg = &config.Config{}
	}

	cfg.Token = ""
	cfg.Password = ""
	cfg.Username = ""

	if writeErr := config.WriteConfig(cfg); writeErr != nil {
		return fmt.Errorf("failed to clear stored authentication: %w", writeErr)
	}

	configPath, err := config.ConfigPath()
	if err != nil {
		fmt.Fprintln(u.Out(), "Logged out. Stored Plex authentication credentials were cleared.")
		return nil
	}

	if _, statErr := os.Stat(configPath); statErr == nil {
		fmt.Fprintf(u.Out(), "Logged out. Cleared stored Plex authentication credentials in %s\n", configPath)
		return nil
	}

	fmt.Fprintln(u.Out(), "Logged out. No stored config file was found.")
	return nil
}

func openBrowser(targetURL string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", targetURL).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL).Start()
	default:
		return exec.Command("xdg-open", targetURL).Start()
	}
}

type AuthServersCmd struct {
	Select      int    `help:"1-based index of discovered server to set in config" default:"0"`
	Output      string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
	PreferLocal bool   `help:"Prefer local connection URIs when selecting a server" default:"true"`
}

type AuthServerItem struct {
	ServerIndex     int    `json:"server_index"`
	ConnectionIndex int    `json:"connection_index"`
	Name            string `json:"name"`
	Product         string `json:"product"`
	Owned           bool   `json:"owned"`
	Online          bool   `json:"online"`
	ServerID        string `json:"server_id"`
	ConnectionCount int    `json:"connection_count"`
	ConnectionURI   string `json:"connection_uri"`
	Protocol        string `json:"protocol"`
	Local           bool   `json:"local"`
	Relay           bool   `json:"relay"`
	Preferred       bool   `json:"preferred"`
}

func (c *AuthServersCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is required")
	}
	if cfg.Token == "" {
		return fmt.Errorf("token is required to discover servers; run 'plex auth login --browser' first")
	}

	authCtx, cancel := context.WithTimeout(context.Background(), auth.DefaultTimeout)
	defer cancel()

	servers, err := auth.DiscoverServers(authCtx, cfg.Token)
	if err != nil {
		return fmt.Errorf("failed to discover servers: %w", err)
	}
	if len(servers) == 0 {
		return fmt.Errorf("no Plex Media Servers found for this account")
	}

	items := make([]AuthServerItem, 0, len(servers))
	for i, server := range servers {
		uri, hasPreferred := auth.PreferredServerURI(server, c.PreferLocal)
		if len(server.Connections) == 0 {
			items = append(items, AuthServerItem{
				ServerIndex:     i + 1,
				ConnectionIndex: 1,
				Name:            server.Name,
				Product:         server.Product,
				Owned:           server.Owned,
				Online:          server.Presence,
				ServerID:        server.ClientIdentifier,
				ConnectionCount: 0,
			})
			continue
		}

		for j, conn := range server.Connections {
			preferred := hasPreferred && conn.URI == uri
			items = append(items, AuthServerItem{
				ServerIndex:     i + 1,
				ConnectionIndex: j + 1,
				Name:            server.Name,
				Product:         server.Product,
				Owned:           server.Owned,
				Online:          server.Presence,
				ServerID:        server.ClientIdentifier,
				ConnectionCount: len(server.Connections),
				ConnectionURI:   conn.URI,
				Protocol:        conn.Protocol,
				Local:           conn.Local,
				Relay:           conn.Relay,
				Preferred:       preferred,
			})
		}
	}

	if c.Select > 0 {
		if c.Select > len(servers) {
			return fmt.Errorf("invalid selection %d: choose between 1 and %d", c.Select, len(servers))
		}

		server := servers[c.Select-1]
		serverURL, ok := auth.PreferredServerURI(server, c.PreferLocal)
		if !ok || serverURL == "" {
			return fmt.Errorf("selected server has no usable connection URI")
		}

		savedCfg, err := config.ReadConfigFileOnly()
		if err != nil {
			savedCfg = &config.Config{}
		}
		applySelectedServer(savedCfg, server, serverURL)
		if err := config.WriteConfig(savedCfg); err != nil {
			return fmt.Errorf("failed to save selected server: %w", err)
		}

		fmt.Fprintf(u.Out(), "Selected server %q (%s)\n", server.Name, serverURL)
	}

	return c.output(u.Out(), items)
}

func (c *AuthServersCmd) output(w io.Writer, items []AuthServerItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"SERVER", "CONN", "NAME", "ONLINE", "OWNED", "LOCAL", "RELAY", "PROTO", "PREFERRED", "CONNECTION_URI", "CONNS"}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			strconv.Itoa(item.ServerIndex),
			strconv.Itoa(item.ConnectionIndex),
			item.Name,
			strconv.FormatBool(item.Online),
			strconv.FormatBool(item.Owned),
			strconv.FormatBool(item.Local),
			strconv.FormatBool(item.Relay),
			item.Protocol,
			strconv.FormatBool(item.Preferred),
			item.ConnectionURI,
			strconv.Itoa(item.ConnectionCount),
		})
	}

	return formatter.Format(w, header, rows, items)
}

func applySelectedServer(cfg *config.Config, server auth.ServerResource, serverURL string) {
	cfg.ServerURL = serverURL
	if server.AccessToken != "" {
		cfg.Token = server.AccessToken
	}
}
