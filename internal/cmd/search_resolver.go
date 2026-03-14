package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"

	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
)

type SearchResolveOptions struct {
	SectionID      string
	Type           string
	AllowedTypes   []string
	Limit          int
	Exact          bool
	Year           int
	First          bool
	FailAmbiguous  bool
	AllowRatingKey bool
}

type AmbiguousSearchError struct {
	Query      string
	Candidates []plexclient.SearchResult
}

func (e *AmbiguousSearchError) Error() string {
	return fmt.Sprintf("query %q is ambiguous", e.Query)
}

type NoSearchResultsError struct {
	Query string
}

func (e *NoSearchResultsError) Error() string {
	return fmt.Sprintf("no matches found for %q", e.Query)
}

func resolveSearchResults(ctx context.Context, client *plexclient.Client, query string, opts SearchResolveOptions) ([]plexclient.SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, &NoSearchResultsError{Query: query}
	}

	if opts.AllowRatingKey && looksLikeRatingKey(query) {
		if result, err := client.GetMetadataSearchResult(ctx, query); err == nil {
			results := filterSearchResults([]plexclient.SearchResult{result}, query, opts)
			if len(results) > 0 {
				return results, nil
			}
		} else if !errors.Is(err, plexclient.ErrMetadataNotFound) {
			return nil, err
		}
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = plexclient.DefaultSearchLimit
	}
	searchLimit := limit
	if opts.FailAmbiguous && !opts.First && searchLimit < plexclient.DefaultSearchLimit {
		searchLimit = plexclient.DefaultSearchLimit
	}

	var sectionID *string
	if strings.TrimSpace(opts.SectionID) != "" {
		sectionID = &opts.SectionID
	}

	results, err := client.SearchLibrary(ctx, query, sectionID, searchLimit)
	if err != nil {
		return nil, err
	}

	results = filterSearchResults(results, query, opts)
	if len(results) == 0 {
		return nil, &NoSearchResultsError{Query: query}
	}

	if opts.First && len(results) > 1 {
		results = results[:1]
	}
	if !opts.First && !opts.FailAmbiguous && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func resolveSingleSearchResult(ctx context.Context, client *plexclient.Client, query string, opts SearchResolveOptions) (plexclient.SearchResult, error) {
	results, err := resolveSearchResults(ctx, client, query, opts)
	if err != nil {
		return plexclient.SearchResult{}, err
	}
	if len(results) > 1 {
		return plexclient.SearchResult{}, &AmbiguousSearchError{
			Query:      query,
			Candidates: results,
		}
	}
	return results[0], nil
}

func filterSearchResults(results []plexclient.SearchResult, query string, opts SearchResolveOptions) []plexclient.SearchResult {
	filtered := make([]plexclient.SearchResult, 0, len(results))
	normalizedQuery := normalizeSearchTitle(query)

	for _, result := range results {
		if searchResultMatchesOptions(result, normalizedQuery, opts) {
			filtered = append(filtered, result)
		}
	}

	return filtered
}

func searchResultMatchesOptions(result plexclient.SearchResult, normalizedQuery string, opts SearchResolveOptions) bool {
	if !searchResultMatchesAllowedTypes(result, opts) {
		return false
	}
	if strings.TrimSpace(opts.SectionID) != "" {
		if result.LibrarySectionID == nil || strconv.Itoa(*result.LibrarySectionID) != strings.TrimSpace(opts.SectionID) {
			return false
		}
	}
	if opts.Exact && normalizeSearchTitle(result.Title) != normalizedQuery {
		return false
	}
	if opts.Year > 0 {
		if result.Year == nil || *result.Year != opts.Year {
			return false
		}
	}
	return true
}

func searchResultMatchesAllowedTypes(result plexclient.SearchResult, opts SearchResolveOptions) bool {
	if opts.Type != "" && opts.Type != "all" && result.Type != opts.Type {
		return false
	}
	if len(opts.AllowedTypes) == 0 {
		return true
	}
	for _, allowedType := range opts.AllowedTypes {
		if result.Type == allowedType {
			return true
		}
	}
	return false
}

func normalizeSearchTitle(input string) string {
	var b strings.Builder
	spacePending := false

	for _, r := range strings.ToLower(strings.TrimSpace(input)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if spacePending && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteRune(r)
			spacePending = false
		case unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r):
			spacePending = true
		default:
			spacePending = true
		}
	}

	return strings.TrimSpace(b.String())
}

func looksLikeRatingKey(input string) bool {
	if input == "" {
		return false
	}
	for _, r := range input {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func searchItemsFromResults(results []plexclient.SearchResult) []SearchItem {
	items := make([]SearchItem, 0, len(results))
	for _, r := range results {
		item := SearchItem{
			RatingKey: r.RatingKey,
			Title:     r.Title,
			Type:      r.Type,
		}
		if r.Year != nil {
			item.Year = *r.Year
		}
		if r.GrandparentTitle != nil {
			item.Show = *r.GrandparentTitle
		}
		if r.ParentIndex != nil {
			item.Season = *r.ParentIndex
		}
		if r.Index != nil {
			item.Episode = *r.Index
		}
		items = append(items, item)
	}
	return items
}

func outputSearchItems(w io.Writer, format string, items []SearchItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(format))

	header := []string{"RATING KEY", "TITLE", "TYPE", "YEAR", "SHOW", "S", "E"}
	rows := make([][]string, 0, len(items))

	for _, item := range items {
		yearStr := ""
		if item.Year > 0 {
			yearStr = fmt.Sprintf("%d", item.Year)
		}
		seasonStr := ""
		if item.Season > 0 {
			seasonStr = fmt.Sprintf("%d", item.Season)
		}
		episodeStr := ""
		if item.Episode > 0 {
			episodeStr = fmt.Sprintf("%d", item.Episode)
		}
		rows = append(rows, []string{
			item.RatingKey,
			item.Title,
			item.Type,
			yearStr,
			item.Show,
			seasonStr,
			episodeStr,
		})
	}

	return formatter.Format(w, header, rows, items)
}

func dedupeRatingKeys(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	deduped := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		deduped = append(deduped, item)
	}
	return deduped
}
