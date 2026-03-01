package cmd

import (
	"context"
	"fmt"
	"io"
	"sort"
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
	Season   int    `help:"Season number (required unless --all-seasons is set; use 0 for specials)" short:"s" default:"-1"`
	All      bool   `help:"Apply to all seasons in the show" name:"all-seasons"`
	Audio    string `help:"Audio stream to select (language code or name, e.g., 'eng', 'jpn', 'japanese')" short:"a"`
	Subtitle string `help:"Subtitle stream to select (language/code/title, e.g., 'eng', 'full english', 'off')" short:"t"`
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
		if title == "" {
			title = s.DisplayTitle
		}
		if s.Channels > 0 {
			title = fmt.Sprintf("%s (%d ch)", title, s.Channels)
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
	if err := c.validateArgs(); err != nil {
		return err
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

	// Get episodes to update
	episodes, err := c.getTargetEpisodes(cc.Ctx, cc.Client, showKey)
	if err != nil {
		return err
	}

	if len(episodes) == 0 {
		if c.All {
			return fmt.Errorf("no episodes found for show")
		}
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
		var audioStreamID *int
		if c.Audio != "" {
			if streamID := findStreamByLanguage(streams, 2, c.Audio); streamID > 0 {
				audioStreamID = &streamID
				result.AudioSet = c.Audio
			} else {
				result.AudioSet = fmt.Sprintf("%s (not found)", c.Audio)
			}
		}

		// Find matching subtitle stream
		var subtitleStreamID *int
		if c.Subtitle != "" {
			if strings.ToLower(c.Subtitle) == "off" || strings.ToLower(c.Subtitle) == "none" {
				disableSubtitles := 0 // Special value to disable subtitles
				subtitleStreamID = &disableSubtitles
				result.SubtitleSet = "off"
			} else {
				if streamID := findStreamByLanguage(streams, 3, c.Subtitle); streamID > 0 {
					subtitleStreamID = &streamID
					result.SubtitleSet = c.Subtitle
				} else {
					result.SubtitleSet = fmt.Sprintf("%s (not found)", c.Subtitle)
				}
			}
		}
		if audioStreamID == nil && subtitleStreamID == nil {
			result.Status = "Skipped (no matching streams)"
			results = append(results, result)
			continue
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

func (c *StreamsSetCmd) validateArgs() error {
	if c.Audio == "" && c.Subtitle == "" {
		return fmt.Errorf("at least one of --audio or --subtitle must be specified")
	}
	if !c.All && c.Season < 0 {
		return fmt.Errorf("specify --season or --all-seasons")
	}
	if c.All && c.Season >= 0 {
		return fmt.Errorf("use either --season or --all-seasons, not both")
	}
	return nil
}

func (c *StreamsSetCmd) getTargetEpisodes(ctx context.Context, client *plexclient.Client, showKey string) ([]plexclient.EpisodeInfo, error) {
	if !c.All {
		return client.GetSeasonEpisodes(ctx, showKey, c.Season)
	}

	allEpisodes, err := client.GetShowEpisodes(ctx, showKey)
	if err != nil {
		return nil, err
	}

	seasonSet := make(map[int]struct{})
	for _, ep := range allEpisodes {
		seasonSet[ep.SeasonNum] = struct{}{}
	}

	seasons := make([]int, 0, len(seasonSet))
	for season := range seasonSet {
		seasons = append(seasons, season)
	}
	sort.Ints(seasons)

	results := make([]plexclient.EpisodeInfo, 0, len(allEpisodes))
	for _, season := range seasons {
		episodes, seasonErr := client.GetSeasonEpisodes(ctx, showKey, season)
		if seasonErr != nil {
			return nil, seasonErr
		}
		results = append(results, episodes...)
	}

	return results, nil
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
	query := normalizeStreamText(langCode)
	if query == "" {
		return 0
	}
	isFullEnglishSubtitle := streamType == 3 && strings.Contains(query, "full") && isEnglishQuery(query)
	isEnglishSubtitle := streamType == 3 && isEnglishQuery(query)

	// Prefer full English subtitles when explicitly requested.
	if isFullEnglishSubtitle {
		if id := preferFullEnglishSubtitle(streams, streamType, true); id > 0 {
			return id
		}
	}

	// For generic English subtitle requests, prefer full subtitles over signs/songs.
	if isEnglishSubtitle {
		if id := preferFullEnglishSubtitle(streams, streamType, false); id > 0 {
			return id
		}
	}

	// Exact match on language code, language, or stream title.
	for _, s := range streams {
		if s.StreamType != streamType {
			continue
		}
		if normalizeStreamText(s.LanguageCode) == query ||
			normalizeStreamText(s.Language) == query ||
			normalizeStreamText(s.Title) == query {
			return s.ID
		}
	}

	// Alias-aware match (e.g. jpn/japanese, eng/english).
	queryCanonical := canonicalLanguage(query)
	for _, s := range streams {
		if s.StreamType != streamType {
			continue
		}
		if canonicalLanguage(s.LanguageCode) == queryCanonical ||
			canonicalLanguage(s.Language) == queryCanonical {
			return s.ID
		}
	}

	// Fuzzy token match (useful for inputs like "full english").
	for _, s := range streams {
		if s.StreamType != streamType {
			continue
		}
		candidate := normalizeStreamText(strings.Join([]string{
			s.LanguageCode,
			s.Language,
			s.Title,
			s.DisplayTitle,
		}, " "))
		if containsAllTokens(candidate, query) {
			return s.ID
		}
	}
	return 0
}

func preferFullEnglishSubtitle(streams []plexclient.StreamInfo, streamType int, allowSDH bool) int {
	bestNonSDH := 0
	bestSDH := 0
	for _, s := range streams {
		if s.StreamType != streamType {
			continue
		}
		if !(isEnglishQuery(s.LanguageCode) || isEnglishQuery(s.Language)) {
			continue
		}

		title := normalizeStreamText(strings.Join([]string{s.Title, s.DisplayTitle}, " "))
		if !strings.Contains(title, "full") {
			continue
		}

		if strings.Contains(title, "sdh") {
			if bestSDH == 0 {
				bestSDH = s.ID
			}
			continue
		}
		if bestNonSDH == 0 {
			bestNonSDH = s.ID
		}
	}

	if bestNonSDH > 0 {
		return bestNonSDH
	}
	if allowSDH {
		return bestSDH
	}
	return 0
}

func normalizeStreamText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("-", " ", "_", " ", "/", " ", "(", " ", ")", " ", ",", " ")
	value = replacer.Replace(value)
	return strings.Join(strings.Fields(value), " ")
}

func containsAllTokens(haystack, query string) bool {
	if query == "" {
		return false
	}

	haystackTokens := make(map[string]struct{})
	for _, token := range strings.Fields(haystack) {
		haystackTokens[token] = struct{}{}
		haystackTokens[canonicalLanguage(token)] = struct{}{}
	}

	for _, token := range strings.Fields(query) {
		normalized := normalizeStreamText(token)
		if _, ok := haystackTokens[normalized]; ok {
			continue
		}
		if _, ok := haystackTokens[canonicalLanguage(normalized)]; ok {
			continue
		}
		if _, ok := haystackTokens[token]; ok {
			continue
		}
		if _, ok := haystackTokens[canonicalLanguage(token)]; ok {
			continue
		}

		return false
	}
	return true
}

func isEnglishQuery(value string) bool {
	switch canonicalLanguage(value) {
	case "english":
		return true
	default:
		return false
	}
}

func canonicalLanguage(value string) string {
	switch normalizeStreamText(value) {
	case "en", "eng", "english":
		return "english"
	case "ja", "jp", "jpn", "japanese":
		return "japanese"
	default:
		return normalizeStreamText(value)
	}
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
