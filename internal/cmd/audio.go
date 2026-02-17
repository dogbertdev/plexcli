package cmd

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type AudioCheckCmd struct {
	Codecs      string `help:"Comma-separated list of audio codecs to check (e.g., aac,ac3,dts)" default:""`
	MinChannels int    `help:"Minimum number of audio channels" default:"2"`
	Section     string `help:"Library section ID to scan (empty = all sections)" default:""`
	Type        string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Limit       int    `help:"Maximum number of items to display" default:"0"`
	Output      string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type AudioInfo struct {
	Title    string `json:"title"`
	Year     int    `json:"year,omitempty"`
	Type     string `json:"type"`
	Codec    string `json:"codec"`
	Channels int    `json:"channels"`
	Status   string `json:"status"`
}

func (c *AudioCheckCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	requestedCodecs := parseCodecList(c.Codecs)

	items, err := c.fetchItems(cc.Ctx, cc.Client)
	if err != nil {
		return err
	}

	results := c.checkAudio(items, requestedCodecs)

	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No items with audio issues found")
		return nil
	}

	if c.Limit > 0 && len(results) > c.Limit {
		results = results[:c.Limit]
	}

	return c.outputResults(u.Out(), results)
}

func (c *AudioCheckCmd) fetchItems(ctx context.Context, client *plexclient.Client) ([]*components.Metadata, error) {
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

func (c *AudioCheckCmd) checkAudio(items []*components.Metadata, requestedCodecs []string) []AudioInfo {
	var results []AudioInfo

	for _, item := range items {
		itemType := anyToString(item.Type)
		if c.Type != "all" && itemType != c.Type {
			continue
		}

		audioInfos := c.extractAudioInfo(item)
		for _, info := range audioInfos {
			if c.hasAudioIssue(info, requestedCodecs) {
				results = append(results, info)
			}
		}
	}

	return results
}

func (c *AudioCheckCmd) extractAudioInfo(item *components.Metadata) []AudioInfo {
	var infos []AudioInfo
	title := audioGetTitle(item)
	year := int64PtrToInt(item.Year)

	if item.Media != nil {
		for _, media := range item.Media {
			if media.Part != nil {
				for _, part := range media.Part {
					if part.Stream != nil {
						for _, stream := range part.Stream {
							// StreamType 2 = audio
							if stream.StreamType != nil && *stream.StreamType == 2 {
								info := AudioInfo{
									Title: title,
									Year:  year,
									Type:  anyToString(item.Type),
								}

								info.Codec = anyToString(stream.Codec)
								if media.AudioChannels != nil {
									info.Channels = int(*media.AudioChannels)
								}

								infos = append(infos, info)
							}
						}
					}
				}
			}
		}
	}

	return infos
}

func (c *AudioCheckCmd) hasAudioIssue(info AudioInfo, requestedCodecs []string) bool {
	if len(requestedCodecs) > 0 {
		codecMatch := false
		for _, codec := range requestedCodecs {
			if strings.EqualFold(info.Codec, codec) {
				codecMatch = true
				break
			}
		}
		if !codecMatch {
			info.Status = fmt.Sprintf("Missing codec: need %s", strings.Join(requestedCodecs, ", "))
			return true
		}
	}

	if info.Channels < c.MinChannels {
		info.Status = fmt.Sprintf("Low channels: %d < %d", info.Channels, c.MinChannels)
		return true
	}

	return false
}

func (c *AudioCheckCmd) outputResults(w io.Writer, results []AudioInfo) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "YEAR", "TYPE", "CODEC", "CHANNELS", "STATUS"}
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
			r.Codec,
			strconv.Itoa(r.Channels),
			r.Status,
		}
	}

	return formatter.Format(w, header, rows, results)
}

func parseCodecList(codecStr string) []string {
	var result []string
	parts := strings.Split(codecStr, ",")
	for _, part := range parts {
		codec := strings.TrimSpace(strings.ToLower(part))
		if codec != "" {
			result = append(result, codec)
		}
	}
	return result
}

func audioGetTitle(item *components.Metadata) string {
	return anyToString(item.Title)
}
