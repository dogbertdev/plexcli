package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/cmd"
	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/ui"
)

var version = "dev"

// CLI represents the root command structure
type CLI struct {
	Version bool `help:"Show version information" short:"v"`

	// Global flags
	JSON         bool   `help:"Output JSON to stdout" default:"false"`
	Plain        bool   `help:"Output TSV to stdout" default:"false"`
	Color        string `help:"Color mode: auto|always|never" default:"auto" enum:"auto,always,never"`
	Config       string `help:"Path to config file" type:"path" default:""`
	Server       string `help:"Plex server URL" env:"PLEX_SERVER" default:""`
	Token        string `help:"Plex authentication token" env:"PLEX_TOKEN" default:""`
	Timeout      int    `help:"Request timeout in seconds" default:"120"`
	CacheTTL     int    `help:"Library cache TTL in seconds (0 disables cache)" env:"PLEX_CACHE_TTL" default:"300"`
	NoCache      bool   `help:"Disable local library cache for this run" env:"PLEX_NO_CACHE" default:"false"`
	RefreshCache bool   `help:"Bypass cache reads and refresh local cache for this run" env:"PLEX_REFRESH_CACHE" default:"false"`

	// Subcommands
	Unwatched    cmd.UnwatchedCmd        `cmd:"" help:"List unwatched items in the library"`
	Unmatched    cmd.UnmatchedCmd        `cmd:"" help:"List unmatched items in the library"`
	Recently     cmd.RecentlyWatchedCmd  `cmd:"" name:"recently-watched" help:"List recently watched items"`
	Added        cmd.RecentlyAddedCmd    `cmd:"" name:"recently-added" help:"List recently added items"`
	Duplicates   cmd.DuplicatesCmd       `cmd:"" help:"Find duplicate media files"`
	FilePaths    cmd.FilePathsCmd        `cmd:"" name:"file-paths" help:"List file paths for media items"`
	Subtitles    cmd.SubtitlesMissingCmd `cmd:"" name:"subtitles-missing" help:"Find items missing subtitle languages"`
	Audio        cmd.AudioCheckCmd       `cmd:"" name:"audio-check" help:"Check audio codec and channel configuration"`
	EpisodesMiss cmd.EpisodesMissingCmd  `cmd:"" name:"episodes-missing" help:"Find missing episodes in TV series"`
	Quality      cmd.QualityCheckCmd     `cmd:"" name:"quality-check" help:"Check video quality (resolution, HDR)"`
	Metadata     cmd.MetadataMissingCmd  `cmd:"" name:"metadata-missing" help:"Find items with incomplete metadata"`
	Search       cmd.SearchCmd           `cmd:"" help:"Search the Plex library"`
	Library      cmd.LibraryCmd          `cmd:"" help:"Manage library maintenance operations"`
	ServerInfo   cmd.ServerInfoCmd       `cmd:"" name:"server-info" help:"Show Plex server and library summary information"`
	Playlist     cmd.PlaylistCmd         `cmd:"" help:"Manage playlists"`
	Episodes     cmd.EpisodesListCmd     `cmd:"" help:"List episodes for a show with optional filtering"`
	Movies       cmd.MoviesCmd           `cmd:"" help:"List movies with optional filtering (e.g., by director)"`
	Directors    cmd.DirectorsCmd        `cmd:"" help:"List all directors in a library"`
	Match        cmd.MatchCmd            `cmd:"" help:"Fix metadata matches for items"`
	Editions     cmd.EditionsCmd         `cmd:"" help:"List movies with editions and check for issues"`
	Streams      cmd.StreamsCmd          `cmd:"" help:"Manage audio and subtitle streams"`
	Watch        cmd.WatchCmd            `cmd:"" help:"View watch activity and statistics"`
	Cache        cmd.CacheCmd            `cmd:"" help:"Manage local cache"`
	Auth         cmd.AuthCmd             `cmd:"" help:"Authenticate with Plex and manage stored credentials"`
}

