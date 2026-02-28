package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type StreamsCmd struct {
	List StreamsListCmd `cmd:"" help:"List available audio and subtitle streams for an item"`
	Set  StreamsSetCmd  `cmd:"" help:"Set default audio and/or subtitle streams for episodes"`
}

type StreamsListCmd struct {
	RatingKey string `arg:"" help:"Rating key of the item to list streams for"`
	Output    string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type StreamsSetCmd struct {
	Show     string `arg:"" help:"Show name or rating key"`
	Season   int    `help:"Season number (required)" short:"s" required:""`
	Audio    string `help:"Audio stream to select (by language code, e.g., 'eng', 'jpn')" short:"a"`
	Subtitle string `help:"Subtitle stream to select (by language code, e.g., 'eng', 'off')" short:"t"`
	DryRun   bool   `help:"Show what would be changed without making changes" short:"n"`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type StreamInfo struct {
	ID           int    `json:"id"`
	StreamType   string `json:"stream_type"`
	Language     string `json:"language"`
	LanguageCode string `json:"language_code"`
	Codec        string `json:"codec"`
	Title        string `json:"title,omitempty"`
	Channels     int    `json:"channels,omitempty"`
	Selected     bool   `json:"selected"`
	Default      bool   `json:"default"`
}

type EpisodeStreamResult struct {
	Episode     string `json:"episode"`
	Season      int    `json:"season"`
	EpisodeNum  int    `json:"episode_num"`
	AudioSet    string `json:"audio_set,omitempty"`
	SubtitleSet string `json:"subtitle_set,omitempty"`
	Status      string `json:"status"`
}

func (c *StreamsListCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	streams, err := cc.Client.GetStreams(cc.Ctx, c.RatingKey)
	if err != nil {
		return err
	}

	if len(streams) == 0 {
		fmt.Fprintln(u.Err(), "No streams found")
		return nil
	}

	return c.outputResults(u.Out(), streams)
}

func (c *StreamsListCmd) outputResults(w io.Writer, streams []plexclient.StreamInfo) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"ID", "TYPE", "LANGUAGE", "CODE", "CODEC", "TITLE", "SELECTED", "DEFAULT"}
	rows := make([][]string, len(streams))

	for i, s := range streams {
		streamType := ""
		switch s.StreamType {
		case 1:
			streamType = "video"
		case 2:
			streamType = "audio"
		case 3:
			streamType = "subtitle"
		}

		selected := ""
		if s.Selected {
			selected = "✓"
		}
		defaultStr := ""
		if s.Default {
			defaultStr = "✓"
		}

		title := s.Title
		if s.Channels > 0 {
			title = fmt.Sprintf("%s (%d ch)", s.Title, s.Channels)
		}

		rows[i] = []string{
			fmt.Sprintf("%d", s.ID),
			streamType,
			s.Language,
			s.LanguageCode,
			s.Codec,
			title,
			selected,
			defaultStr,
		}
	}

	return formatter.Format(w, header, rows, streams)
}

func (c *StreamsSetCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if c.Audio == "" && c.Subtitle == "" {
		return fmt.Errorf("at least one of --audio or --subtitle must be specified")
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	// Find the show
	showKey, err := c.findShow(cc.Ctx, cc.Client)
	if err != nil {
		return err
	}

	// Get episodes for the season
	episodes, err := cc.Client.GetSeasonEpisodes(cc.Ctx, showKey, c.Season)
	if err != nil {
		return err
	}

	if len(episodes) == 0 {
		return fmt.Errorf("no episodes found for season %d", c.Season)
	}

	var results []EpisodeStreamResult

	for _, ep := range episodes {
		result := EpisodeStreamResult{
			Episode:    ep.Title,
			Season:     ep.Season,
			EpisodeNum: ep.Episode,
		}

		// Get streams for this episode
		streams, err := cc.Client.GetStreams(cc.Ctx, ep.RatingKey)
		if err != nil {
			result.Status = fmt.Sprintf("Error: %v", err)
			results = append(results, result)
			continue
		}

		// Find matching audio stream
		var audioStreamID int
		if c.Audio != "" {
			audioStreamID = findStreamByLanguage(streams, 2, c.Audio)
			if audioStreamID > 0 {
				result.AudioSet = c.Audio
			} else {
				result.AudioSet = fmt.Sprintf("%s (not found)", c.Audio)
			}
		}

		// Find matching subtitle stream
		var subtitleStreamID int
		if c.Subtitle != "" {
			if strings.ToLower(c.Subtitle) == "off" || strings.ToLower(c.Subtitle) == "none" {
				subtitleStreamID = 0 // Special value to disable subtitles
				result.SubtitleSet = "off"
			} else {
				subtitleStreamID = findStreamByLanguage(streams, 3, c.Subtitle)
				if subtitleStreamID > 0 {
					result.SubtitleSet = c.Subtitle
				} else {
					result.SubtitleSet = fmt.Sprintf("%s (not found)", c.Subtitle)
				}
			}
		}

		if c.DryRun {
			result.Status = "Would update"
		} else {
			err = cc.Client.SetStreams(cc.Ctx, ep.PartID, audioStreamID, subtitleStreamID)
			if err != nil {
				result.Status = fmt.Sprintf("Error: %v", err)
			} else {
				result.Status = "Updated"
			}
		}

		results = append(results, result)
	}

	return c.outputResults(u.Out(), results)
}

func (c *StreamsSetCmd) findShow(ctx context.Context, client *plexclient.Client) (string, error) {
	// First try as rating key
	if _, err := client.GetItemMetadata(ctx, c.Show); err == nil {
		return c.Show, nil
	}

	// Search by name
	results, err := client.SearchLibrary(ctx, c.Show, nil, 50)
	if err != nil {
		return "", fmt.Errorf("failed to search for show: %w", err)
	}

	match, found := pickShowMatch(results, c.Show)
	if found {
		return match.RatingKey, nil
	}

	return "", fmt.Errorf("show not found: %s", c.Show)
}

func pickShowMatch(results []plexclient.SearchResult, query string) (plexclient.SearchResult, bool) {
	for _, result := range results {
		if result.Type == "show" && strings.EqualFold(result.Title, query) {
			return result, true
		}
	}

	queryLower := strings.ToLower(query)
	for _, result := range results {
		if result.Type == "show" && strings.Contains(strings.ToLower(result.Title), queryLower) {
			return result, true
		}
	}

	return plexclient.SearchResult{}, false
}

func findStreamByLanguage(streams []plexclient.StreamInfo, streamType int, langCode string) int {
	langCode = strings.ToLower(langCode)

	for _, s := range streams {
		if s.StreamType != streamType {
			continue
		}
		if strings.ToLower(s.LanguageCode) == langCode || strings.ToLower(s.Language) == langCode {
			return s.ID
		}
	}
	return 0
}

func (c *StreamsSetCmd) outputResults(w io.Writer, results []EpisodeStreamResult) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"EPISODE", "S", "E", "AUDIO", "SUBTITLE", "STATUS"}
	rows := make([][]string, len(results))

	for i, r := range results {
		rows[i] = []string{
			r.Episode,
			fmt.Sprintf("%d", r.Season),
			fmt.Sprintf("%d", r.EpisodeNum),
			r.AudioSet,
			r.SubtitleSet,
			r.Status,
		}
	}

	return formatter.Format(w, header, rows, results)
}
