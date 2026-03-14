package cmd

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

func testSearchMatchesClient(t *testing.T) (string, *plexclient.Client) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/metadata/48458/matches" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("manual"); got != "1" {
			t.Fatalf("manual query = %q, want 1", got)
		}
		if got := r.URL.Query().Get("X-Plex-Token"); got != "test-token" {
			t.Fatalf("token query = %q, want test-token", got)
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<MediaContainer size="2">
<SearchResult guid="plex://movie/5d77682454c0f0001f301a42" name="Akira" year="1988" summary="Classic cyberpunk anime."></SearchResult>
<SearchResult guid="plex://movie/5f40bf27ce2564003fa64a60" name="Akira Sound Clip" summary="Behind the scenes."></SearchResult>
</MediaContainer>`))
	}))

	t.Cleanup(server.Close)

	client, err := plexclient.NewClient(server.URL, "test-token", plexclient.WithMaxRetries(0))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	return server.URL, client
}

func TestCompactDiscoveryItems(t *testing.T) {
	year := 1979
	sectionID := 1
	sectionTitle := "Movies"
	guid := "tmdb://1398"

	items := compactDiscoveryItems([]plexclient.SearchResult{
		{
			RatingKey:           "101",
			Title:               "Stalker",
			Type:                "movie",
			Year:                &year,
			LibrarySectionID:    &sectionID,
			LibrarySectionTitle: &sectionTitle,
			GUID:                &guid,
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].SectionID != "1" || items[0].SectionTitle != "Movies" {
		t.Fatalf("unexpected section info: %#v", items[0])
	}
}

func TestOutputCompactDiscoveryItemsTSV(t *testing.T) {
	var buf bytes.Buffer
	if err := outputCompactDiscoveryItems(&buf, "tsv", []DiscoveryCompactItem{
		{RatingKey: "101", Title: "Stalker", Type: "movie", Year: 1979, SectionID: "1", SectionTitle: "Movies", GUID: "tmdb://1398"},
	}); err != nil {
		t.Fatalf("outputCompactDiscoveryItems() error = %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "101\tStalker\tmovie\t1979\t1\tMovies\ttmdb://1398" {
		t.Fatalf("unexpected TSV output: %q", got)
	}
}

func TestCompactDiscoveryMatchesTSV(t *testing.T) {
	var buf bytes.Buffer
	err := outputDiscoveryMatches(&buf, "tsv", true, []plexclient.MatchResult{
		{GUID: "plex://movie/123", Name: "Stalker", Year: 1979, Summary: "ignored in compact"},
	})
	if err != nil {
		t.Fatalf("outputDiscoveryMatches() error = %v", err)
	}

	got := strings.TrimSpace(buf.String())
	if got != "plex://movie/123\tStalker\t1979" {
		t.Fatalf("unexpected compact matches TSV output: %q", got)
	}
}

func TestLibraryDiscoverMatchesRun_UsesSearchMatchesEndpoint(t *testing.T) {
	original := newLibraryDiscoverClientContext
	defer func() {
		newLibraryDiscoverClientContext = original
	}()

	serverURL, client := testSearchMatchesClient(t)
	newLibraryDiscoverClientContext = func(_ *config.Config) (*ClientContext, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		return &ClientContext{
			Client:  client,
			Ctx:     ctx,
			Cancel:  cancel,
			Timeout: 5 * time.Second,
		}, nil
	}

	var out bytes.Buffer
	u := ui.New(ui.Options{Out: &out, Err: &out})
	cmd := LibraryDiscoverMatchesCmd{
		RatingKey: "48458",
		Compact:   true,
		Output:    "tsv",
	}

	err := cmd.Run(&kong.Context{}, u, &config.Config{ServerURL: serverURL, Token: "test-token"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := strings.Join([]string{
		"plex://movie/5d77682454c0f0001f301a42\tAkira\t1988",
		"plex://movie/5f40bf27ce2564003fa64a60\tAkira Sound Clip",
	}, "\n")
	if got != want {
		t.Fatalf("unexpected compact output: %q", got)
	}
}
