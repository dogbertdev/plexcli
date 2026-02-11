package cmd

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"
	"github.com/user/plexcli/internal/config"
	"github.com/user/plexcli/internal/outfmt"
	"github.com/user/plexcli/internal/ui"
)

type UnwatchedCmd struct {
	Type    string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Section string `help:"Filter by library section ID" default:""`
	Limit   int    `help:"Maximum number of items to display" default:"50"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type UnwatchedItem struct {
	Title   string `json:"title"`
	Year    int    `json:"year,omitempty"`
	Type    string `json:"type"`
	AddedAt string `json:"added_at"`
}

func (c *UnwatchedCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	var items []*components.Metadata
	if c.Section != "" {
		items, err = cc.Client.GetAllLibraryItems(cc.Ctx, c.Section)
	} else {
		sections, err := cc.Client.GetSections(cc.Ctx)
		if err != nil {
			return fmt.Errorf("failed to get sections: %w", err)
		}
		for _, section := range sections {
			sectionItems, err := cc.Client.GetAllLibraryItems(cc.Ctx, section.ID)
			if err != nil {
				return fmt.Errorf("failed to get items from section %s: %w", section.ID, err)
			}
			items = append(items, sectionItems...)
		}
	}
	if err != nil {
		return fmt.Errorf("failed to fetch library items: %w", err)
	}

	unwatched := c.filterUnwatched(items)
	unwatched = c.filterByType(unwatched)
	unwatched = c.sortByAddedDate(unwatched)

	if len(unwatched) > c.Limit {
		unwatched = unwatched[:c.Limit]
	}

	if len(unwatched) == 0 {
		fmt.Fprintln(u.Err(), "No unwatched items found")
		return nil
	}

	outputItems := c.toOutputItems(unwatched)
	return c.output(u.Out(), outputItems)
}

func (c *UnwatchedCmd) filterUnwatched(items []*components.Metadata) []*components.Metadata {
	var unwatched []*components.Metadata
	for _, item := range items {
		if item == nil {
			continue
		}
		if item.ViewCount == nil || *item.ViewCount == 0 {
			unwatched = append(unwatched, item)
		}
	}
	return unwatched
}

func (c *UnwatchedCmd) filterByType(items []*components.Metadata) []*components.Metadata {
	if c.Type == "all" {
		return items
	}

	var filtered []*components.Metadata
	for _, item := range items {
		if item == nil {
			continue
		}

		switch c.Type {
		case "movie":
			if string(item.Type) == "movie" {
				filtered = append(filtered, item)
			}
		case "episode":
			if string(item.Type) == "episode" || string(item.Type) == "show" {
				filtered = append(filtered, item)
			}
		}
	}
	return filtered
}

func (c *UnwatchedCmd) sortByAddedDate(items []*components.Metadata) []*components.Metadata {
	sorted := make([]*components.Metadata, len(items))
	copy(sorted, items)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].AddedAt > sorted[j].AddedAt
	})

	return sorted
}

func (c *UnwatchedCmd) toOutputItems(items []*components.Metadata) []UnwatchedItem {
	output := make([]UnwatchedItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}

		ui := UnwatchedItem{
			Title: item.Title,
			Type:  string(item.Type),
		}

		if item.Year != nil {
			ui.Year = *item.Year
		}

		if item.AddedAt > 0 {
			t := time.Unix(item.AddedAt, 0)
			ui.AddedAt = t.Format("2006-01-02")
		}

		output = append(output, ui)
	}
	return output
}

func (c *UnwatchedCmd) output(w io.Writer, items []UnwatchedItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "YEAR", "TYPE", "ADDED AT"}
	rows := make([][]string, 0, len(items))

	for _, item := range items {
		yearStr := ""
		if item.Year > 0 {
			yearStr = fmt.Sprintf("%d", item.Year)
		}
		rows = append(rows, []string{
			item.Title,
			yearStr,
			item.Type,
			item.AddedAt,
		})
	}

	return formatter.Format(w, header, rows, items)
}
