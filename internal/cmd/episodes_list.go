package cmd

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/user/plexcli/internal/auth"
	"github.com/user/plexcli/internal/config"
	"github.com/user/plexcli/internal/outfmt"
	"github.com/user/plexcli/internal/plexclient"
	"github.com/user/plexcli/internal/ui"
)

type EpisodesListCmd struct {
	Show     string `arg:"" help:"Show name to search for"`
	Filter   string `help:"Filter episodes by S##E## patterns (comma-separated, e.g., 'S1E1,S1E3,S2E5')" default:""`
	KeysOnly bool   `help:"Output only rating keys (space-separated, for piping to playlist create)" default:"false"`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type EpisodeListItem struct {
	RatingKey string `json:"rating_key"`
	Show      string `json:"show"`
	Season    int    `json:"season"`
	Episode   int    `json:"episode"`
	Title     string `json:"title"`
	SeasonEp  string `json:"season_episode"`
}

func (c *EpisodesListCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
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

	// Find the show
	showRatingKey, showTitle, err := client.FindShow(fetchCtx, c.Show)
	if err != nil {
		return fmt.Errorf("failed to find show: %w", err)
	}

	// Get all episodes
	episodes, err := client.GetShowEpisodes(fetchCtx, showRatingKey)
	if err != nil {
		return fmt.Errorf("failed to get episodes: %w", err)
	}

	// Parse filter if provided
	var filterSet map[string]bool
	if c.Filter != "" {
		filterSet = parseEpisodeFilterPatterns(c.Filter)
	}

	// Convert to output items and apply filter
	var outputItems []EpisodeListItem
	for _, ep := range episodes {
		seasonEp := fmt.Sprintf("S%dE%d", ep.SeasonNum, ep.EpisodeNum)

		// Apply filter if set
		if filterSet != nil {
			// Check both with and without leading zeros
			key1 := fmt.Sprintf("S%dE%d", ep.SeasonNum, ep.EpisodeNum)
			key2 := fmt.Sprintf("S%02dE%02d", ep.SeasonNum, ep.EpisodeNum)
			if !filterSet[key1] && !filterSet[key2] {
				continue
			}
		}

		outputItems = append(outputItems, EpisodeListItem{
			RatingKey: ep.RatingKey,
			Show:      showTitle,
			Season:    ep.SeasonNum,
			Episode:   ep.EpisodeNum,
			Title:     ep.Title,
			SeasonEp:  seasonEp,
		})
	}

	// Sort by season, then episode
	sort.Slice(outputItems, func(i, j int) bool {
		if outputItems[i].Season != outputItems[j].Season {
			return outputItems[i].Season < outputItems[j].Season
		}
		return outputItems[i].Episode < outputItems[j].Episode
	})

	if len(outputItems) == 0 {
		if filterSet != nil {
			fmt.Fprintln(u.Err(), "No matching episodes found")
		} else {
			fmt.Fprintln(u.Err(), "No episodes found")
		}
		return nil
	}

	// Keys-only output for piping
	if c.KeysOnly {
		keys := make([]string, len(outputItems))
		for i, item := range outputItems {
			keys[i] = item.RatingKey
		}
		fmt.Fprintln(u.Out(), strings.Join(keys, " "))
		return nil
	}

	return c.output(u.Out(), outputItems)
}

// parseEpisodeFilterPatterns parses a comma-separated list of S##E## patterns
func parseEpisodeFilterPatterns(filter string) map[string]bool {
	result := make(map[string]bool)

	// Regex to match S##E## patterns (with optional leading zeros)
	re := regexp.MustCompile(`(?i)S(\d+)E(\d+)`)

	parts := strings.Split(filter, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		matches := re.FindStringSubmatch(part)
		if len(matches) == 3 {
			season, _ := strconv.Atoi(matches[1])
			episode, _ := strconv.Atoi(matches[2])
			// Store both formats for matching
			result[fmt.Sprintf("S%dE%d", season, episode)] = true
			result[fmt.Sprintf("S%02dE%02d", season, episode)] = true
		}
	}

	return result
}

func (c *EpisodesListCmd) output(w io.Writer, items []EpisodeListItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"RATING KEY", "SHOW", "S", "E", "TITLE"}
	rows := make([][]string, 0, len(items))

	for _, item := range items {
		rows = append(rows, []string{
			item.RatingKey,
			item.Show,
			fmt.Sprintf("%d", item.Season),
			fmt.Sprintf("%d", item.Episode),
			item.Title,
		})
	}

	return formatter.Format(w, header, rows, items)
}
