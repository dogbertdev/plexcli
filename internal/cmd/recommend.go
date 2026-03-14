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

type RecommendCmd struct {
	Like         []string `help:"Seed title or rating key" name:"like"`
	Section      string   `help:"Restrict resolution to a library section ID"`
	Type         string   `help:"Seed and result type" default:"movie" enum:"movie,show,artist"`
	Limit        int      `help:"Maximum number of ranked results" default:"25"`
	IncludeSeeds bool     `help:"Include seed items in the ranked results"`
	PlaylistName string   `help:"Create a playlist with the ranked results"`
	Output       string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type RecommendationItem struct {
	RatingKey string `json:"rating_key"`
	Title     string `json:"title"`
	Year      int    `json:"year,omitempty"`
	Score     int    `json:"score"`
	SeedHits  int    `json:"seed_hits"`
	Reason    string `json:"reason"`
}

type recommendationSeed struct {
	Input string
	Item  plexclient.SearchResult
}

type recommendationCandidate struct {
	Item           plexclient.SearchResult
	SeedHits       map[string]struct{}
	GenreOverlap   int
	CountryOverlap int
	YearNearSeed   bool
}

func (c *RecommendCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if len(c.Like) < 2 {
		return fmt.Errorf("at least two --like values are required")
	}
	if c.Limit <= 0 {
		return fmt.Errorf("--limit must be greater than zero")
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	seeds, err := c.resolveSeeds(u, cc.Client, cc.Ctx)
	if err != nil {
		return err
	}

	results, err := c.recommend(cc.Client, cc.Ctx, seeds)
	if err != nil {
		return err
	}

	if c.PlaylistName != "" {
		keys := make([]string, 0, len(results))
		for _, item := range results {
			keys = append(keys, item.RatingKey)
		}
		if len(keys) > 0 {
			playlistType := "video"
			if c.Type == "artist" {
				playlistType = "audio"
			}
			if _, err := cc.Client.CreatePlaylist(cc.Ctx, c.PlaylistName, playlistType, keys); err != nil {
				return fmt.Errorf("failed to create recommendation playlist: %w", err)
			}
		}
	}

	return c.output(u.Out(), results)
}

func (c *RecommendCmd) resolveSeeds(u *ui.UI, client *plexclient.Client, ctx context.Context) ([]recommendationSeed, error) {
	seeds := make([]recommendationSeed, 0, len(c.Like))
	seen := make(map[string]struct{}, len(c.Like))

	for _, like := range c.Like {
		result, err := resolveSingleSearchResult(ctx, client, like, SearchResolveOptions{
			SectionID:      c.Section,
			Type:           c.Type,
			Limit:          plexclient.DefaultSearchLimit,
			AllowRatingKey: true,
		})
		if err != nil {
			if ambErr, ok := err.(*AmbiguousSearchError); ok {
				_ = outputSearchItems(u.Err(), c.Output, searchItemsFromResults(ambErr.Candidates))
			}
			return nil, fmt.Errorf("failed to resolve seed %q: %w", like, err)
		}

		detailed, err := client.GetMetadataSearchResult(ctx, result.RatingKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load metadata for seed %q: %w", like, err)
		}
		result = mergeSearchResult(result, detailed)

		dedupeKey := recommendationDedupeKey(result)
		if _, ok := seen[dedupeKey]; ok {
			continue
		}
		seen[dedupeKey] = struct{}{}

		seeds = append(seeds, recommendationSeed{Input: like, Item: result})
	}

	if len(seeds) < 2 {
		return nil, fmt.Errorf("at least two distinct seeds are required")
	}

	return seeds, nil
}

func (c *RecommendCmd) recommend(client *plexclient.Client, ctx context.Context, seeds []recommendationSeed) ([]RecommendationItem, error) {
	limit := c.Limit
	similarCount := limit * 4
	if similarCount < 50 {
		similarCount = 50
	}

	candidates := make(map[string]*recommendationCandidate)
	seedRatingKeys := make(map[string]struct{}, len(seeds))
	seedKeys := make(map[string]struct{}, len(seeds))
	seedGenres := make(map[string]struct{})
	seedCountries := make(map[string]struct{})
	seedYears := make([]int, 0, len(seeds))

	for _, seed := range seeds {
		seedRatingKeys[seed.Item.RatingKey] = struct{}{}
		seedKeys[recommendationDedupeKey(seed.Item)] = struct{}{}
		for genre := range normalizeStringSet(seed.Item.Genres) {
			seedGenres[genre] = struct{}{}
		}
		for country := range normalizeStringSet(seed.Item.Countries) {
			seedCountries[country] = struct{}{}
		}
		if seed.Item.Year != nil && *seed.Item.Year > 0 {
			seedYears = append(seedYears, *seed.Item.Year)
		}

		if c.IncludeSeeds {
			addRecommendationCandidate(candidates, seed.Item, seed.Item.RatingKey)
		}

		similarItems, err := client.ListSimilar(ctx, seed.Item.RatingKey, similarCount)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch similar items for %q: %w", seed.Item.Title, err)
		}

		for _, similar := range similarItems {
			if !searchResultMatchesOptions(similar, "", SearchResolveOptions{
				SectionID: c.Section,
				Type:      c.Type,
			}) {
				continue
			}
			addRecommendationCandidate(candidates, similar, seed.Item.RatingKey)
		}
	}

	return rankRecommendationCandidates(
		candidates,
		seedRatingKeys,
		seedKeys,
		seedGenres,
		seedCountries,
		seedYears,
		c.IncludeSeeds,
		c.Limit,
	), nil
}

func rankRecommendationCandidates(
	candidates map[string]*recommendationCandidate,
	seedRatingKeys map[string]struct{},
	seedKeys map[string]struct{},
	seedGenres map[string]struct{},
	seedCountries map[string]struct{},
	seedYears []int,
	includeSeeds bool,
	limit int,
) []RecommendationItem {
	recommendations := make([]RecommendationItem, 0, len(candidates))
	for key, candidate := range candidates {
		if !includeSeeds {
			if _, ok := seedRatingKeys[candidate.Item.RatingKey]; ok {
				continue
			}
			if _, ok := seedKeys[key]; ok {
				continue
			}
		}

		candidate.GenreOverlap = overlapCount(seedGenres, normalizeStringSet(candidate.Item.Genres))
		candidate.CountryOverlap = overlapCount(seedCountries, normalizeStringSet(candidate.Item.Countries))
		candidate.YearNearSeed = isNearSeedYear(candidate.Item.Year, seedYears)

		score := scoreRecommendation(candidate)
		recommendations = append(recommendations, RecommendationItem{
			RatingKey: candidate.Item.RatingKey,
			Title:     candidate.Item.Title,
			Year:      yearValue(candidate.Item.Year),
			Score:     score,
			SeedHits:  len(candidate.SeedHits),
			Reason:    recommendationReason(candidate),
		})
	}

	sort.SliceStable(recommendations, func(i, j int) bool {
		return recommendationItemLess(recommendations[i], recommendations[j])
	})

	return limitRecommendationItems(recommendations, limit)
}

func addRecommendationCandidate(
	candidates map[string]*recommendationCandidate,
	item plexclient.SearchResult,
	seedRatingKey string,
) {
	key := recommendationDedupeKey(item)
	if key == "" {
		return
	}

	candidate := candidates[key]
	if candidate == nil {
		candidate = &recommendationCandidate{
			Item:     item,
			SeedHits: map[string]struct{}{},
		}
		candidates[key] = candidate
	} else {
		candidate.Item = mergeSearchResult(candidate.Item, item)
	}

	if seedRatingKey != "" {
		candidate.SeedHits[seedRatingKey] = struct{}{}
	}
}

func limitRecommendationItems(recommendations []RecommendationItem, limit int) []RecommendationItem {
	if limit <= 0 || len(recommendations) <= limit {
		return recommendations
	}
	return recommendations[:limit]
}

func recommendationItemLess(left RecommendationItem, right RecommendationItem) bool {
	if left.Score != right.Score {
		return left.Score > right.Score
	}
	if left.SeedHits != right.SeedHits {
		return left.SeedHits > right.SeedHits
	}
	if normalizeSearchTitle(left.Title) != normalizeSearchTitle(right.Title) {
		return normalizeSearchTitle(left.Title) < normalizeSearchTitle(right.Title)
	}
	if left.Year != right.Year {
		return left.Year < right.Year
	}
	return left.RatingKey < right.RatingKey
}

func (c *RecommendCmd) output(w io.Writer, items []RecommendationItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))
	header := []string{"RATING KEY", "TITLE", "YEAR", "SCORE", "SEED HITS", "REASON"}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{
			item.RatingKey,
			item.Title,
			intString(item.Year),
			fmt.Sprintf("%d", item.Score),
			fmt.Sprintf("%d", item.SeedHits),
			item.Reason,
		})
	}
	return formatter.Format(w, header, rows, items)
}

