package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

func TestRecommendationDedupeKey_PrefersGUID(t *testing.T) {
	guid := "tmdb://123"
	key := recommendationDedupeKey(plexclient.SearchResult{
		RatingKey: "42",
		Title:     "Avalon",
		Type:      "movie",
		GUID:      &guid,
	})

	if key != "guid:tmdb://123" {
		t.Fatalf("unexpected dedupe key: %s", key)
	}
}

func TestRecommendationDedupeKey_DistinguishesEpisodesBySeriesContext(t *testing.T) {
	showOne := "Show One"
	showTwo := "Show Two"
	seasonOne := "Season 1"
	parentIndex := 1
	episodeIndex := 1

	first := recommendationDedupeKey(plexclient.SearchResult{
		Title:            "Pilot",
		Type:             "episode",
		GrandparentTitle: &showOne,
		ParentTitle:      &seasonOne,
		ParentIndex:      &parentIndex,
		Index:            &episodeIndex,
	})
	second := recommendationDedupeKey(plexclient.SearchResult{
		Title:            "Pilot",
		Type:             "episode",
		GrandparentTitle: &showTwo,
		ParentTitle:      &seasonOne,
		ParentIndex:      &parentIndex,
		Index:            &episodeIndex,
	})

	if first == second {
		t.Fatalf("expected episode fallback keys to preserve series context, got %q", first)
	}
}

func TestScoreRecommendationAndReason(t *testing.T) {
	candidate := &recommendationCandidate{
		SeedHits:       map[string]struct{}{"1": {}, "2": {}},
		GenreOverlap:   2,
		CountryOverlap: 1,
		YearNearSeed:   true,
	}

	if got := scoreRecommendation(candidate); got != 228 {
		t.Fatalf("scoreRecommendation() = %d, want 228", got)
	}

	if got := recommendationReason(candidate); got != "similar-to-2-seeds, genre-overlap, country-overlap, year-near-seed" {
		t.Fatalf("recommendationReason() = %q", got)
	}
}

func TestIsNearSeedYear(t *testing.T) {
	year := 1988
	if !isNearSeedYear(&year, []int{1990}) {
		t.Fatal("expected year to be near seed")
	}

	year = 1970
	if isNearSeedYear(&year, []int{1990}) {
		t.Fatal("expected year not to be near seed")
	}
}

func TestRankRecommendationCandidates_IncludeSeeds(t *testing.T) {
	seedGUID := "guid://seed"
	seed := plexclient.SearchResult{
		RatingKey: "1",
		Title:     "Stalker",
		Type:      "movie",
		GUID:      &seedGUID,
	}
	candidateGUID := "guid://candidate"
	candidate := plexclient.SearchResult{
		RatingKey: "2",
		Title:     "Avalon",
		Type:      "movie",
		GUID:      &candidateGUID,
	}

	candidates := map[string]*recommendationCandidate{
		recommendationDedupeKey(seed): {
			Item:     seed,
			SeedHits: map[string]struct{}{"1": {}},
		},
		recommendationDedupeKey(candidate): {
			Item:     candidate,
			SeedHits: map[string]struct{}{"1": {}, "2": {}},
		},
	}

	withoutSeeds := rankRecommendationCandidates(
		candidates,
		map[string]struct{}{"1": {}},
		map[string]struct{}{recommendationDedupeKey(seed): {}},
		map[string]struct{}{},
		map[string]struct{}{},
		nil,
		false,
		10,
	)
	if len(withoutSeeds) != 1 || withoutSeeds[0].RatingKey != "2" {
		t.Fatalf("expected only non-seed candidate, got %#v", withoutSeeds)
	}

	withSeeds := rankRecommendationCandidates(
		candidates,
		map[string]struct{}{"1": {}},
		map[string]struct{}{recommendationDedupeKey(seed): {}},
		map[string]struct{}{},
		map[string]struct{}{},
		nil,
		true,
		1,
	)
	if len(withSeeds) != 1 {
		t.Fatalf("expected one ranked result under limiting, got %#v", withSeeds)
	}
	if withSeeds[0].RatingKey != "2" {
		t.Fatalf("expected highest-ranked item to win under limiting, got %#v", withSeeds)
	}
}

func TestLimitRecommendationItems_PreservesRanking(t *testing.T) {
	recommendations := []RecommendationItem{
		{RatingKey: "2", Title: "High Score", Score: 300, SeedHits: 2},
		{RatingKey: "3", Title: "Mid Score", Score: 200, SeedHits: 1},
		{RatingKey: "1", Title: "Seed", Score: 100, SeedHits: 1},
	}

	limited := limitRecommendationItems(recommendations, 2)
	if len(limited) != 2 {
		t.Fatalf("expected 2 limited items, got %#v", limited)
	}
	if limited[0].RatingKey != "2" || limited[1].RatingKey != "3" {
		t.Fatalf("expected top-ranked items to be kept, got %#v", limited)
	}
}

