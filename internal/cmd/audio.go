package cmd

import (
	"context"
	"fmt"
	"io"
	"strconv"
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

// AudioCheckCmd represents the audio check command
type AudioCheckCmd struct {
	Codecs      string `help:"Comma-separated list of audio codecs to check (e.g., aac,ac3,dts)" default:""`
	MinChannels int    `help:"Minimum number of audio channels" default:"2"`
	Section     string `help:"Library section ID to scan (empty = all sections)" default:""`
	Type        string `help:"Filter by type: movie, episode, or all" default:"all" enum:"movie,episode,all"`
	Output      string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

// AudioInfo represents audio information for an item
type AudioInfo struct {
	Title    string `json:"title"`
	Year     int    `json:"year,omitempty"`
	Type     string `json:"type"`
	Codec    string `json:"codec"`
	Channels int    `json:"channels"`
	Status   string `json:"status"`
}

// Run executes the audio check command
func (c *AudioCheckCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
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

	requestedCodecs := parseCodecList(c.Codecs)

	fetchCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	items, err := c.fetchItems(fetchCtx, client)
	if err != nil {
		return err
	}

	results := c.checkAudio(items, requestedCodecs)

	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No items with audio issues found")
		return nil
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
		if c.Type != "all" && string(item.Type) != c.Type {
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
	year := 0
	if item.Year != nil {
		year = *item.Year
	}

	if item.Media != nil {
		for _, media := range item.Media {
			if media.Part != nil {
				for _, part := range media.Part {
					if part.Stream != nil {
						for _, stream := range part.Stream {
							if stream.StreamType == 2 {
								info := AudioInfo{
									Title: title,
									Year:  year,
									Type:  string(item.Type),
								}

								info.Codec = stream.Codec
								if stream.Channels != nil {
									info.Channels = *stream.Channels
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
	if item.Title != "" {
		return item.Title
	}
	return "Unknown"
}