func recommendationDedupeKey(item plexclient.SearchResult) string {
	return plexclient.SearchResultLogicalKey(item)
}

func scoreRecommendation(candidate *recommendationCandidate) int {
	score := len(candidate.SeedHits) * 100
	score += candidate.GenreOverlap * 10
	score += candidate.CountryOverlap * 5
	if candidate.YearNearSeed {
		score += 3
	}
	return score
}

func recommendationReason(candidate *recommendationCandidate) string {
	reasons := []string{fmt.Sprintf("similar-to-%d-seeds", len(candidate.SeedHits))}
	if candidate.GenreOverlap > 0 {
		reasons = append(reasons, "genre-overlap")
	}
	if candidate.CountryOverlap > 0 {
		reasons = append(reasons, "country-overlap")
	}
	if candidate.YearNearSeed {
		reasons = append(reasons, "year-near-seed")
	}
	return strings.Join(reasons, ", ")
}

func normalizeStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := normalizeSearchTitle(value)
		if normalized == "" {
			continue
		}
		set[normalized] = struct{}{}
	}
	return set
}

func overlapCount(seedSet map[string]struct{}, values map[string]struct{}) int {
	count := 0
	for value := range values {
		if _, ok := seedSet[value]; ok {
			count++
		}
	}
	return count
}

