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

// SubtitlesMissingCmd represents the subtitles missing command
type SubtitlesMissingCmd struct {
	Lang    string `help:"Comma-separated list of language codes to check (e.g., en,de,fr)" required:"true"`
	Section string `help:"Library section ID to scan (empty = all sections)" default:""`
	Type    string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

// SubtitleInfo represents subtitle information for an item
type SubtitleInfo struct {
	Title         string   `json:"title"`
	Year          int      `json:"year,omitempty"`
	Type          string   `json:"type"`
	AvailableSubs []string `json:"available_subs"`
	MissingSubs   []string `json:"missing_subs"`
}

// Run executes the subtitles missing command
func (c *SubtitlesMissingCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
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

	requestedLangs := parseLangList(c.Lang)
	if len(requestedLangs) == 0 {
		return fmt.Errorf("no valid language codes provided")
	}

	fetchCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	items, err := c.fetchItems(fetchCtx, client)
	if err != nil {
		return err
	}

	results := c.checkSubtitles(items, requestedLangs)

	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No items with missing subtitles found")
		return nil
	}

	return c.outputResults(u.Out(), results)
}

func (c *SubtitlesMissingCmd) fetchItems(ctx context.Context, client *plexclient.Client) ([]*components.Metadata, error) {
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

func (c *SubtitlesMissingCmd) checkSubtitles(items []*components.Metadata, requestedLangs []string) []SubtitleInfo {
	var results []SubtitleInfo

	for _, item := range items {
		if c.Type != "all" && string(item.Type) != c.Type {
			continue
		}

		info := c.extractSubtitleInfo(item, requestedLangs)
		if len(info.MissingSubs) > 0 {
			results = append(results, info)
		}
	}

	return results
}

func (c *SubtitlesMissingCmd) extractSubtitleInfo(item *components.Metadata, requestedLangs []string) SubtitleInfo {
	info := SubtitleInfo{
		Title: getTitle(item),
		Type:  string(item.Type),
	}

	if item.Year != nil {
		info.Year = *item.Year
	}

	availableLangs := make(map[string]bool)

	if item.Media != nil {
		for _, media := range item.Media {
			if media.Part != nil {
				for _, part := range media.Part {
					if part.Stream != nil {
						for _, stream := range part.Stream {
							if stream.StreamType == 3 {
								lang := ""
								if stream.LanguageCode != nil {
									lang = *stream.LanguageCode
								} else if stream.Language != nil {
									lang = *stream.Language
								}
								if lang != "" {
									availableLangs[strings.ToLower(lang)] = true
								}
							}
						}
					}
				}
			}
		}
	}

	for lang := range availableLangs {
		info.AvailableSubs = append(info.AvailableSubs, lang)
	}

	for _, lang := range requestedLangs {
		if !availableLangs[lang] {
			info.MissingSubs = append(info.MissingSubs, lang)
		}
	}

	return info
}

func (c *SubtitlesMissingCmd) outputResults(w io.Writer, results []SubtitleInfo) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "YEAR", "TYPE", "AVAILABLE SUBS", "MISSING SUBS"}
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
			strings.Join(r.AvailableSubs, ","),
			strings.Join(r.MissingSubs, ","),
		}
	}

	return formatter.Format(w, header, rows, results)
}

func parseLangList(langStr string) []string {
	var result []string
	parts := strings.Split(langStr, ",")
	for _, part := range parts {
		lang := strings.TrimSpace(strings.ToLower(part))
		if lang != "" {
			result = append(result, lang)
		}
	}
	return result
}

func getTitle(item *components.Metadata) string {
	if item.Title != "" {
		return item.Title
	}
	return "Unknown"
}
