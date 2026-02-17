package cmd

import (
	"context"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type RecentlyWatchedCmd struct {
	Section string `help:"Filter by library section ID" default:""`
	Type    string `help:"Filter by type: movie, tv, or all" default:"all" enum:"movie,tv,all"`
	Limit   int    `help:"Maximum number of items to show" default:"50"`
	Days    int    `help:"Filter items watched within the last N days (0 = no filter)" default:"0"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type RecentlyWatchedItem struct {
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	Library   string    `json:"library"`
	WatchedAt time.Time `json:"watched_at"`
}

func (c *RecentlyWatchedCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	items, err := c.fetchHistory(cc.Ctx, cc.Client)
	if err != nil {
		return err
	}

	processed := c.processHistory(cc.Ctx, items, cc.Client)

	if len(processed) == 0 {
		fmt.Fprintln(u.Err(), "No recently watched items found")
		return nil
	}

	if c.Limit > 0 && len(processed) > c.Limit {
		processed = processed[:c.Limit]
	}

	return c.output(u.Out(), processed)
}

func (c *RecentlyWatchedCmd) fetchHistory(ctx context.Context, client *plexclient.Client) ([]plexclient.HistoryItem, error) {
	history, err := client.GetHistory(ctx, c.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	return history, nil
}

func (c *RecentlyWatchedCmd) processHistory(ctx context.Context, history []plexclient.HistoryItem, client *plexclient.Client) []RecentlyWatchedItem {
	var items []RecentlyWatchedItem
	cutoffTime := time.Time{}

	if c.Days > 0 {
		cutoffTime = time.Now().AddDate(0, 0, -c.Days)
	}

	var sections []plexclient.Library
	if len(history) > 0 && client != nil {
		sections, _ = client.GetSections(ctx)
	}

	for _, h := range history {
		if !c.matchesType(h.Type) {
			continue
		}

		watchedAt := time.Unix(h.ViewedAt, 0)
		if c.Days > 0 && watchedAt.Before(cutoffTime) {
			continue
		}

		item := RecentlyWatchedItem{
			Title:     c.formatTitle(h),
			Type:      h.Type,
			WatchedAt: watchedAt,
		}

		libraryID := ""
		if h.LibrarySectionID != nil {
			libraryID = *h.LibrarySectionID
		}

		if libraryID != "" {
			for _, section := range sections {
				if section.ID == libraryID && section.Title != nil {
					item.Library = *section.Title
					break
				}
			}
		}

		if item.Library == "" {
			item.Library = "unknown"
		}

		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].WatchedAt.After(items[j].WatchedAt)
	})

	if c.Limit > 0 && len(items) > c.Limit {
		items = items[:c.Limit]
	}

	return items
}

func (c *RecentlyWatchedCmd) formatTitle(h plexclient.HistoryItem) string {
	if h.GrandparentTitle != "" && h.Title != "" {
		return h.GrandparentTitle + " - " + h.Title
	}
	if h.Title != "" {
		return h.Title
	}
	return "Unknown"
}

func (c *RecentlyWatchedCmd) matchesType(itemType string) bool {
	if c.Type == "all" {
		return true
	}
	if c.Type == "tv" {
		return itemType == "episode" || itemType == "show" || itemType == "season"
	}
	return itemType == c.Type
}

func (c *RecentlyWatchedCmd) output(w io.Writer, items []RecentlyWatchedItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "TYPE", "LIBRARY", "WATCHED AT"}
	rows := make([][]string, len(items))

	for i, item := range items {
		rows[i] = []string{
			item.Title,
			item.Type,
			item.Library,
			item.WatchedAt.Format("2006-01-02 15:04"),
		}
	}

	return formatter.Format(w, header, rows, items)
}
