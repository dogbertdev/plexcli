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

type MetadataMissingCmd struct {
	Section string `help:"Library section ID to scan (empty = all sections)" default:""`
	Type    string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Limit   int    `help:"Maximum number of items to display" default:"0"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type MetadataInfo struct {
	Title         string   `json:"title"`
	Year          int      `json:"year,omitempty"`
	Type          string   `json:"type"`
	MissingFields []string `json:"missing_fields"`
}

func (c *MetadataMissingCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	items, err := c.fetchItems(cc.Ctx, cc.Client)
	if err != nil {
		return err
	}

	results := c.checkMetadata(items)

	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No items with missing metadata found")
		return nil
	}

	if c.Limit > 0 && len(results) > c.Limit {
		results = results[:c.Limit]
	}

	return c.outputResults(u.Out(), results)
}

func (c *MetadataMissingCmd) fetchItems(ctx context.Context, client *plexclient.Client) ([]*components.Metadata, error) {
	return fetchLibraryItems(ctx, client, c.Section)
}

func (c *MetadataMissingCmd) checkMetadata(items []*components.Metadata) []MetadataInfo {
	var results []MetadataInfo

	for _, item := range items {
		itemType := anyToString(item.Type)
		if c.Type != "all" && itemType != c.Type {
			continue
		}

		missing := c.getMissingFields(item)
		if len(missing) > 0 {
			info := MetadataInfo{
				Title:         metadataGetTitle(item),
				Type:          itemType,
				MissingFields: missing,
			}

			if item.Year != nil {
				info.Year = int(*item.Year)
			}

			results = append(results, info)
		}
	}

	return results
}

func (c *MetadataMissingCmd) getMissingFields(item *components.Metadata) []string {
	var missing []string

	if anyToString(item.Title) == "" {
		missing = append(missing, "title")
	}

	if item.Year == nil {
		missing = append(missing, "year")
	}

	if item.Summary == nil || anyToString(item.Summary) == "" {
		missing = append(missing, "summary")
	}

	if item.Rating == nil {
		missing = append(missing, "rating")
	}

	if item.Thumb == nil || anyToString(item.Thumb) == "" {
		missing = append(missing, "thumb")
	}

	if item.Art == nil || anyToString(item.Art) == "" {
		missing = append(missing, "art")
	}

	return missing
}

func (c *MetadataMissingCmd) outputResults(w io.Writer, results []MetadataInfo) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "YEAR", "TYPE", "MISSING FIELDS"}
	rows := make([][]string, len(results))

	for i, r := range results {
		yearStr := ""
		if r.Year > 0 {
			yearStr = fmt.Sprintf("%d", r.Year)
		}

		missingStr := ""
		if len(r.MissingFields) > 0 {
			missingStr = joinMetadataStrings(r.MissingFields, ", ")
		}

		rows[i] = []string{
			r.Title,
			yearStr,
			r.Type,
			missingStr,
		}
	}

	return formatter.Format(w, header, rows, results)
}

func metadataGetTitle(item *components.Metadata) string {
	title := anyToString(item.Title)
	if title == "" {
		return "Unknown"
	}
	return title
}

func joinMetadataStrings(parts []string, separator string) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(parts[0])
	for i := 1; i < len(parts); i++ {
		b.WriteString(separator)
		b.WriteString(parts[i])
	}
	return b.String()
}
