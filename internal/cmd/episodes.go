package cmd

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type EpisodesMissingCmd struct {
	Show   string `help:"Filter by specific show name" default:""`
	Season int    `help:"Filter by specific season number (0 = all seasons)" default:"0"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type EpisodeGap struct {
	Show            string `json:"show"`
	Season          int    `json:"season"`
	MissingEpisodes []int  `json:"missing_episodes"`
	TotalEpisodes   int    `json:"total_episodes"`
}

func (c *EpisodesMissingCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	episodes, err := c.fetchEpisodes(cc.Ctx, cc.Client)
	if err != nil {
		return err
	}

	if len(episodes) == 0 {
		fmt.Fprintln(u.Err(), "No episodes found")
		return nil
	}

	gaps := c.findMissingEpisodes(episodes)

	if len(gaps) == 0 {
		fmt.Fprintln(u.Err(), "No missing episodes found")
		return nil
	}

	return c.outputResults(u.Out(), gaps)
}

func (c *EpisodesMissingCmd) fetchEpisodes(ctx context.Context, client *plexclient.Client) ([]*components.Metadata, error) {
	sections, err := client.GetSections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sections: %w", err)
	}

	var allEpisodes []*components.Metadata
	for _, section := range sections {
		if section.Type == "show" {
			episodes, err := client.GetAllLibraryItems(ctx, section.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to get episodes from section %s: %w", section.ID, err)
			}
			allEpisodes = append(allEpisodes, episodes...)
		}
	}

	return allEpisodes, nil
}

func (c *EpisodesMissingCmd) findMissingEpisodes(episodes []*components.Metadata) []EpisodeGap {
	type showSeasonKey struct {
		show   string
		season int
	}

	showSeasonEpisodes := make(map[showSeasonKey][]int)

	for _, ep := range episodes {
		if anyToString(ep.Type) != "episode" {
			continue
		}

		showTitle := episodesGetShowTitle(ep)
		if c.Show != "" && showTitle != c.Show {
			continue
		}

		season := 0
		if ep.ParentIndex != nil {
			season = int(*ep.ParentIndex)
		}

		if c.Season > 0 && season != c.Season {
			continue
		}

		episodeNum := 0
		if ep.Index != nil {
			episodeNum = int(*ep.Index)
		}

		if episodeNum > 0 {
			key := showSeasonKey{show: showTitle, season: season}
			showSeasonEpisodes[key] = append(showSeasonEpisodes[key], episodeNum)
		}
	}

	var gaps []EpisodeGap

	for key, episodeNums := range showSeasonEpisodes {
		if len(episodeNums) < 2 {
			continue
		}

		sort.Ints(episodeNums)
		missing := c.findGaps(episodeNums)

		if len(missing) > 0 {
			gaps = append(gaps, EpisodeGap{
				Show:            key.show,
				Season:          key.season,
				MissingEpisodes: missing,
				TotalEpisodes:   len(episodeNums) + len(missing),
			})
		}
	}

	return gaps
}

func (c *EpisodesMissingCmd) findGaps(episodes []int) []int {
	if len(episodes) < 2 {
		return nil
	}

	var missing []int
	for i := 0; i < len(episodes)-1; i++ {
		current := episodes[i]
		next := episodes[i+1]

		for j := current + 1; j < next; j++ {
			missing = append(missing, j)
		}
	}

	return missing
}

func (c *EpisodesMissingCmd) outputResults(w io.Writer, gaps []EpisodeGap) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"SHOW", "SEASON", "MISSING EPISODES", "TOTAL EPISODES"}
	rows := make([][]string, len(gaps))

	for i, g := range gaps {
		missingStr := formatEpisodeList(g.MissingEpisodes)
		rows[i] = []string{
			g.Show,
			fmt.Sprintf("S%02d", g.Season),
			missingStr,
			strconv.Itoa(g.TotalEpisodes),
		}
	}

	return formatter.Format(w, header, rows, gaps)
}

func episodesGetShowTitle(ep *components.Metadata) string {
	if ep.GrandparentTitle != nil && *ep.GrandparentTitle != "" {
		return *ep.GrandparentTitle
	}
	if ep.ParentTitle != nil && *ep.ParentTitle != "" {
		return *ep.ParentTitle
	}
	return anyToString(ep.Title)
}

func formatEpisodeList(episodes []int) string {
	if len(episodes) == 0 {
		return ""
	}

	var parts []string
	start := episodes[0]
	end := episodes[0]

	for i := 1; i < len(episodes); i++ {
		if episodes[i] == end+1 {
			end = episodes[i]
		} else {
			if start == end {
				parts = append(parts, fmt.Sprintf("E%02d", start))
			} else {
				parts = append(parts, fmt.Sprintf("E%02d-E%02d", start, end))
			}
			start = episodes[i]
			end = episodes[i]
		}
	}

	if start == end {
		parts = append(parts, fmt.Sprintf("E%02d", start))
	} else {
		parts = append(parts, fmt.Sprintf("E%02d-E%02d", start, end))
	}

	return strings.Join(parts, ", ")
}
