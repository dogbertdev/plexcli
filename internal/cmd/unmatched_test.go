package cmd

import (
	"bytes"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
)

func int64PtrUnmatched(i int64) *int64 {
	return &i
}

func TestUnmatchedCmd_findUnmatched(t *testing.T) {
	items := []*components.Metadata{
		{
			Title: "Matched (plex guid)",
			Type:  "movie",
			AdditionalProperties: map[string]any{
				"guid": "plex://movie/abc123",
			},
		},
		{
			Title: "Unmatched (local guid)",
			Type:  "movie",
			AdditionalProperties: map[string]any{
				"guid": "local://987",
			},
			RatingKey: "22",
		},
		{
			Title:     "Unmatched (no guid)",
			Type:      "show",
			RatingKey: "33",
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
			AdditionalProperties: map[string]any{
				"guid": "local://movie-1",
			},
		},
		{
			Title: "Show Unmatched",
			Type:  "show",
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

func TestIsMetadataUnmatched(t *testing.T) {
	tests := []struct {
		name string
		item *components.Metadata
		want bool
	}{
		{
			name: "guid local is unmatched",
			item: &components.Metadata{AdditionalProperties: map[string]any{"guid": "local://123"}},
			want: true,
		},
		{
			name: "guid plex is matched",
			item: &components.Metadata{AdditionalProperties: map[string]any{"guid": "plex://movie/123"}},
			want: false,
		},
		{
			name: "guid missing and tags missing is unmatched",
			item: &components.Metadata{},
			want: true,
		},
		{
			name: "guid missing and tag local is unmatched",
			item: &components.Metadata{GUID: []components.Tag{{Tag: "local://abc"}}},
			want: true,
		},
		{
			name: "guid missing and non-local tag is matched",
			item: &components.Metadata{GUID: []components.Tag{{Tag: "imdb://tt123"}}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isMetadataUnmatched(tt.item)
			if got != tt.want {
				t.Fatalf("isMetadataUnmatched()=%v want %v", got, tt.want)
			}
		})
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
			Year:  int64PtrUnmatched(2021),
			AdditionalProperties: map[string]any{
				"guid": "local://movie-year",
			},
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
