package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alecthomas/kong"
	"github.com/user/plexcli/internal/cmd"
	"github.com/user/plexcli/internal/config"
	"github.com/user/plexcli/internal/outfmt"
	"github.com/user/plexcli/internal/ui"
)

var version = "dev"

// CLI represents the root command structure
type CLI struct {
	Version bool `help:"Show version information" short:"v"`

	// Global flags
	JSON    bool   `help:"Output JSON to stdout" default:"false"`
	Plain   bool   `help:"Output TSV to stdout" default:"false"`
	Color   string `help:"Color mode: auto|always|never" default:"auto" enum:"auto,always,never"`
	Config  string `help:"Path to config file" type:"path" default:""`
	Server  string `help:"Plex server URL" env:"PLEX_SERVER" default:""`
	Token   string `help:"Plex authentication token" env:"PLEX_TOKEN" default:""`
	Timeout int    `help:"Request timeout in seconds" default:"120"`

	// Subcommands
	Unwatched  cmd.UnwatchedCmd        `cmd:"" help:"List unwatched items in the library"`
	Recently   cmd.RecentlyWatchedCmd  `cmd:"" name:"recently-watched" help:"List recently watched items"`
	Added      cmd.RecentlyAddedCmd    `cmd:"" name:"recently-added" help:"List recently added items"`
	Duplicates cmd.DuplicatesCmd       `cmd:"" help:"Find duplicate media files"`
	FilePaths  cmd.FilePathsCmd        `cmd:"" name:"file-paths" help:"List file paths for media items"`
	Subtitles  cmd.SubtitlesMissingCmd `cmd:"" name:"subtitles-missing" help:"Find items missing subtitle languages"`
	Audio      cmd.AudioCheckCmd       `cmd:"" name:"audio-check" help:"Check audio codec and channel configuration"`
	Episodes   cmd.EpisodesMissingCmd  `cmd:"" name:"episodes-missing" help:"Find missing episodes in TV series"`
	Quality    cmd.QualityCheckCmd     `cmd:"" name:"quality-check" help:"Check video quality (resolution, HDR)"`
	Metadata   cmd.MetadataMissingCmd  `cmd:"" name:"metadata-missing" help:"Find items with incomplete metadata"`
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
	cfg, err := loadConfig(cli.Config, cli.Server, cli.Token, cli.Timeout)
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

	err = kctx.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// loadConfig loads configuration from file and/or environment/cli flags
func loadConfig(_, serverURL, token string, timeout int) (*config.Config, error) {
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

	return cfg, nil
}
