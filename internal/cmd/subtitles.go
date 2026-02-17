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

type SubtitlesMissingCmd struct {
	Lang    string `help:"Comma-separated list of language codes to check (e.g., en,de,fr)" required:"true"`
	Section string `help:"Library section ID to scan (empty = all sections)" default:""`
	Type    string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Limit   int    `help:"Maximum number of items to display" default:"0"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type SubtitleInfo struct {
	Title         string   `json:"title"`
	Year          int      `json:"year,omitempty"`
	Type          string   `json:"type"`
	AvailableSubs []string `json:"available_subs"`
	MissingSubs   []string `json:"missing_subs"`
}

func (c *SubtitlesMissingCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	requestedLangs := parseLangList(c.Lang)
	if len(requestedLangs) == 0 {
		return fmt.Errorf("no valid language codes provided")
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	items, err := c.fetchItems(cc.Ctx, cc.Client)
	if err != nil {
		return err
	}

	results := c.checkSubtitles(cc.Ctx, items, cc.Client, requestedLangs)

	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No items with missing subtitles found")
		return nil
	}

	if c.Limit > 0 && len(results) > c.Limit {
		results = results[:c.Limit]
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

func (c *SubtitlesMissingCmd) checkSubtitles(ctx context.Context, items []*components.Metadata, client *plexclient.Client, requestedLangs []string) []SubtitleInfo {
	var results []SubtitleInfo

	for _, item := range items {
		itemType := anyToString(item.Type)
		if c.Type != "all" && itemType != c.Type {
			continue
		}

		ratingKey := anyToString(item.RatingKey)
		if ratingKey == "" {
			continue
		}

		detailedItem, err := client.GetItemMetadata(ctx, ratingKey)
		if err != nil || detailedItem == nil {
			continue
		}

		info := c.extractSubtitleInfo(detailedItem, requestedLangs)
		if len(info.MissingSubs) > 0 {
			results = append(results, info)
		}

		if c.Limit > 0 && len(results) >= c.Limit {
			break
		}
	}

	return results
}

func (c *SubtitlesMissingCmd) extractSubtitleInfo(item *components.Metadata, requestedLangs []string) SubtitleInfo {
	info := SubtitleInfo{
		Title: getTitle(item),
		Type:  anyToString(item.Type),
	}

	if item.Year != nil {
		info.Year = int(*item.Year)
	}

	availableLangs := make(map[string]bool)

	if item.Media != nil {
		for _, media := range item.Media {
			if media.Part != nil {
				for _, part := range media.Part {
					if part.Stream != nil {
						for _, stream := range part.Stream {
							// StreamType 3 = subtitles
							if stream.StreamType != nil && *stream.StreamType == 3 {
								lang := anyToString(stream.LanguageCode)
								if lang == "" {
									lang = anyToString(stream.Language)
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
		normalizedLang := normalizeLangCode(lang)
		if !availableLangs[lang] && !availableLangs[normalizedLang] {
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

var langCodeMap = map[string]string{
	"en": "eng", "de": "deu", "fr": "fra", "es": "spa", "it": "ita",
	"pt": "por", "ru": "rus", "ja": "jpn", "ko": "kor", "zh": "zho",
	"pl": "pol", "nl": "nld", "sv": "swe", "da": "dan", "no": "nor",
	"fi": "fin", "cs": "ces", "sk": "slk", "hu": "hun", "ro": "ron",
	"bg": "bul", "hr": "hrv", "sl": "slv", "uk": "ukr", "el": "ell",
	"tr": "tur", "ar": "ara", "he": "heb", "th": "tha", "vi": "vie",
}

func normalizeLangCode(code string) string {
	code = strings.ToLower(code)
	if mapped, ok := langCodeMap[code]; ok {
		return mapped
	}
	return code
}

func getTitle(item *components.Metadata) string {
	title := anyToString(item.Title)
	if title == "" {
		return "Unknown"
	}
	return title
}
