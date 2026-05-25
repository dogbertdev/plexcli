package cmd

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

func TestResolvePlaylistItems_PreservesExplicitAndQueryDuplicates(t *testing.T) {
	client, cleanup := newPlaylistTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "Stalker" {
			t.Fatalf("unexpected query: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"2","key":"/library/metadata/2","title":"Stalker","type":"movie"}]}]}}`))
	})
	defer cleanup()

	u := ui.New(ui.Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, ColorMode: ui.ColorNever})
	items, err := resolvePlaylistItems(u, client, context.Background(), "tsv", []string{"1", "2", "1"}, []string{"Stalker"}, SearchResolveOptions{
		Limit: plexclient.DefaultSearchLimit,
	})
	if err != nil {
		t.Fatalf("resolvePlaylistItems() error = %v", err)
	}

	want := []string{"1", "2", "1", "2"}
	if len(items) != len(want) {
		t.Fatalf("expected %d items, got %#v", len(want), items)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("item %d = %q, want %q", i, items[i], want[i])
		}
	}
}

func TestResolvePlaylistItems_AmbiguousQueryOutputsCandidates(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	u := ui.New(ui.Options{Out: &out, Err: &errOut, ColorMode: ui.ColorNever})

	client, cleanup := newPlaylistTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"1","key":"/library/metadata/1","title":"Crash","type":"movie","year":1996},{"ratingKey":"2","key":"/library/metadata/2","title":"Crash","type":"movie","year":2004}]}]}}`))
	})
	defer cleanup()

	_, err := resolvePlaylistItems(u, client, context.Background(), "tsv", nil, []string{"Crash"}, SearchResolveOptions{
		Limit: plexclient.DefaultSearchLimit,
	})
	if err == nil {
		t.Fatal("expected ambiguous query to fail")
	}
	if !strings.Contains(err.Error(), `query "Crash" is ambiguous`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected stdout to stay empty on error, got %q", out.String())
	}
	if !strings.Contains(errOut.String(), "1\tCrash\tmovie\t1996") || !strings.Contains(errOut.String(), "2\tCrash\tmovie\t2004") {
		t.Fatalf("expected candidate rows on stderr, got %q", errOut.String())
	}
}

func TestParsePlaylistItemTokens_AcceptsWhitespaceAndCommas(t *testing.T) {
	got := parsePlaylistItemTokens("  1,2\n3\t4\r\n  ")
	want := []string{"1", "2", "3", "4"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("parsePlaylistItemTokens() = %#v, want %#v", got, want)
	}
}

func TestPreviewPlaylistItems_ResolvesRatingKeys(t *testing.T) {
	client, cleanup := newPlaylistTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/42" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"42","key":"/library/metadata/42","title":"Drunken Master","type":"movie","year":1978}]}}`))
	})
	defer cleanup()

	items, err := previewPlaylistItems(client, context.Background(), []string{"42"})
	if err != nil {
		t.Fatalf("previewPlaylistItems() error = %v", err)
	}
	if len(items) != 1 || items[0].RatingKey != "42" || items[0].Title != "Drunken Master" || items[0].Year != 1978 {
		t.Fatalf("unexpected preview items: %#v", items)
	}
}

