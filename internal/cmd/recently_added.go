package cmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"
	"github.com/user/plexcli/internal/auth"
	"github.com/user/plexcli/internal/config"
	"github.com/user/plexcli/internal/outfmt"
	"github.com/user/plexcli/internal/plexclient"
	"github.com/user/plexcli/internal/ui"
)

// RecentlyAddedCmd represents the recently added command
type RecentlyAddedCmd struct {
	Limit  int    `help:"Maximum number of items to show" default:"50"`
	Days   int    `help:"Only show items added in the last N days" default:"7"`
	Type   string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

// RecentlyAddedItem represents a recently added item
type RecentlyAddedItem struct {
	Title   string    `json:"title"`
	Year    int       `json:"year,omitempty"`
	Type    string    `json:"type"`
	AddedAt time.Time `json:"added_at"`
}

// Run executes the recently added command
func (c *RecentlyAddedCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	authCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	items, err := c.fetchRecentlyAdded(fetchCtx, client)
	if err != nil {
		return err
	}

	results := c.processItems(items)

	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No recently added items found")
		return nil
	}

	return c.outputResults(u.Out(), results)
}

func (c *RecentlyAddedCmd) fetchRecentlyAdded(ctx context.Context, client *plexclient.Client) ([]*components.Metadata, error) {
	sections, err := client.GetSections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sections: %w", err)
	}

	var allItems []*components.Metadata
	for _, section := range sections {
		items, err := client.GetAllLibraryItems(ctx, section.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get items from section %s: %w", section.ID, err)
		}
		allItems = append(allItems, items...)
	}

	return allItems, nil
}

func (c *RecentlyAddedCmd) processItems(items []*components.Metadata) []RecentlyAddedItem {
	cutoffTime := time.Now().AddDate(0, 0, -c.Days)
	var results []RecentlyAddedItem

	for _, item := range items {
		if c.Type != "all" && string(item.Type) != c.Type {
			continue
		}

		addedAt := time.Time{}
		if item.AddedAt > 0 {
			addedAt = time.Unix(item.AddedAt, 0)
		}

		if addedAt.Before(cutoffTime) {
			continue
		}

		result := RecentlyAddedItem{
			Title:   recentlyGetTitle(item),
			Type:    string(item.Type),
			AddedAt: addedAt,
		}

		if item.Year != nil {
			result.Year = *item.Year
		}

		results = append(results, result)

		if c.Limit > 0 && len(results) >= c.Limit {
			break
		}
	}

	return results
}

func (c *RecentlyAddedCmd) outputResults(w io.Writer, results []RecentlyAddedItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "YEAR", "TYPE", "ADDED AT"}
	rows := make([][]string, len(results))

	for i, r := range results {
		yearStr := ""
		if r.Year > 0 {
			yearStr = fmt.Sprintf("%d", r.Year)
		}

		addedAtStr := ""
		if !r.AddedAt.IsZero() {
			addedAtStr = r.AddedAt.Format("2006-01-02")
		}

		rows[i] = []string{
			r.Title,
			yearStr,
			r.Type,
			addedAtStr,
		}
	}

	return formatter.Format(w, header, rows, results)
}

func recentlyGetTitle(item *components.Metadata) string {
	if item.Title != "" {
		return item.Title
	}
	return "Unknown"
}
