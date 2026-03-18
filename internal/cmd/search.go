package cmd

import (
	"errors"
	"fmt"
	"io"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type SearchCmd struct {
	Query         string `arg:"" help:"Search query"`
	Section       string `help:"Filter by library section ID" default:""`
	Type          string `help:"Filter by type: movie, show, episode, artist, album, track, or all" default:"all" enum:"movie,show,episode,artist,album,track,all"`
	Limit         int    `help:"Maximum number of results" default:"50"`
	Exact         bool   `help:"Only return normalized exact title matches"`
	Year          int    `help:"Filter results by year after searching"`
	First         bool   `help:"Return only the first surviving result"`
	FailAmbiguous bool   `help:"Fail when more than one result remains after filtering"`
	Output        string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type SearchItem struct {
	RatingKey string `json:"rating_key"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Year      int    `json:"year,omitempty"`
	Show      string `json:"show,omitempty"`
	Season    int    `json:"season,omitempty"`
	Episode   int    `json:"episode,omitempty"`
}

func (c *SearchCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := validateSearchCommandOptions(c.First, c.FailAmbiguous); err != nil {
		return err
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	results, err := resolveSearchResults(cc.Ctx, cc.Client, c.Query, SearchResolveOptions{
		SectionID:     c.Section,
		Type:          c.Type,
		Limit:         c.Limit,
		Exact:         c.Exact,
		Year:          c.Year,
		First:         c.First,
		FailAmbiguous: c.FailAmbiguous,
	})
	if err != nil {
		if handledErr := handleSearchResolveError(u.Err(), err); handledErr == nil {
			return nil
		}
		return fmt.Errorf("search failed: %w", err)
	}

	if err := handleResolvedSearchResults(u.Err(), c.Output, c.Query, results, c.FailAmbiguous); err != nil {
		return err
	}

	outputItems := searchItemsFromResults(results)
	if err := outputSearchItems(u.Out(), c.Output, outputItems); err != nil {
		return err
	}
	return nil
}

func validateSearchCommandOptions(first bool, failAmbiguous bool) error {
	if first && failAmbiguous {
		return fmt.Errorf("--first and --fail-ambiguous cannot be used together")
	}
	return nil
}

func validateResolvedSearchResults(query string, results []plexclient.SearchResult, failAmbiguous bool) error {
	if failAmbiguous && len(results) > 1 {
		return fmt.Errorf("search returned %d results for %q", len(results), query)
	}
	return nil
}

func handleResolvedSearchResults(errOut io.Writer, output string, query string, results []plexclient.SearchResult, failAmbiguous bool) error {
	if err := validateResolvedSearchResults(query, results, failAmbiguous); err != nil {
		if failAmbiguous && len(results) > 1 {
			_ = outputSearchItems(errOut, output, searchItemsFromResults(results))
		}
		return err
	}
	return nil
}

func handleSearchResolveError(errOut io.Writer, err error) error {
	var noResultsErr *NoSearchResultsError
	if errors.As(err, &noResultsErr) {
		_, _ = fmt.Fprintln(errOut, "No results found")
		return nil
	}
	return err
}
