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

type SearchCmd struct {
	Query   string `arg:"" help:"Search query"`
	Section string `help:"Filter by library section ID" default:""`
	Type    string `help:"Filter by type: movie, show, episode, artist, album, track, or all" default:"all" enum:"movie,show,episode,artist,album,track,all"`
	Limit   int    `help:"Maximum number of results" default:"50"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type SearchItem struct {
	RatingKey string `json:"rating_key"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Year      int    `json:"year,omitempty"`
	Show      string `json:"show,omitempty"`
	Season    int    `json:"season,omitempty"`
	Episode   int    `json:"episode,omitempty"`
}

func (c *SearchCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
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

	var sectionID *string
	if c.Section != "" {
		sectionID = &c.Section
	}

	results, err := client.SearchLibrary(fetchCtx, c.Query, sectionID, c.Limit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	// Filter by type if specified
	if c.Type != "all" {
		filtered := make([]plexclient.SearchResult, 0)
		for _, r := range results {
			if r.Type == c.Type {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}

	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No results found")
		return nil
	}

	// Apply limit
	if len(results) > c.Limit {
		results = results[:c.Limit]
	}

	outputItems := c.toOutputItems(results)
	return c.output(u.Out(), outputItems)
}

func (c *SearchCmd) toOutputItems(results []plexclient.SearchResult) []SearchItem {
	output := make([]SearchItem, 0, len(results))
	for _, r := range results {
		item := SearchItem{
			RatingKey: r.RatingKey,
			Title:     r.Title,
			Type:      r.Type,
		}

		if r.Year != nil {
			item.Year = *r.Year
		}

		if r.GrandparentTitle != nil {
			item.Show = *r.GrandparentTitle
		}

		if r.ParentIndex != nil {
			item.Season = *r.ParentIndex
		}

		if r.Index != nil {
			item.Episode = *r.Index
		}

		output = append(output, item)
	}
	return output
}

func (c *SearchCmd) output(w io.Writer, items []SearchItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"RATING KEY", "TITLE", "TYPE", "YEAR", "SHOW", "S", "E"}
	rows := make([][]string, 0, len(items))

	for _, item := range items {
		yearStr := ""
		if item.Year > 0 {
			yearStr = fmt.Sprintf("%d", item.Year)
		}
		seasonStr := ""
		if item.Season > 0 {
			seasonStr = fmt.Sprintf("%d", item.Season)
		}
		episodeStr := ""
		if item.Episode > 0 {
			episodeStr = fmt.Sprintf("%d", item.Episode)
		}
		rows = append(rows, []string{
			item.RatingKey,
			item.Title,
			item.Type,
			yearStr,
			item.Show,
			seasonStr,
			episodeStr,
		})
	}

	return formatter.Format(w, header, rows, items)
}
