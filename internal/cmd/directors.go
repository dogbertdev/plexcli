package cmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/alecthomas/kong"
	"github.com/user/plexcli/internal/auth"
	"github.com/user/plexcli/internal/config"
	"github.com/user/plexcli/internal/outfmt"
	"github.com/user/plexcli/internal/plexclient"
	"github.com/user/plexcli/internal/ui"
)

// DirectorsCmd lists all directors in a library
type DirectorsCmd struct {
	Section string `help:"Library section ID (required)" short:"s" required:""`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type DirectorItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *DirectorsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	authCtx, cancel := context.WithTimeout(context.Background(), auth.DefaultTimeout)
	defer cancel()

	token, err := auth.GetToken(authCtx, *cfg)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	if cfg.Timeout == 0 {
		timeout = plexclient.DefaultTimeout
	}

	client, err := plexclient.NewClient(cfg.ServerURL, token, plexclient.WithTimeout(timeout))
	if err != nil {
		return fmt.Errorf("failed to create plex client: %w", err)
	}

	fetchCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	directors, err := client.GetDirectors(fetchCtx, c.Section)
	if err != nil {
		return fmt.Errorf("failed to get directors: %w", err)
	}

	if len(directors) == 0 {
		fmt.Fprintln(u.Err(), "No directors found in this library")
		return nil
	}

	outputItems := make([]DirectorItem, 0, len(directors))
	for _, d := range directors {
		outputItems = append(outputItems, DirectorItem{
			ID:   d.ID,
			Name: d.Name,
		})
	}

	return c.output(u.Out(), outputItems)
}

func (c *DirectorsCmd) output(w io.Writer, items []DirectorItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"ID", "NAME"}
	rows := make([][]string, 0, len(items))

	for _, item := range items {
		rows = append(rows, []string{
			item.ID,
			item.Name,
		})
	}

	return formatter.Format(w, header, rows, items)
}