func main() {
	var cli CLI

	parser, err := kong.New(&cli,
		kong.Name("plex"),
		kong.Description("Plex CLI for managing your media server"),
		kong.UsageOnError(),
		kong.Vars{
			"version": version,
		},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating parser: %v\n", err)
		os.Exit(1)
	}

	kctx, err := parser.Parse(os.Args[1:])
	if err != nil {
		parser.FatalIfErrorf(err)
	}

	// Handle version flag
	if cli.Version {
		fmt.Printf("plex version %s\n", version)
		os.Exit(0)
	}

	// Setup output format
	format := outfmt.Table
	if cli.JSON {
		format = outfmt.JSON
	} else if cli.Plain {
		format = outfmt.TSV
	}

	// Setup UI
	colorMode := ui.ColorAuto
	switch cli.Color {
	case "always":
		colorMode = ui.ColorAlways
	case "never":
		colorMode = ui.ColorNever
	}
	u := ui.New(ui.Options{ColorMode: colorMode})

	// Load configuration
	cacheTTLSet := hasCLIFlag(os.Args[1:], "cache-ttl")
	noCacheSet := hasCLIFlag(os.Args[1:], "no-cache")
	refreshCacheSet := hasCLIFlag(os.Args[1:], "refresh-cache")

	cfg, err := loadConfig(
		cli.Config,
		cli.Server,
		cli.Token,
		cli.Timeout,
		cli.CacheTTL,
		cli.NoCache,
		cli.RefreshCache,
		cacheTTLSet,
		noCacheSet,
		refreshCacheSet,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Bind dependencies and run
	kctx.Bind(kctx)
	kctx.Bind(u)
	kctx.Bind(cfg)
	ctx := outfmt.WithMode(context.Background(), format)
	kctx.BindTo(ctx, (*context.Context)(nil))

	if cli.JSON || cli.Plain {
		outputStr := "json"
		if cli.Plain {
			outputStr = "tsv"
		}
		applyOutputFormat(&cli, outputStr)
	}

	err = kctx.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func applyOutputFormat(cli *CLI, format string) {
	cli.Unwatched.Output = format
	cli.Unmatched.Output = format
	cli.Recently.Output = format
	cli.Added.Output = format
	cli.Duplicates.Output = format
	cli.FilePaths.Output = format
	cli.Subtitles.Output = format
	cli.Audio.Output = format
	cli.EpisodesMiss.Output = format
	cli.Quality.Output = format
	cli.Metadata.Output = format
	cli.Search.Output = format
	cli.Library.List.Output = format
	cli.Library.Update.Output = format
	cli.Library.Clean.Output = format
	cli.Library.Status.Output = format
	cli.ServerInfo.Output = format
	cli.Playlist.List.Output = format
	cli.Playlist.Create.Output = format
	cli.Playlist.Smart.Output = format
	cli.Playlist.Add.Output = format
	cli.Playlist.Show.Output = format
	cli.Playlist.Delete.Output = format
	cli.Episodes.Output = format
	cli.Movies.Output = format
	cli.Directors.Output = format
	cli.Match.Search.Output = format
	cli.Editions.Output = format
	cli.Streams.List.Output = format
	cli.Streams.Set.Output = format
	cli.Watch.Now.Output = format
	cli.Watch.History.Output = format
	cli.Watch.Stats.Output = format
	cli.Cache.Clear.Output = format
	cli.Auth.Servers.Output = format
}

// loadConfig loads configuration from file and/or environment/cli flags
func loadConfig(
	_ string,
	serverURL, token string,
	timeout, cacheTTL int,
	noCache, refreshCache bool,
	cacheTTLSet, noCacheSet, refreshCacheSet bool,
) (*config.Config, error) {
	cfg, err := config.ReadConfig()
	if err != nil {
		cfg = &config.Config{}
	}

	if serverURL != "" {
		cfg.ServerURL = serverURL
	}
	if token != "" {
		cfg.Token = token
	}
	if timeout > 0 {
		cfg.Timeout = timeout
	}
	if cacheTTLSet {
		cfg.CacheTTL = cacheTTL
		if cacheTTL == 0 {
			cfg.CacheDisabled = true
		}
	}
	if noCacheSet {
		cfg.CacheDisabled = noCache
	}
	if refreshCacheSet {
		cfg.CacheRefresh = refreshCache
	}

	return cfg, nil
}

func hasCLIFlag(args []string, name string) bool {
	prefix := "--" + name
	for _, arg := range args {
		if arg == prefix || strings.HasPrefix(arg, prefix+"=") {
			return true
		}
	}
	return false
}
