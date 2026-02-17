package cmd

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"
	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type RecentlyAddedCmd struct {
	Limit  int    `help:"Maximum number of items to show" default:"50"`
	Days   int    `help:"Only show items added in the last N days" default:"7"`
	Type   string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type RecentlyAddedItem struct {
	Title   string    `json:"title"`
	Year    int       `json:"year,omitempty"`
	Type    string    `json:"type"`
	Section string    `json:"section"`
	AddedAt time.Time `json:"added_at"`
}

type itemWithSection struct {
	item    *components.Metadata
	section string
}

func (c *RecentlyAddedCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	items, err := c.fetchRecentlyAdded(cc.Ctx, cc.Client)
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

func (c *RecentlyAddedCmd) fetchRecentlyAdded(ctx context.Context, client *plexclient.Client) ([]itemWithSection, error) {
	sections, err := client.GetSections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sections: %w", err)
	}

	var allItems []itemWithSection
	for _, section := range sections {
		sectionName := ""
		if section.Title != nil {
			sectionName = *section.Title
		}
		items, err := client.GetAllLibraryItems(ctx, section.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get items from section %s: %w", section.ID, err)
		}
		for _, item := range items {
			allItems = append(allItems, itemWithSection{item: item, section: sectionName})
		}
	}

	return allItems, nil
}

func (c *RecentlyAddedCmd) processItems(items []itemWithSection) []RecentlyAddedItem {
	cutoffTime := time.Now().AddDate(0, 0, -c.Days)
	var results []RecentlyAddedItem

	for _, iws := range items {
		item := iws.item
		itemType := anyToString(item.Type)
		if c.Type != "all" && itemType != c.Type {
			continue
		}

		addedAt := time.Time{}
		if item.AddedAt != nil && *item.AddedAt > 0 {
			addedAt = time.Unix(*item.AddedAt, 0)
		}

		if addedAt.Before(cutoffTime) {
			continue
		}

		result := RecentlyAddedItem{
			Title:   recentlyGetTitle(item),
			Type:    itemType,
			Section: iws.section,
			AddedAt: addedAt,
		}

		if item.Year != nil {
			result.Year = int(*item.Year)
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

	header := []string{"TITLE", "YEAR", "TYPE", "SECTION", "ADDED AT"}
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
			r.Section,
			addedAtStr,
		}
	}

	return formatter.Format(w, header, rows, results)
}

func recentlyGetTitle(item *components.Metadata) string {
	title := anyToString(item.Title)
	if title == "" {
		return "Unknown"
	}
	return title
}
