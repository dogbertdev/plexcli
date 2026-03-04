package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type UnmatchedCmd struct {
	Section string `help:"Library section ID to scan (empty = all sections)" default:""`
	Type    string `help:"Filter by type: movie, show, episode, or all" default:"all" enum:"movie,show,episode,all"`
	Limit   int    `help:"Maximum number of items to display" default:"0"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type UnmatchedInfo struct {
	Title     string `json:"title"`
	Year      int    `json:"year,omitempty"`
	Type      string `json:"type"`
	RatingKey string `json:"rating_key"`
	GUID      string `json:"guid,omitempty"`
}

func (c *UnmatchedCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	items, err := c.fetchItems(cc.Ctx, cc.Client)
	if err != nil {
		return err
	}

	results := c.findUnmatched(items)
	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No unmatched items found")
		return nil
	}

	if c.Limit > 0 && len(results) > c.Limit {
		results = results[:c.Limit]
	}

	return c.outputResults(u.Out(), results)
}

func (c *UnmatchedCmd) fetchItems(ctx context.Context, client *plexclient.Client) ([]*components.Metadata, error) {
	return fetchLibraryItems(ctx, client, c.Section)
}

func (c *UnmatchedCmd) findUnmatched(items []*components.Metadata) []UnmatchedInfo {
	results := make([]UnmatchedInfo, 0)
	filterType := c.Type
	if filterType == "" {
		filterType = "all"
	}

	for _, item := range items {
		if item == nil {
			continue
		}

		itemType := anyToString(item.Type)
		if filterType != "all" && itemType != filterType {
			continue
		}
		if !isMetadataUnmatched(item) {
			continue
		}

		info := UnmatchedInfo{
			Title:     metadataGetTitle(item),
			Type:      itemType,
			RatingKey: anyToString(item.RatingKey),
			GUID:      metadataGUID(item),
		}
		if item.Year != nil {
			info.Year = int(*item.Year)
		}
		results = append(results, info)
	}

	return results
}

func (c *UnmatchedCmd) outputResults(w io.Writer, results []UnmatchedInfo) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "YEAR", "TYPE", "RATING KEY", "GUID"}
	rows := make([][]string, len(results))

	for i, r := range results {
		yearStr := ""
		if r.Year > 0 {
			yearStr = fmt.Sprintf("%d", r.Year)
		}

		rows[i] = []string{
			r.Title,
			yearStr,
			r.Type,
			r.RatingKey,
			r.GUID,
		}
	}

	return formatter.Format(w, header, rows, results)
}

func isMetadataUnmatched(item *components.Metadata) bool {
	guid := strings.ToLower(strings.TrimSpace(metadataGUID(item)))
	if guid != "" {
		return strings.HasPrefix(guid, "local://")
	}

	if len(item.GUID) == 0 {
		return true
	}

	for _, g := range item.GUID {
		tag := strings.ToLower(strings.TrimSpace(anyToString(g.Tag)))
		if tag == "" {
			continue
		}
		if !strings.HasPrefix(tag, "local://") {
			return false
		}
	}

	return true
}

func metadataGUID(item *components.Metadata) string {
	if item == nil || item.AdditionalProperties == nil {
		return ""
	}
	guid, ok := item.AdditionalProperties["guid"]
	if !ok {
		return ""
	}
	return anyToString(guid)
}
