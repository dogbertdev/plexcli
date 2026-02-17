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

type QualityCheckCmd struct {
	MinResolution string `help:"Minimum resolution filter (720p, 1080p, 4k)" default:"1080p" enum:"720p,1080p,4k"`
	HDR           bool   `help:"Only show HDR content"`
	Section       string `help:"Library section ID to scan (empty = all sections)" default:""`
	Type          string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Limit         int    `help:"Maximum number of items to display" default:"0"`
	Output        string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type QualityInfo struct {
	Title      string `json:"title"`
	Year       int    `json:"year,omitempty"`
	Type       string `json:"type"`
	Resolution string `json:"resolution"`
	HDR        bool   `json:"hdr"`
	Bitrate    int    `json:"bitrate"`
	VideoCodec string `json:"video_codec"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

func (c *QualityCheckCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	minResValue := resolutionToValue(c.MinResolution)

	items, err := c.fetchItems(cc.Ctx, cc.Client)
	if err != nil {
		return err
	}

	results := c.checkQuality(items, minResValue)

	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No items found matching quality criteria")
		return nil
	}

	if c.Limit > 0 && len(results) > c.Limit {
		results = results[:c.Limit]
	}

	return c.outputResults(u.Out(), results)
}

func (c *QualityCheckCmd) fetchItems(ctx context.Context, client *plexclient.Client) ([]*components.Metadata, error) {
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

func (c *QualityCheckCmd) checkQuality(items []*components.Metadata, minResValue int) []QualityInfo {
	var results []QualityInfo

	for _, item := range items {
		itemType := anyToString(item.Type)
		if c.Type != "all" && itemType != c.Type {
			continue
		}

		info := c.extractQualityInfo(item)
		if info.meetsCriteria(minResValue, c.HDR) {
			results = append(results, info)
		}
	}

	return results
}

func (c *QualityCheckCmd) extractQualityInfo(item *components.Metadata) QualityInfo {
	info := QualityInfo{
		Title: qualityGetTitle(item),
		Type:  anyToString(item.Type),
	}

	if item.Year != nil {
		info.Year = int(*item.Year)
	}

	if len(item.Media) > 0 {
		media := item.Media[0]

		info.Resolution = anyToString(media.VideoResolution)

		if media.Bitrate != nil {
			info.Bitrate = int(*media.Bitrate)
		}

		if media.Width != nil {
			info.Width = int(*media.Width)
		}

		if media.Height != nil {
			info.Height = int(*media.Height)
		}

		if len(media.Part) > 0 {
			part := media.Part[0]
			if part.Stream != nil {
				for _, stream := range part.Stream {
					if stream.StreamType != nil && *stream.StreamType == 1 {
						info.VideoCodec = anyToString(stream.Codec)
					}
				}
			}
		}
	}

	return info
}

func (q QualityInfo) meetsCriteria(minResValue int, requireHDR bool) bool {
	resValue := resolutionToValue(q.Resolution)

	if resValue < minResValue {
		return false
	}

	if requireHDR && !q.HDR {
		return false
	}

	return true
}

func (c *QualityCheckCmd) outputResults(w io.Writer, results []QualityInfo) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "YEAR", "TYPE", "RESOLUTION", "HDR", "BITRATE", "CODEC"}
	rows := make([][]string, len(results))

	for i, r := range results {
		yearStr := ""
		if r.Year > 0 {
			yearStr = fmt.Sprintf("%d", r.Year)
		}

		hdrStr := "No"
		if r.HDR {
			hdrStr = "Yes"
		}

		bitrateStr := ""
		if r.Bitrate > 0 {
			bitrateStr = fmt.Sprintf("%d kbps", r.Bitrate)
		}

		rows[i] = []string{
			r.Title,
			yearStr,
			r.Type,
			r.Resolution,
			hdrStr,
			bitrateStr,
			r.VideoCodec,
		}
	}

	return formatter.Format(w, header, rows, results)
}

func resolutionToValue(res string) int {
	res = strings.ToLower(res)
	switch {
	case strings.Contains(res, "4k") || strings.Contains(res, "2160"):
		return 2160
	case strings.Contains(res, "1080"):
		return 1080
	case strings.Contains(res, "720"):
		return 720
	case strings.Contains(res, "480"):
		return 480
	default:
		return 0
	}
}

func qualityGetTitle(item *components.Metadata) string {
	title := anyToString(item.Title)
	if title == "" {
		return "Unknown"
	}
	return title
}
