package plexclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSearchLibrary_DedupesSharedGUIDAcrossHubs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"MediaContainer": {
				"Hub": [
					{
						"Metadata": [
							{
								"ratingKey": "42",
								"key": "/library/metadata/42",
								"title": "Ghost in the Shell",
								"type": "movie",
								"guid": "plex://movie/123",
								"librarySectionID": 1,
								"librarySectionTitle": "Movies"
							}
						]
					},
					{
						"Metadata": [
							{
								"ratingKey": "42",
								"key": "/library/metadata/42",
								"title": "Ghost in the Shell",
								"type": "movie",
								"guid": "plex://movie/123",
								"thumb": "/library/metadata/99/thumb/1"
							}
						]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	results, err := client.SearchLibrary(context.Background(), "Ghost in the Shell", nil, 10)
	if err != nil {
		t.Fatalf("SearchLibrary() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 logical result, got %#v", results)
	}
	if results[0].RatingKey != "42" {
		t.Fatalf("expected first local rating key to survive merge, got %#v", results[0])
	}
	if results[0].Key != "/library/metadata/42" {
		t.Fatalf("expected first metadata key to survive merge, got %#v", results[0])
	}
	if results[0].GUID == nil || *results[0].GUID != "plex://movie/123" {
		t.Fatalf("expected shared GUID to be preserved, got %#v", results[0])
	}
	if results[0].Thumb == nil || *results[0].Thumb != "/library/metadata/99/thumb/1" {
		t.Fatalf("expected merged thumb metadata, got %#v", results[0])
	}
}

func TestGetMoviesPagesFullSectionAndFiltersActorMetadata(t *testing.T) {
	var starts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections/14/all" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		start := r.URL.Query().Get("X-Plex-Container-Start")
		starts = append(starts, start)
		w.Header().Set("Content-Type", "application/json")

		switch start {
		case "0":
			var b strings.Builder
			b.WriteString(`{"MediaContainer":{"Metadata":[`)
			for i := 0; i < DefaultPageSize; i++ {
				if i > 0 {
					b.WriteByte(',')
				}
				fmt.Fprintf(&b, `{"ratingKey":"%d","title":"Movie %d","type":"movie"}`, i, i)
			}
			b.WriteString(`]}}`)
			_, _ = w.Write([]byte(b.String()))
		case "100":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"100","title":"Drunken Master","originalTitle":"醉拳","type":"movie","year":1978,"Director":[{"tag":"Yuen Woo-Ping"}],"Role":[{"tag":"Jackie Chan"}],"Genre":[{"tag":"Action"}],"Country":[{"tag":"Hong Kong"}]}]}}`))
		default:
			t.Fatalf("unexpected page start: %s", start)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	movies, err := client.GetMovies(context.Background(), "14", MovieFilters{Actor: []string{"Jackie"}})
	if err != nil {
		t.Fatalf("GetMovies() error = %v", err)
	}
	if len(movies) != 1 {
		t.Fatalf("expected one actor-filtered movie from the second page, got %#v", movies)
	}
	if movies[0].RatingKey != "100" || movies[0].OriginalTitle != "醉拳" {
		t.Fatalf("expected second-page movie metadata to survive conversion, got %#v", movies[0])
	}
	if strings.Join(starts, ",") != "0,100" {
		t.Fatalf("expected paged section fetches, got %q", strings.Join(starts, ","))
	}
}

func TestMovieFiltersMatchRepeatedValuesAndDedupeModes(t *testing.T) {
	movies := []MovieInfo{
		{RatingKey: "1", GUID: "plex://movie/a", Title: "Enter the Dragon", Year: 1973, Actors: []string{"Bruce Lee"}, Countries: []string{"Hong Kong"}},
		{RatingKey: "2", GUID: "plex://movie/a", Title: "Enter the Dragon", Year: 1973, Actors: []string{"Bruce Lee"}, Countries: []string{"Hong Kong"}},
		{RatingKey: "3", GUID: "plex://movie/b", Title: "Drunken Master", Year: 1978, Actors: []string{"Jackie Chan"}, Countries: []string{"Hong Kong"}},
	}

	filtered := make([]MovieInfo, 0, len(movies))
	for _, movie := range movies {
		if movieMatchesFilters(movie, MovieFilters{
			Actor:   []string{"Jackie Chan", "Bruce Lee"},
			Country: []string{"Hong Kong"},
		}) {
			filtered = append(filtered, movie)
		}
	}
	if len(filtered) != 3 {
		t.Fatalf("expected repeated actor values to OR-match all movies, got %#v", filtered)
	}

	deduped := dedupeMovies(filtered, "guid")
	if len(deduped) != 2 {
		t.Fatalf("expected guid dedupe to collapse duplicate copies, got %#v", deduped)
	}
	notDeduped := dedupeMovies(filtered, "none")
	if len(notDeduped) != 3 {
		t.Fatalf("expected none dedupe to preserve copies, got %#v", notDeduped)
	}
}

func TestSearchLibrary_PreservesDistinctSectionsWhenOneHubOmitsSectionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"MediaContainer": {
				"Hub": [
					{
						"Metadata": [
							{
								"ratingKey": "42",
								"key": "/library/metadata/42",
								"title": "Heat",
								"type": "movie",
								"guid": "plex://movie/heat",
								"librarySectionID": 1,
								"librarySectionTitle": "Movies"
							}
						]
					},
					{
						"Metadata": [
							{
								"ratingKey": "84",
								"key": "/library/metadata/84",
								"title": "Heat",
								"type": "movie",
								"guid": "plex://movie/heat"
							}
						]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	results, err := client.SearchLibrary(context.Background(), "Heat", nil, 10)
	if err != nil {
		t.Fatalf("SearchLibrary() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected ambiguous cross-section copies to stay separate, got %#v", results)
	}
}

func TestSearchLibrary_PreservesDistinctSectionsSharingGUID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"MediaContainer": {
				"Hub": [
					{
						"Metadata": [
							{
								"ratingKey": "42",
								"key": "/library/metadata/42",
								"title": "Heat",
								"type": "movie",
								"guid": "plex://movie/heat",
								"librarySectionID": 1,
								"librarySectionTitle": "Movies"
							}
						]
					},
					{
						"Metadata": [
							{
								"ratingKey": "84",
								"key": "/library/metadata/84",
								"title": "Heat",
								"type": "movie",
								"guid": "plex://movie/heat",
								"librarySectionID": 2,
								"librarySectionTitle": "4K Movies"
							}
						]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	results, err := client.SearchLibrary(context.Background(), "Heat", nil, 10)
	if err != nil {
		t.Fatalf("SearchLibrary() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected section-distinct results to survive, got %#v", results)
	}
	if results[0].LibrarySectionID == nil || *results[0].LibrarySectionID != 1 {
		t.Fatalf("expected first result to keep section 1, got %#v", results[0])
	}
	if results[1].LibrarySectionID == nil || *results[1].LibrarySectionID != 2 {
		t.Fatalf("expected second result to keep section 2, got %#v", results[1])
	}
}

func TestSearchLibrary_PreservesDistinctEpisodesWithSharedFallbackTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"MediaContainer": {
				"Hub": [
					{
						"Metadata": [
							{
								"ratingKey": "101",
								"key": "/library/metadata/101",
								"title": "Pilot",
								"type": "episode",
								"year": 2024,
								"grandparentTitle": "Show One",
								"parentTitle": "Season 1",
								"parentIndex": 1,
								"index": 1
							}
						]
					},
					{
						"Metadata": [
							{
								"title": "Pilot",
								"type": "episode",
								"year": 2024,
								"grandparentTitle": "Show Two",
								"parentTitle": "Season 1",
								"parentIndex": 1,
								"index": 1,
								"guid": "plex://episode/202"
							}
						]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	results, err := client.SearchLibrary(context.Background(), "Pilot", nil, 10)
	if err != nil {
		t.Fatalf("SearchLibrary() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected title-only episodic collisions to stay distinct, got %#v", results)
	}
}

func TestCreateSmartPlaylist_RejectsEmptyLegacyFilterValue(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Fatalf("CreateSmartPlaylist() should fail before making a request, got %s", r.URL.String())
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	playlist, err := client.CreateSmartPlaylist(context.Background(), "Directors", "video", "1", "director", "   ")
	if err == nil {
		t.Fatal("expected empty legacy filter value to fail")
	}
	if playlist != nil {
		t.Fatalf("expected nil playlist on validation failure, got %#v", playlist)
	}
	if !strings.Contains(err.Error(), "filter value is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no outbound requests, got %d", requests)
	}
}

func TestCreateSmartPlaylistWithFilters_RejectsWhitespaceOnlyFilters(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Fatalf("CreateSmartPlaylistWithFilters() should fail before making a request, got %s", r.URL.String())
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	playlist, err := client.CreateSmartPlaylistWithFilters(context.Background(), "Blank", "video", "1", SmartPlaylistFilters{
		Directors: []string{"", "   "},
	})
	if err == nil {
		t.Fatal("expected whitespace-only filters to fail")
	}
	if playlist != nil {
		t.Fatalf("expected nil playlist on validation failure, got %#v", playlist)
	}
	if !strings.Contains(err.Error(), "at least one filter is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no outbound requests, got %d", requests)
	}
}

func TestCreateSmartPlaylistWithFilters_RejectsNegativeYears(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		t.Fatalf("CreateSmartPlaylistWithFilters() should fail before making a request, got %s", r.URL.String())
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	playlist, err := client.CreateSmartPlaylistWithFilters(context.Background(), "Years", "video", "1", SmartPlaylistFilters{
		YearFrom: -1,
	})
	if err == nil {
		t.Fatal("expected negative year bounds to fail")
	}
	if playlist != nil {
		t.Fatalf("expected nil playlist on validation failure, got %#v", playlist)
	}
	if !strings.Contains(err.Error(), "year-from cannot be negative") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no outbound requests, got %d", requests)
	}
}

func TestCreateSmartPlaylistWithFilters_AllowsMultipleGenreFilters(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.URL.Query().Get("uri"); got == "" {
			t.Fatalf("expected smart playlist URI query, got %s", r.URL.String())
		} else if decoded, err := url.QueryUnescape(got); err != nil {
			t.Fatalf("QueryUnescape() error = %v", err)
		} else if !strings.Contains(decoded, "genre=21%2C22") && !strings.Contains(decoded, "genre=21,22") {
			t.Fatalf("expected OR genre filter in URI, got %q", decoded)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"999","key":"/playlists/999","title":"Genres","type":"playlist","playlistType":"video","leafCount":10,"smart":true}]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	playlist, err := client.CreateSmartPlaylistWithFilters(context.Background(), "Genres", "video", "1", SmartPlaylistFilters{
		Genres: []string{"21", "22"},
	})
	if err != nil {
		t.Fatalf("expected multiple genre filters to succeed, got %v", err)
	}
	if playlist == nil || playlist.RatingKey != "999" {
		t.Fatalf("expected created playlist metadata, got %#v", playlist)
	}
	if requests != 1 {
		t.Fatalf("expected one outbound request, got %d", requests)
	}
}

func TestGetRelatedItems_EncodesMetadataIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.RequestURI, "/library/metadata/abc%2Fdef,456/related?") {
			t.Fatalf("unexpected request URI: %s", r.RequestURI)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := client.GetRelatedItems(context.Background(), "abc/def, 456"); err != nil {
		t.Fatalf("GetRelatedItems() error = %v", err)
	}
}

func TestListSimilar_EncodesMetadataIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.RequestURI, "/library/metadata/abc%2Fdef,456/similar?") {
			t.Fatalf("unexpected request URI: %s", r.RequestURI)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := client.ListSimilar(context.Background(), "abc/def, 456", 10); err != nil {
		t.Fatalf("ListSimilar() error = %v", err)
	}
}

func TestGetMetadataSearchResult_PreservesCollectionsFromMetadataLookup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"MediaContainer": {
				"Metadata": [
					{
						"ratingKey": "123",
						"key": "/library/metadata/123",
						"title": "Avalon",
						"type": "episode",
						"thumb": "/library/metadata/123/thumb/1",
						"parentTitle": "Season 1",
						"parentIndex": 1,
						"index": 7,
						"Collection": [
							{"tag": "Cyberpunk"},
							{"tag": "Anime Essentials"}
						]
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	result, err := client.GetMetadataSearchResult(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetMetadataSearchResult() error = %v", err)
	}
	if strings.Join(result.Collections, ",") != "Cyberpunk,Anime Essentials" {
		t.Fatalf("expected collections to survive metadata conversion, got %#v", result)
	}
	if result.Thumb == nil || *result.Thumb != "/library/metadata/123/thumb/1" {
		t.Fatalf("expected thumb to survive metadata conversion, got %#v", result)
	}
	if result.ParentTitle == nil || *result.ParentTitle != "Season 1" {
		t.Fatalf("expected parent title to survive metadata conversion, got %#v", result)
	}
	if result.ParentIndex == nil || *result.ParentIndex != 1 {
		t.Fatalf("expected parent index to survive metadata conversion, got %#v", result)
	}
	if result.Index == nil || *result.Index != 7 {
		t.Fatalf("expected item index to survive metadata conversion, got %#v", result)
	}
}

func TestGetRelatedItems_PreservesThumbAndParentContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/123/related" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"MediaContainer": {
				"Metadata": [
					{
						"ratingKey": "456",
						"key": "/library/metadata/456",
						"title": "Stand Alone Complex",
						"type": "episode",
						"guid": "plex://episode/456",
						"thumb": "/library/metadata/456/thumb/1",
						"parentTitle": "Season 2",
						"parentIndex": 2,
						"index": 3
					}
				]
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	items, err := client.GetRelatedItems(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetRelatedItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 related item, got %#v", items)
	}
	if items[0].Thumb == nil || *items[0].Thumb != "/library/metadata/456/thumb/1" {
		t.Fatalf("expected thumb to survive discovery conversion, got %#v", items[0])
	}
	if items[0].ParentTitle == nil || *items[0].ParentTitle != "Season 2" {
		t.Fatalf("expected parent title to survive discovery conversion, got %#v", items[0])
	}
	if items[0].ParentIndex == nil || *items[0].ParentIndex != 2 {
		t.Fatalf("expected parent index to survive discovery conversion, got %#v", items[0])
	}
	if items[0].Index == nil || *items[0].Index != 3 {
		t.Fatalf("expected item index to survive discovery conversion, got %#v", items[0])
	}
}
