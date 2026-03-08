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

func TestParseKeyValueFlags(t *testing.T) {
	got, err := parseKeyValueFlags([]string{"title=Movie", "summary=Hello"})
	if err != nil {
		t.Fatalf("parseKeyValueFlags() error = %v", err)
	}
	if got["title"] != "Movie" || got["summary"] != "Hello" {
		t.Fatalf("unexpected parseKeyValueFlags() result: %#v", got)
	}
}

func TestGenericRowsForMediaContainerMetadata(t *testing.T) {
	header, rows := genericRows(map[string]any{
		"MediaContainer": map[string]any{
			"Metadata": []any{
				map[string]any{
					"ratingKey": "42",
					"title":     "Movie",
					"type":      "movie",
					"year":      float64(2024),
				},
			},
		},
	})

	if len(header) == 0 || len(rows) != 1 {
		t.Fatalf("unexpected genericRows() output: header=%v rows=%v", header, rows)
	}
	if rows[0][0] != "42" {
		t.Fatalf("expected rating key in first column, got %v", rows[0])
	}
}

func TestOutputBinaryResultJSON(t *testing.T) {
	var out bytes.Buffer
	result := &plexclient.LibraryBinaryResult{
		Action:     "GetItemArtwork",
		Target:     "library/metadata/1/poster/0",
		OutputPath: "/tmp/poster.jpg",
		Bytes:      128,
	}

	if err := outputBinaryResult(&out, "json", result); err != nil {
		t.Fatalf("outputBinaryResult() error = %v", err)
	}
	if !strings.Contains(out.String(), `"output_path": "/tmp/poster.jpg"`) {
		t.Fatalf("unexpected outputBinaryResult() JSON: %s", out.String())
	}
}

func TestRunLibraryMutationSummaryTable(t *testing.T) {
	var out bytes.Buffer
	ui := ui.New(ui.Options{Out: &out, Err: &out})
	err := runLibraryMutationSummary(ui, "table", plexclient.LibraryMutationSummary{
		Action:  "item-edit",
		Target:  "1,2",
		Outcome: "items updated",
	})
	if err != nil {
		t.Fatalf("runLibraryMutationSummary() error = %v", err)
	}
	if !strings.Contains(out.String(), "items updated") {
		t.Fatalf("expected summary output, got %s", out.String())
	}
}

func TestRequireConfirmed(t *testing.T) {
	if err := requireConfirmed(false, "delete"); err == nil {
		t.Fatalf("expected requireConfirmed() error")
	}
}

func TestRequireOutputPath(t *testing.T) {
	if err := requireOutputPath(""); err == nil {
		t.Fatalf("expected requireOutputPath() error")
	}
}

func TestSectionTypeIDForLibrary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><MediaContainer><Directory key="10" title="Anime" type="show" /></MediaContainer>`))
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	sectionTypeID, err := sectionTypeIDForLibrary(context.Background(), client, "10")
	if err != nil {
		t.Fatalf("sectionTypeIDForLibrary() error = %v", err)
	}
	if sectionTypeID != 2 {
		t.Fatalf("expected show type ID 2, got %d", sectionTypeID)
	}
}

func TestSectionTypeIDForLibraryMapsArtistSectionsToMusicType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><MediaContainer><Directory key="7" title="Audiobooks" type="artist" /></MediaContainer>`))
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	sectionTypeID, err := sectionTypeIDForLibrary(context.Background(), client, "7")
	if err != nil {
		t.Fatalf("sectionTypeIDForLibrary() error = %v", err)
	}
	if sectionTypeID != 8 {
		t.Fatalf("expected artist section to map to music type ID 8, got %d", sectionTypeID)
	}
}

func TestSectionTypeIDForLibraryFailsForUnknownSection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/library/sections" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><MediaContainer><Directory key="10" title="Anime" type="show" /></MediaContainer>`))
	}))
	defer server.Close()

	client, err := plexclient.NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	_, err = sectionTypeIDForLibrary(context.Background(), client, "99")
	if err == nil {
		t.Fatalf("expected sectionTypeIDForLibrary() error")
	}
	if !strings.Contains(err.Error(), "section not found") {
		t.Fatalf("unexpected sectionTypeIDForLibrary() error: %v", err)
	}
}