func TestRankRecommendationCandidates_ExcludesSeedCopiesSharingGUID(t *testing.T) {
	seedGUID := "guid://seed"
	candidate := plexclient.SearchResult{
		RatingKey: "99",
		Title:     "Stalker",
		Type:      "movie",
		GUID:      &seedGUID,
	}

	ranked := rankRecommendationCandidates(
		map[string]*recommendationCandidate{
			recommendationDedupeKey(candidate): {
				Item:     candidate,
				SeedHits: map[string]struct{}{"1": {}},
			},
		},
		map[string]struct{}{"1": {}},
		map[string]struct{}{recommendationDedupeKey(plexclient.SearchResult{RatingKey: "1", GUID: &seedGUID}): {}},
		map[string]struct{}{},
		map[string]struct{}{},
		nil,
		false,
		10,
	)
	if len(ranked) != 0 {
		t.Fatalf("expected logical seed duplicate to be excluded, got %#v", ranked)
	}
}

func TestResolveSeeds_RequiresDistinctLogicalSeeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/library/metadata/1":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"1","key":"/library/metadata/1","title":"Avalon","type":"movie","guid":"tmdb://123"}]}}`))
		case "/library/metadata/2":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"2","key":"/library/metadata/2","title":"Avalon","type":"movie","guid":"tmdb://123"}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	u := ui.New(ui.Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, ColorMode: ui.ColorNever})
	cmd := RecommendCmd{Like: []string{"1", "2"}, Type: "movie"}

	_, err = cmd.resolveSeeds(u, client, context.Background())
	if err == nil {
		t.Fatal("expected duplicate logical seeds to fail distinct-seed validation")
	}
	if !strings.Contains(err.Error(), "at least two distinct seeds are required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRecommend_FiltersCandidatesByTypeAndSection(t *testing.T) {
	sectionOne := 1
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/1/similar" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"2","key":"/library/metadata/2","title":"Avalon","type":"movie","librarySectionID":1},{"ratingKey":"3","key":"/library/metadata/3","title":"Serial Experiments Lain","type":"show","librarySectionID":1},{"ratingKey":"4","key":"/library/metadata/4","title":"Ghost in the Shell","type":"movie","librarySectionID":2}]}}`))
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	cmd := RecommendCmd{Type: "movie", Section: "1", Limit: 10}
	results, err := cmd.recommend(client, context.Background(), []recommendationSeed{
		{
			Input: "1",
			Item: plexclient.SearchResult{
				RatingKey:        "1",
				Title:            "Seed",
				Type:             "movie",
				LibrarySectionID: &sectionOne,
			},
		},
	})
	if err != nil {
		t.Fatalf("recommend() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected only one filtered recommendation, got %#v", results)
	}
	if results[0].RatingKey != "2" {
		t.Fatalf("expected in-section movie result to survive, got %#v", results)
	}
}

func TestResolveSeeds_AmbiguousSeedOutputsCandidatesToStderr(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"1","key":"/library/metadata/1","title":"Crash","type":"movie","year":1996},{"ratingKey":"2","key":"/library/metadata/2","title":"Crash","type":"movie","year":2004}]}]}}`))
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	u := ui.New(ui.Options{Out: &out, Err: &errOut, ColorMode: ui.ColorNever})
	cmd := RecommendCmd{Like: []string{"Crash"}, Type: "movie", Output: "json"}

	_, err = cmd.resolveSeeds(u, client, context.Background())
	if err == nil {
		t.Fatal("expected ambiguous seed resolution to fail")
	}
	if out.Len() != 0 {
		t.Fatalf("expected stdout to stay empty on error, got %q", out.String())
	}
	if !strings.Contains(errOut.String(), `"rating_key": "1"`) || !strings.Contains(errOut.String(), `"rating_key": "2"`) {
		t.Fatalf("expected JSON candidates on stderr, got %q", errOut.String())
	}
}

func TestResolveSeeds_MetadataLookupFailureFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/hubs/search":
			switch r.URL.Query().Get("query") {
			case "Avalon":
				_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"1","key":"/library/metadata/1","title":"Avalon","type":"movie"}]}]}}`))
			case "Stalker":
				_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"2","key":"/library/metadata/2","title":"Stalker","type":"movie"}]}]}}`))
			default:
				t.Fatalf("unexpected query: %s", r.URL.Query().Get("query"))
			}
		case "/library/metadata/1":
			http.Error(w, `{"error":"temporary failure"}`, http.StatusBadGateway)
		case "/library/metadata/2":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"2","key":"/library/metadata/2","title":"Stalker","type":"movie","guid":"tmdb://2"}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	u := ui.New(ui.Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, ColorMode: ui.ColorNever})
	cmd := RecommendCmd{Like: []string{"Avalon", "Stalker"}, Type: "movie"}

	_, err = cmd.resolveSeeds(u, client, context.Background())
	if err == nil {
		t.Fatal("expected metadata lookup failure to abort seed resolution")
	}
	if !strings.Contains(err.Error(), `failed to load metadata for seed "Avalon"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