func isNearSeedYear(year *int, seedYears []int) bool {
	if year == nil || *year == 0 {
		return false
	}
	for _, seedYear := range seedYears {
		if seedYear-*year <= 5 && *year-seedYear <= 5 {
			return true
		}
	}
	return false
}

func yearValue(year *int) int {
	if year == nil {
		return 0
	}
	return *year
}

func mergeSearchResult(base plexclient.SearchResult, overlay plexclient.SearchResult) plexclient.SearchResult {
	if base.RatingKey == "" {
		base.RatingKey = overlay.RatingKey
	}
	if base.Key == "" {
		base.Key = overlay.Key
	}
	if base.Title == "" {
		base.Title = overlay.Title
	}
	if base.Type == "" {
		base.Type = overlay.Type
	}
	if base.Year == nil {
		base.Year = overlay.Year
	}
	if base.GUID == nil {
		base.GUID = overlay.GUID
	}
	if base.LibrarySectionID == nil {
		base.LibrarySectionID = overlay.LibrarySectionID
	}
	if base.LibrarySectionTitle == nil {
		base.LibrarySectionTitle = overlay.LibrarySectionTitle
	}
	if base.Studio == nil {
		base.Studio = overlay.Studio
	}
	if len(base.Genres) == 0 {
		base.Genres = append([]string{}, overlay.Genres...)
	}
	if len(base.Countries) == 0 {
		base.Countries = append([]string{}, overlay.Countries...)
	}
	if len(base.Collections) == 0 {
		base.Collections = append([]string{}, overlay.Collections...)
	}
	return base
}