func TestPlaylistCreateResolveItems_ConstrainsQueriesByPlaylistType(t *testing.T) {
	client, cleanup := newPlaylistTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"2","key":"/library/metadata/2","title":"Moon","type":"movie"}]}]}}`))
	})
	defer cleanup()

	cmd := PlaylistCreateCmd{Type: "audio", Queries: []string{"Moon"}, Output: "tsv"}
	u := ui.New(ui.Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, ColorMode: ui.ColorNever})
	_, err := cmd.resolveItems(u, client, context.Background())
	if err == nil {
		t.Fatal("expected audio playlist query resolution to reject movie matches")
	}

	var noResultsErr *NoSearchResultsError
	if !errors.As(err, &noResultsErr) {
		t.Fatalf("expected no results error, got %v", err)
	}
}

func TestPlaylistAddResolveItems_UsesTargetPlaylistType(t *testing.T) {
	inspectedPlaylist := false
	client, cleanup := newPlaylistTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/playlists":
			inspectedPlaylist = true
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"55","title":"Mix","type":"playlist","playlistType":"audio","leafCount":0}]}}`))
		case "/hubs/search":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"2","key":"/library/metadata/2","title":"Moon","type":"movie"}]}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
	defer cleanup()

	cmd := PlaylistAddCmd{Playlist: "55", Queries: []string{"Moon"}, Output: "tsv"}
	u := ui.New(ui.Options{Out: &bytes.Buffer{}, Err: &bytes.Buffer{}, ColorMode: ui.ColorNever})
	_, err := cmd.resolveItems(u, client, context.Background())
	if err == nil {
		t.Fatal("expected playlist add query resolution to reject movie matches for an audio playlist")
	}
	if !inspectedPlaylist {
		t.Fatal("expected playlist metadata to be inspected before resolving queries")
	}

	var noResultsErr *NoSearchResultsError
	if !errors.As(err, &noResultsErr) {
		t.Fatalf("expected no results error, got %v", err)
	}
}

