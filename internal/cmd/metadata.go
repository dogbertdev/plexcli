package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"
	"github.com/user/plexcli/internal/auth"
	"github.com/user/plexcli/internal/config"
	"github.com/user/plexcli/internal/outfmt"
	"github.com/user/plexcli/internal/plexclient"
	"github.com/user/plexcli/internal/ui"
)

type MetadataMissingCmd struct {
	Section string `help:"Library section ID to scan (empty = all sections)" default:""`
	Type    string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type MetadataInfo struct {
	Title         string   `json:"title"`
	Year          int      `json:"year,omitempty"`
	Type          string   `json:"type"`
	MissingFields []string `json:"missing_fields"`
}

func (c *MetadataMissingCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
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

	items, err := c.fetchItems(fetchCtx, client)
	if err != nil {
		return err
	}

	results := c.checkMetadata(items)

	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No items with missing metadata found")
		return nil
	}

	return c.outputResults(u.Out(), results)
}

func (c *MetadataMissingCmd) fetchItems(ctx context.Context, client *plexclient.Client) ([]*components.Metadata, error) {
	if c.Section != "" {
		return client.GetAllLibraryItems(ctx, c.Section)
	}

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

func (c *MetadataMissingCmd) checkMetadata(items []*components.Metadata) []MetadataInfo {
	var results []MetadataInfo

	for _, item := range items {
		if c.Type != "all" && string(item.Type) != c.Type {
			continue
		}

		missing := c.getMissingFields(item)
		if len(missing) > 0 {
			info := MetadataInfo{
				Title:         metadataGetTitle(item),
				Type:          string(item.Type),
				MissingFields: missing,
			}

			if item.Year != nil {
				info.Year = *item.Year
			}

			results = append(results, info)
		}
	}

	return results
}

func (c *MetadataMissingCmd) getMissingFields(item *components.Metadata) []string {
	var missing []string

	if item.Title == "" {
		missing = append(missing, "title")
	}

	if item.Year == nil {
		missing = append(missing, "year")
	}

	if item.Summary == nil || *item.Summary == "" {
		missing = append(missing, "summary")
	}

	if item.Rating == nil {
		missing = append(missing, "rating")
	}

	if item.Thumb == nil || *item.Thumb == "" {
		missing = append(missing, "thumb")
	}

	if item.Art == nil || *item.Art == "" {
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
	if item.Title != "" {
		return item.Title
	}
	return plexclient.DefaultUnknownTitle
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
