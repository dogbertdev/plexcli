package plexclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetItemMetadata_EmptyMetadataIsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/999" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	item, err := client.GetItemMetadata(context.Background(), "999")
	if err == nil {
		t.Fatal("expected missing metadata to return an error")
	}
	if item != nil {
		t.Fatalf("expected nil metadata, got %#v", item)
	}
}

func TestGetMetadataSearchResult_EmptyMetadataDoesNotBackfillRatingKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/999" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	result, err := client.GetMetadataSearchResult(context.Background(), "999")
	if err == nil {
		t.Fatal("expected missing metadata search result to fail")
	}
	if result.RatingKey != "" {
		t.Fatalf("expected empty result on failure, got %#v", result)
	}
}

func TestGetMetadataSearchResult_PreservesLibrarySectionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/123" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Metadata":[{"ratingKey":"123","key":"/library/metadata/123","title":"Avalon","type":"movie","librarySectionID":1,"librarySectionTitle":"Movies"}]}}`))
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
	if result.LibrarySectionID == nil || *result.LibrarySectionID != 1 {
		t.Fatalf("expected section ID to survive metadata conversion, got %#v", result)
	}
	if result.LibrarySectionTitle == nil || *result.LibrarySectionTitle != "Movies" {
		t.Fatalf("expected section title to survive metadata conversion, got %#v", result)
	}
}