func TestPlaylistSmartResolveFilters_DedupesDuplicateValues(t *testing.T) {
	client, cleanup := newPlaylistTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/library/sections/1/director":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"11","title":"Mamoru Oshii"}]}}`))
		case "/library/sections/1/genre":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"21","title":"Cyberpunk"}]}}`))
		case "/library/sections/1/country":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"31","title":"Japan"}]}}`))
		case "/library/sections/1/collection":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"41","title":"Anime"}]}}`))
		case "/library/sections/1/studio":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"51","title":"Production I.G"}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	})
	defer cleanup()

	cmd := PlaylistSmartCmd{
		Section:    "1",
		Director:   []string{"Mamoru Oshii", "Mamoru Oshii"},
		Genre:      []string{"Cyberpunk"},
		Country:    []string{"Japan"},
		Collection: []string{"Anime"},
		Studio:     []string{"Production IG"},
	}

	filters, err := cmd.resolveFilters(context.Background(), client)
	if err != nil {
		t.Fatalf("resolveFilters() error = %v", err)
	}
	if strings.Join(filters.Directors, ",") != "11" {
		t.Fatalf("unexpected directors: %#v", filters.Directors)
	}
	if strings.Join(filters.Genres, ",") != "21" {
		t.Fatalf("unexpected genres: %#v", filters.Genres)
	}
	if strings.Join(filters.Countries, ",") != "31" {
		t.Fatalf("unexpected countries: %#v", filters.Countries)
	}
	if strings.Join(filters.Collections, ",") != "41" {
		t.Fatalf("unexpected collections: %#v", filters.Collections)
	}
	if strings.Join(filters.Studios, ",") != "51" {
		t.Fatalf("unexpected studios: %#v", filters.Studios)
	}
}

func TestPlaylistSmartValidate_AllowsMultipleGenreValues(t *testing.T) {
	cmd := PlaylistSmartCmd{
		Section: "1",
		Genre:   []string{"Cyberpunk", "Mecha"},
	}

	err := cmd.validate()
	if err != nil {
		t.Fatalf("expected multiple genre values to be allowed, got %v", err)
	}
}

func TestPlaylistSmartResolveFilters_AmbiguousValueFails(t *testing.T) {
	client, cleanup := newPlaylistTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections/1/genre" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"21","title":"Cyberpunk"},{"key":"22","title":"Cyber Thriller"}]}}`))
	})
	defer cleanup()

	cmd := PlaylistSmartCmd{
		Section: "1",
		Genre:   []string{"Cyber"},
	}

	_, err := cmd.resolveFilters(context.Background(), client)
	if err == nil {
		t.Fatal("expected ambiguous filter value to fail")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlaylistSmartValidate_RejectsNegativeYears(t *testing.T) {
	cmd := PlaylistSmartCmd{
		Section:  "1",
		YearFrom: -1,
	}

	err := cmd.validate()
	if err == nil {
		t.Fatal("expected negative year-from to fail")
	}
	if !strings.Contains(err.Error(), "--year-from cannot be negative") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlaylistSmartNoFilters_IgnoresBlankValues(t *testing.T) {
	cmd := PlaylistSmartCmd{
		Director: []string{"", "   "},
		Genre:    []string{"\t"},
	}

	if !cmd.noFilters() {
		t.Fatalf("expected blank filter values to be treated as empty: %#v", cmd)
	}
}

func TestSortPlaylistItems_ByYearAscending(t *testing.T) {
	year1978 := 1978
	year1972 := 1972
	year2000 := 2000
	items := []plexclient.SearchResult{
		{RatingKey: "1", Title: "Drunken Master", Year: &year1978},
		{RatingKey: "2", Title: "Fist of Fury", Year: &year1972},
		{RatingKey: "3", Title: "Unknown"},
		{RatingKey: "4", Title: "Crouching Tiger", Year: &year2000},
	}

	sorted := sortPlaylistItems(items, "year", "asc")

	got := []string{sorted[0].RatingKey, sorted[1].RatingKey, sorted[2].RatingKey, sorted[3].RatingKey}
	want := []string{"2", "1", "4", "3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected sort order: got %#v want %#v", got, want)
	}
}

func TestPlanPlaylistMoves_SkipsAlreadySortedItems(t *testing.T) {
	items := []plexclient.SearchResult{
		{RatingKey: "1", PlaylistItemID: "11", Title: "A"},
		{RatingKey: "2", PlaylistItemID: "22", Title: "B"},
	}

	moves, err := planPlaylistMoves(items, append([]plexclient.SearchResult{}, items...))
	if err != nil {
		t.Fatalf("planPlaylistMoves() error = %v", err)
	}
	if len(moves) != 0 {
		t.Fatalf("expected no moves for an already sorted playlist, got %#v", moves)
	}
}

func TestPlanPlaylistMoves_BuildsMinimalMoveSequence(t *testing.T) {
	current := []plexclient.SearchResult{
		{RatingKey: "3", PlaylistItemID: "33", Title: "C"},
		{RatingKey: "1", PlaylistItemID: "11", Title: "A"},
		{RatingKey: "2", PlaylistItemID: "22", Title: "B"},
	}
	desired := []plexclient.SearchResult{
		{RatingKey: "1", PlaylistItemID: "11", Title: "A"},
		{RatingKey: "2", PlaylistItemID: "22", Title: "B"},
		{RatingKey: "3", PlaylistItemID: "33", Title: "C"},
	}

	moves, err := planPlaylistMoves(current, desired)
	if err != nil {
		t.Fatalf("planPlaylistMoves() error = %v", err)
	}
	if len(moves) != 2 {
		t.Fatalf("expected 2 moves, got %#v", moves)
	}
	if moves[0].Item.PlaylistItemID != "11" || moves[0].AfterPlaylistItemID != "" {
		t.Fatalf("unexpected first move: %#v", moves[0])
	}
	if moves[1].Item.PlaylistItemID != "22" || moves[1].AfterPlaylistItemID != "11" {
		t.Fatalf("unexpected second move: %#v", moves[1])
	}
}

func TestGetPlaylistItems_PreservesPlaylistItemID(t *testing.T) {
	client, cleanup := newPlaylistTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/playlists/55/items" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"42","playlistItemID":99,"key":"/library/metadata/42","title":"Drunken Master","type":"movie","year":1978}]}}`))
	})
	defer cleanup()

	items, err := client.GetPlaylistItems(context.Background(), "55")
	if err != nil {
		t.Fatalf("GetPlaylistItems() error = %v", err)
	}
	if len(items) != 1 || items[0].PlaylistItemID != "99" {
		t.Fatalf("expected playlist item ID to be preserved, got %#v", items)
	}
}

func newPlaylistTestClient(t *testing.T, handler http.HandlerFunc) (*plexclient.Client, func()) {
	t.Helper()

	server := httptest.NewServer(handler)
	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		server.Close()
		t.Fatalf("NewClient() error = %v", err)
	}

	return client, server.Close
}
