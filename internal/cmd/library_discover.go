package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type LibraryDiscoverCmd struct {
	Similar LibraryDiscoverSimilarCmd `cmd:"" help:"List similar items for a metadata item"`
	Related LibraryDiscoverRelatedCmd `cmd:"" help:"List related items for a metadata item"`
	Matches LibraryDiscoverMatchesCmd `cmd:"" help:"List Plex metadata matches for a metadata item"`
}

type LibraryDiscoverSimilarCmd struct {
	RatingKey string `arg:"" help:"Metadata rating key"`
	Limit     int    `help:"Maximum number of results" default:"50"`
	Compact   bool   `help:"Return a flat compact record set"`
	Output    string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryDiscoverRelatedCmd struct {
	RatingKey string `arg:"" help:"Metadata rating key"`
	Compact   bool   `help:"Return a flat compact record set"`
	Output    string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryDiscoverMatchesCmd struct {
	RatingKey string `arg:"" help:"Metadata rating key"`
	Compact   bool   `help:"Return a flat compact record set"`
	Output    string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type DiscoveryItem struct {
	RatingKey    string   `json:"rating_key"`
	Title        string   `json:"title"`
	Type         string   `json:"type"`
	Year         int      `json:"year,omitempty"`
	SectionID    string   `json:"section_id,omitempty"`
	SectionTitle string   `json:"section_title,omitempty"`
	GUID         string   `json:"guid,omitempty"`
	Genres       []string `json:"genres,omitempty"`
	Countries    []string `json:"countries,omitempty"`
	Collections  []string `json:"collections,omitempty"`
	Studio       string   `json:"studio,omitempty"`
}

type DiscoveryCompactItem struct {
	RatingKey    string `json:"rating_key"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	Year         int    `json:"year,omitempty"`
	SectionID    string `json:"section_id,omitempty"`
	SectionTitle string `json:"section_title,omitempty"`
	GUID         string `json:"guid,omitempty"`
}

type DiscoveryMatchItem struct {
	GUID    string `json:"guid"`
	Name    string `json:"name"`
	Year    int    `json:"year,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type DiscoveryMatchCompactItem struct {
	GUID string `json:"guid"`
	Name string `json:"name"`
	Year int    `json:"year,omitempty"`
}

var newLibraryDiscoverClientContext = NewClientContext

func (c *LibraryDiscoverSimilarCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := newLibraryDiscoverClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	results, err := cc.Client.ListSimilar(cc.Ctx, c.RatingKey, c.Limit)
	if err != nil {
		return fmt.Errorf("failed to list similar items: %w", err)
	}
	return outputDiscoveryResults(u.Out(), c.Output, c.Compact, results)
}

func (c *LibraryDiscoverRelatedCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := newLibraryDiscoverClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	results, err := cc.Client.GetRelatedItems(cc.Ctx, c.RatingKey)
	if err != nil {
		return fmt.Errorf("failed to list related items: %w", err)
	}
	return outputDiscoveryResults(u.Out(), c.Output, c.Compact, results)
}

func (c *LibraryDiscoverMatchesCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := newLibraryDiscoverClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	results, err := cc.Client.SearchMatches(cc.Ctx, c.RatingKey, "", 0)
	if err != nil {
		return fmt.Errorf("failed to list matches: %w", err)
	}
	return outputDiscoveryMatches(u.Out(), c.Output, c.Compact, results)
}

func outputDiscoveryResults(w io.Writer, format string, compact bool, results []plexclient.SearchResult) error {
	if compact {
		items := compactDiscoveryItems(results)
		return outputCompactDiscoveryItems(w, format, items)
	}

	items := discoveryItems(results)
	formatter := outfmt.NewFormatter(outfmt.Format(format))
	header := []string{"RATING KEY", "TITLE", "TYPE", "YEAR", "SECTION", "GUID", "GENRES", "COUNTRIES", "COLLECTIONS", "STUDIO"}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		section := item.SectionTitle
		if section == "" {
			section = item.SectionID
		}
		rows = append(rows, []string{
			item.RatingKey,
			item.Title,
			item.Type,
			intString(item.Year),
			section,
			item.GUID,
			strings.Join(item.Genres, ", "),
			strings.Join(item.Countries, ", "),
			strings.Join(item.Collections, ", "),
			item.Studio,
		})
	}
	return formatter.Format(w, header, rows, items)
}

func outputCompactDiscoveryItems(w io.Writer, format string, items []DiscoveryCompactItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(format))
	header := []string{"RATING KEY", "TITLE", "TYPE", "YEAR", "SECTION ID", "SECTION TITLE", "GUID"}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.RatingKey,
			item.Title,
			item.Type,
			intString(item.Year),
			item.SectionID,
			item.SectionTitle,
			item.GUID,
		})
	}
	return formatter.Format(w, header, rows, items)
}

