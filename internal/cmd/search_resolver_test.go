package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dogbertdev/plexcli/internal/plexclient"
)

func TestResolveSingleSearchResult_DedupesDuplicateHubMatches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"42","key":"/library/metadata/42","title":"Ghost in the Shell","type":"movie","year":1995,"guid":"tmdb://9323","librarySectionID":1}]},{"Metadata":[{"ratingKey":"42","key":"/library/metadata/42","title":"Ghost in the Shell","type":"movie","year":1995,"guid":"tmdb://9323"}]}]}}`))
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	result, err := resolveSingleSearchResult(context.Background(), client, "Ghost in the Shell", SearchResolveOptions{
		Limit: plexclient.DefaultSearchLimit,
		Type:  "movie",
	})
	if err != nil {
		t.Fatalf("resolveSingleSearchResult() error = %v", err)
	}
	if result.RatingKey != "42" {
		t.Fatalf("expected deduped match to survive, got %#v", result)
	}
	if result.LibrarySectionID == nil || *result.LibrarySectionID != 1 {
		t.Fatalf("expected merged section metadata to survive duplicate hubs, got %#v", result)
	}
}

func TestResolveSingleSearchResult_RatingKeyRespectsSectionFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"123","key":"/library/metadata/123","title":"Avalon","type":"movie","librarySectionID":1,"librarySectionTitle":"Movies"}]}}`))
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	result, err := resolveSingleSearchResult(context.Background(), client, "123", SearchResolveOptions{
		SectionID:      "1",
		Type:           "movie",
		AllowRatingKey: true,
	})
	if err != nil {
		t.Fatalf("resolveSingleSearchResult() error = %v", err)
	}
	if result.RatingKey != "123" {
		t.Fatalf("expected rating-key lookup to resolve directly, got %#v", result)
	}
}

func TestResolveSingleSearchResult_RatingKeyFallsBackToSearchWhenFilteredOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/library/metadata/123":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"123","key":"/library/metadata/123","title":"Avalon","type":"movie","librarySectionID":2}]}}`))
		case "/hubs/search":
			_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"456","key":"/library/metadata/456","title":"123","type":"movie","librarySectionID":1}]}]}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	result, err := resolveSingleSearchResult(context.Background(), client, "123", SearchResolveOptions{
		SectionID:      "1",
		Type:           "movie",
		AllowRatingKey: true,
	})
	if err != nil {
		t.Fatalf("resolveSingleSearchResult() error = %v", err)
	}
	if result.RatingKey != "456" {
		t.Fatalf("expected filtered rating-key lookup to fall back to search result, got %#v", result)
	}
}

func TestResolveSearchResults_EnforcesFinalLimitAcrossHubs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"1","key":"/library/metadata/1","title":"Heat","type":"movie"}]},{"Metadata":[{"ratingKey":"2","key":"/library/metadata/2","title":"Heat 2","type":"movie"}]},{"Metadata":[{"ratingKey":"3","key":"/library/metadata/3","title":"Heat 3","type":"movie"}]}]}}`))
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	results, err := resolveSearchResults(context.Background(), client, "Heat", SearchResolveOptions{
		Limit: 2,
		Type:  "movie",
	})
	if err != nil {
		t.Fatalf("resolveSearchResults() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected final results to be capped to 2, got %#v", results)
	}
	if results[0].RatingKey != "1" || results[1].RatingKey != "2" {
		t.Fatalf("expected limit to keep the first 2 merged results, got %#v", results)
	}
}

func TestResolveSearchResults_FailAmbiguousBypassesFinalLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hubs/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("limit"); got != "50" {
			t.Fatalf("expected fail-ambiguous search to overfetch default limit, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Hub":[{"Metadata":[{"ratingKey":"1","key":"/library/metadata/1","title":"Crash","type":"movie","year":1996},{"ratingKey":"2","key":"/library/metadata/2","title":"Crash","type":"movie","year":2004}]}]}}`))
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	results, err := resolveSearchResults(context.Background(), client, "Crash", SearchResolveOptions{
		Limit:         1,
		Type:          "movie",
		FailAmbiguous: true,
	})
	if err != nil {
		t.Fatalf("resolveSearchResults() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected fail-ambiguous mode to preserve all filtered results, got %#v", results)
	}
}
