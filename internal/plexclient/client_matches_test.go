package plexclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func loadSearchMatchesFixture(t *testing.T) []byte {
	t.Helper()

	path := filepath.Join("testdata", "library_matches_manual.xml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}

	return raw
}

func TestSearchMatches_ParsesLiveFixtureResponse(t *testing.T) {
	fixture := loadSearchMatchesFixture(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/48458/matches" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("manual"); got != "1" {
			t.Fatalf("manual query = %q, want 1", got)
		}
		if got := r.URL.Query().Get("title"); got != "Akira" {
			t.Fatalf("title query = %q, want Akira", got)
		}
		if got := r.URL.Query().Get("year"); got != "1988" {
			t.Fatalf("year query = %q, want 1988", got)
		}
		if got := r.URL.Query().Get("X-Plex-Token"); got != "test-token" {
			t.Fatalf("token query = %q, want test-token", got)
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write(fixture)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	results, err := client.SearchMatches(context.Background(), "48458", "Akira", 1988)
	if err != nil {
		t.Fatalf("SearchMatches() error = %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %#v", results)
	}

	if results[0].GUID != "plex://movie/5d77682454c0f0001f301a42" {
		t.Fatalf("unexpected first GUID: %#v", results[0])
	}
	if results[0].Name != "Akira" || results[0].Year != 1988 {
		t.Fatalf("unexpected first result metadata: %#v", results[0])
	}
	if results[3].Year != 0 {
		t.Fatalf("expected missing year to remain zero-valued, got %#v", results[3])
	}
	if results[3].Summary == "" {
		t.Fatalf("expected summary to survive on partial result, got %#v", results[3])
	}
	if results[4].Name != "Akira Kurosawa Making 'Rhapsody in August'" {
		t.Fatalf("expected XML entities to decode in name, got %#v", results[4])
	}
	if results[4].Summary != "" {
		t.Fatalf("expected missing summary to remain empty, got %#v", results[4])
	}
}

func TestSearchMatches_EscapesTitleQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("title"); got != "Ghost in the Shell: Stand Alone Complex" {
			t.Fatalf("title query = %q", got)
		}
		if got := r.URL.Query().Get("manual"); got != "1" {
			t.Fatalf("manual query = %q, want 1", got)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><MediaContainer size="0"></MediaContainer>`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-token", WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = client.SearchMatches(context.Background(), "123", "Ghost in the Shell: Stand Alone Complex", 0)
	if err != nil {
		t.Fatalf("SearchMatches() error = %v", err)
	}
}
