package cmd

import (
	"bytes"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
)

func intPtrUnmatched(i int) *int {
	return &i
}

func unmatchedStrPtr(s string) *string {
	return &s
}

func TestUnmatchedCmd_findUnmatched(t *testing.T) {
	items := []*components.Metadata{
		{
			Title: "Matched (plex guid)",
			Type:  "movie",
			GUID:  unmatchedStrPtr("plex://movie/abc123"),
			AdditionalProperties: map[string]any{
				"guid": "plex://movie/abc123",
			},
		},
		{
			Title: "Unmatched (local guid)",
			Type:  "movie",
			GUID:  unmatchedStrPtr("local://987"),
			AdditionalProperties: map[string]any{
				"guid": "local://987",
			},
			RatingKey: unmatchedStrPtr("22"),
		},
		{
			Title:     "Unmatched (no guid)",
			Type:      "show",
			RatingKey: unmatchedStrPtr("33"),
		},
		nil,
	}

	cmd := &UnmatchedCmd{Type: "all"}
	got := cmd.findUnmatched(items)
	if len(got) != 2 {
		t.Fatalf("expected 2 unmatched items, got %d", len(got))
	}

	if got[0].Title != "Unmatched (local guid)" {
		t.Fatalf("expected first unmatched title to be local guid item, got %q", got[0].Title)
	}
	if got[1].Title != "Unmatched (no guid)" {
		t.Fatalf("expected second unmatched title to be no guid item, got %q", got[1].Title)
	}
}

func TestUnmatchedCmd_findUnmatchedTypeFilter(t *testing.T) {
	items := []*components.Metadata{
		{
			Title: "Movie Unmatched",
			Type:  "movie",
			GUID:  unmatchedStrPtr("local://movie-1"),
			AdditionalProperties: map[string]any{
				"guid": "local://movie-1",
			},
		},
		{
			Title: "Show Unmatched",
			Type:  "show",
			GUID:  unmatchedStrPtr("local://show-1"),
			AdditionalProperties: map[string]any{
				"guid": "local://show-1",
			},
		},
	}

	cmd := &UnmatchedCmd{Type: "movie"}
	got := cmd.findUnmatched(items)
	if len(got) != 1 {
		t.Fatalf("expected 1 unmatched movie, got %d", len(got))
	}
	if got[0].Type != "movie" {
		t.Fatalf("expected movie type, got %q", got[0].Type)
	}
}

func TestUnmatchedCmd_findUnmatchedEpisodeFilterMapsToShow(t *testing.T) {
	items := []*components.Metadata{
		{
			Title: "Show Unmatched",
			Type:  "show",
			GUID:  unmatchedStrPtr("local://show-1"),
			AdditionalProperties: map[string]any{
				"guid": "local://show-1",
			},
		},
		{
			Title: "Movie Unmatched",
			Type:  "movie",
			GUID:  unmatchedStrPtr("local://movie-1"),
			AdditionalProperties: map[string]any{
				"guid": "local://movie-1",
			},
		},
	}

	cmd := &UnmatchedCmd{Type: "episode"}
	got := cmd.findUnmatched(items)
	if len(got) != 1 {
		t.Fatalf("expected 1 unmatched item for episode filter, got %d", len(got))
	}
	if got[0].Type != "show" {
		t.Fatalf("expected show type for episode filter, got %q", got[0].Type)
	}
}

func TestIsMetadataUnmatched(t *testing.T) {
	tests := []struct {
		name string
		item *components.Metadata
		want bool
	}{
		{
			name: "guid local is unmatched",
			item: &components.Metadata{GUID: unmatchedStrPtr("local://123"), AdditionalProperties: map[string]any{"guid": "local://123"}},
			want: true,
		},
		{
			name: "guid plex is matched",
			item: &components.Metadata{GUID: unmatchedStrPtr("plex://movie/123"), AdditionalProperties: map[string]any{"guid": "plex://movie/123"}},
			want: false,
		},
		{
			name: "no guid is unmatched",
			item: &components.Metadata{},
			want: true,
		},
		{
			name: "all guids local is unmatched",
			item: &components.Metadata{Guids: []components.Guids{{ID: "local://1"}, {ID: "local://2"}}},
			want: true,
		},
		{
			name: "non-local guid in guid list is matched",
			item: &components.Metadata{Guids: []components.Guids{{ID: "local://1"}, {ID: "tmdb://2"}}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMetadataUnmatched(tt.item); got != tt.want {
				t.Fatalf("isMetadataUnmatched() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMetadataGUID(t *testing.T) {
	if got := metadataGUID(&components.Metadata{GUID: unmatchedStrPtr("plex://movie/42")}); got != "plex://movie/42" {
		t.Fatalf("expected direct GUID, got %q", got)
	}

	if got := metadataGUID(&components.Metadata{AdditionalProperties: map[string]any{"guid": "local://99"}}); got != "local://99" {
		t.Fatalf("expected additionalProperties guid, got %q", got)
	}
}

func TestUnmatchedCmd_outputResults(t *testing.T) {
	results := []UnmatchedInfo{
		{Title: "Movie A", Year: 2024, Type: "movie", RatingKey: "1", GUID: "local://1"},
		{Title: "Show B", Type: "show", RatingKey: "2"},
	}

	formats := []string{"table", "json", "tsv"}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			cmd := &UnmatchedCmd{Output: format}
			var buf bytes.Buffer
			if err := cmd.outputResults(&buf, results); err != nil {
				t.Fatalf("outputResults() error = %v", err)
			}
			if buf.Len() == 0 {
				t.Fatal("expected output, got empty buffer")
			}
		})
	}
}

func TestUnmatchedCmd_findUnmatchedIncludesYear(t *testing.T) {
	items := []*components.Metadata{
		{
			Title: "Movie Year",
			Type:  "movie",
			Year:  intPtrUnmatched(2021),
			AdditionalProperties: map[string]any{
				"guid": "local://movie-year",
			},
			RatingKey: unmatchedStrPtr("44"),
		},
	}

	cmd := &UnmatchedCmd{}
	got := cmd.findUnmatched(items)
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
	if got[0].Year != 2021 {
		t.Fatalf("expected year 2021, got %d", got[0].Year)
	}
}