func outputDiscoveryMatches(w io.Writer, format string, compact bool, results []plexclient.MatchResult) error {
	formatter := outfmt.NewFormatter(outfmt.Format(format))

	if compact {
		items := compactDiscoveryMatches(results)
		header := []string{"GUID", "NAME", "YEAR"}
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			rows = append(rows, []string{
				item.GUID,
				item.Name,
				intString(item.Year),
			})
		}
		return formatter.Format(w, header, rows, items)
	}

	items := discoveryMatches(results)
	header := []string{"GUID", "NAME", "YEAR", "SUMMARY"}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.GUID,
			item.Name,
			intString(item.Year),
			item.Summary,
		})
	}
	return formatter.Format(w, header, rows, items)
}

func discoveryItems(results []plexclient.SearchResult) []DiscoveryItem {
	items := make([]DiscoveryItem, 0, len(results))
	for _, result := range results {
		item := DiscoveryItem{
			RatingKey:   result.RatingKey,
			Title:       result.Title,
			Type:        result.Type,
			Genres:      append([]string{}, result.Genres...),
			Countries:   append([]string{}, result.Countries...),
			Collections: append([]string{}, result.Collections...),
		}
		if result.Year != nil {
			item.Year = *result.Year
		}
		if result.LibrarySectionID != nil {
			item.SectionID = fmt.Sprintf("%d", *result.LibrarySectionID)
		}
		if result.LibrarySectionTitle != nil {
			item.SectionTitle = *result.LibrarySectionTitle
		}
		if result.GUID != nil {
			item.GUID = *result.GUID
		}
		if result.Studio != nil {
			item.Studio = *result.Studio
		}
		items = append(items, item)
	}
	return items
}

func compactDiscoveryItems(results []plexclient.SearchResult) []DiscoveryCompactItem {
	items := make([]DiscoveryCompactItem, 0, len(results))
	for _, result := range results {
		item := DiscoveryCompactItem{
			RatingKey: result.RatingKey,
			Title:     result.Title,
			Type:      result.Type,
		}
		if result.Year != nil {
			item.Year = *result.Year
		}
		if result.LibrarySectionID != nil {
			item.SectionID = fmt.Sprintf("%d", *result.LibrarySectionID)
		}
		if result.LibrarySectionTitle != nil {
			item.SectionTitle = *result.LibrarySectionTitle
		}
		if result.GUID != nil {
			item.GUID = *result.GUID
		}
		items = append(items, item)
	}
	return items
}

func discoveryMatches(results []plexclient.MatchResult) []DiscoveryMatchItem {
	items := make([]DiscoveryMatchItem, 0, len(results))
	for _, result := range results {
		items = append(items, DiscoveryMatchItem{
			GUID:    result.GUID,
			Name:    result.Name,
			Year:    result.Year,
			Summary: result.Summary,
		})
	}
	return items
}

func compactDiscoveryMatches(results []plexclient.MatchResult) []DiscoveryMatchCompactItem {
	items := make([]DiscoveryMatchCompactItem, 0, len(results))
	for _, result := range results {
		items = append(items, DiscoveryMatchCompactItem{
			GUID: result.GUID,
			Name: result.Name,
			Year: result.Year,
		})
	}
	return items
}

func intString(value int) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
}
