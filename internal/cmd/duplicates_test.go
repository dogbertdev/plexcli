package cmd

import (
	"bytes"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
)

func TestDuplicatesCmd_generateKey(t *testing.T) {
	cmd := &DuplicatesCmd{}

	tests := []struct {
		name     string
		item     *components.Metadata
		expected string
	}{
		{
			name: "movie with year",
			item: &components.Metadata{
				Title: "The Matrix",
				Type:  "movie",
				Year:  intPtr(1999),
			},
			expected: "movie:the matrix:1999",
		},
		{
			name: "movie without year",
			item: &components.Metadata{
				Title: "Unknown Movie",
				Type:  "movie",
			},
			expected: "movie:unknown movie:0",
		},
		{
			name: "episode with show",
			item: &components.Metadata{
				Title:            "Pilot",
				Type:             "episode",
				GrandparentTitle: stringPtr("Breaking Bad"),
			},
			expected: "episode:breaking bad:pilot",
		},
		{
			name: "episode without show",
			item: &components.Metadata{
				Title: "Episode Name",
				Type:  "episode",
			},
			expected: "episode::episode name",
		},
		{
			name: "other type",
			item: &components.Metadata{
				Title: "Some Track",
				Type:  "track",
			},
			expected: "track:some track",
		},
		{
			name:     "nil item",
			item:     nil,
			expected: "",
		},
		{
			name: "empty title",
			item: &components.Metadata{
				Title: "",
				Type:  "movie",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cmd.generateKey(tt.item)
			if result != tt.expected {
				t.Errorf("generateKey() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDuplicatesCmd_findDuplicates(t *testing.T) {
	cmd := &DuplicatesCmd{
		Type:     "all",
		MinCount: 2,
	}

	tests := []struct {
		name     string
		items    []*components.Metadata
		expected int
	}{
		{
			name:     "no items",
			items:    []*components.Metadata{},
			expected: 0,
		},
		{
			name: "no duplicates",
			items: []*components.Metadata{
				{Title: "Movie 1", Type: "movie", Year: intPtr(2020), RatingKey: stringPtr("1")},
				{Title: "Movie 2", Type: "movie", Year: intPtr(2021), RatingKey: stringPtr("2")},
			},
			expected: 0,
		},
		{
			name: "one duplicate pair",
			items: []*components.Metadata{
				{Title: "The Matrix", Type: "movie", Year: intPtr(1999), RatingKey: stringPtr("1")},
				{Title: "The Matrix", Type: "movie", Year: intPtr(1999), RatingKey: stringPtr("2")},
			},
			expected: 1,
		},
		{
			name: "multiple duplicates",
			items: []*components.Metadata{
				{Title: "Movie A", Type: "movie", Year: intPtr(2020), RatingKey: stringPtr("1")},
				{Title: "Movie A", Type: "movie", Year: intPtr(2020), RatingKey: stringPtr("2")},
				{Title: "Movie B", Type: "movie", Year: intPtr(2021), RatingKey: stringPtr("3")},
				{Title: "Movie B", Type: "movie", Year: intPtr(2021), RatingKey: stringPtr("4")},
				{Title: "Movie B", Type: "movie", Year: intPtr(2021), RatingKey: stringPtr("5")},
			},
			expected: 2,
		},
		{
			name: "filter by type movie",
			items: []*components.Metadata{
				{Title: "The Matrix", Type: "movie", Year: intPtr(1999), RatingKey: stringPtr("1")},
				{Title: "The Matrix", Type: "movie", Year: intPtr(1999), RatingKey: stringPtr("2")},
				{Title: "Pilot", Type: "episode", GrandparentTitle: stringPtr("Show"), RatingKey: stringPtr("3")},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cmd.findDuplicates(tt.items)
			if len(result) != tt.expected {
				t.Errorf("findDuplicates() returned %d groups, want %d", len(result), tt.expected)
			}
		})
	}
}

func TestDuplicatesCmd_findDuplicates_withTypeFilter(t *testing.T) {
	cmd := &DuplicatesCmd{
		Type:     "movie",
		MinCount: 2,
	}

	items := []*components.Metadata{
		{Title: "The Matrix", Type: "movie", Year: intPtr(1999), RatingKey: stringPtr("1")},
		{Title: "The Matrix", Type: "movie", Year: intPtr(1999), RatingKey: stringPtr("2")},
		{Title: "Pilot", Type: "episode", GrandparentTitle: stringPtr("Show"), RatingKey: stringPtr("3")},
		{Title: "Pilot", Type: "episode", GrandparentTitle: stringPtr("Show"), RatingKey: stringPtr("4")},
	}

	result := cmd.findDuplicates(items)

	if len(result) != 1 {
		t.Errorf("Expected 1 duplicate group for movies, got %d", len(result))
	}

	if len(result) > 0 && result[0].Type != "movie" {
		t.Errorf("Expected movie type, got %s", result[0].Type)
	}
}

func TestDuplicatesCmd_outputResults(t *testing.T) {
	cmd := &DuplicatesCmd{
		Output: "table",
	}

	groups := []DuplicateGroup{
		{
			Key:        "movie:the matrix:1999",
			Title:      "The Matrix",
			Year:       1999,
			Type:       "movie",
			Count:      2,
			RatingKeys: []string{"1", "2"},
		},
		{
			Key:        "episode:breaking bad:pilot",
			Title:      "Pilot",
			Show:       "Breaking Bad",
			Type:       "episode",
			Count:      3,
			RatingKeys: []string{"10", "11", "12"},
		},
	}

	var buf bytes.Buffer

	err := cmd.outputResults(&buf, groups)
	if err != nil {
		t.Errorf("outputResults() error = %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("outputResults() produced no output")
	}
}

func TestDuplicatesCmd_outputResults_JSON(t *testing.T) {
	cmd := &DuplicatesCmd{
		Output: "json",
	}

	groups := []DuplicateGroup{
		{
			Key:        "movie:the matrix:1999",
			Title:      "The Matrix",
			Year:       1999,
			Type:       "movie",
			Count:      2,
			RatingKeys: []string{"1", "2"},
		},
	}

	var buf bytes.Buffer

	err := cmd.outputResults(&buf, groups)
	if err != nil {
		t.Errorf("outputResults() error = %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("outputResults() produced no output")
	}

	if !bytes.Contains(buf.Bytes(), []byte("The Matrix")) {
		t.Error("JSON output should contain movie title")
	}
}

func TestDuplicatesCmd_outputResults_TSV(t *testing.T) {
	cmd := &DuplicatesCmd{
		Output: "tsv",
	}

	groups := []DuplicateGroup{
		{
			Key:        "movie:the matrix:1999",
			Title:      "The Matrix",
			Year:       1999,
			Type:       "movie",
			Count:      2,
			RatingKeys: []string{"1", "2"},
		},
	}

	var buf bytes.Buffer

	err := cmd.outputResults(&buf, groups)
	if err != nil {
		t.Errorf("outputResults() error = %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("outputResults() produced no output")
	}

	if !bytes.Contains(buf.Bytes(), []byte("\t")) {
		t.Error("TSV output should contain tabs")
	}
}

func TestDuplicateGroup_structure(t *testing.T) {
	group := DuplicateGroup{
		Key:        "test-key",
		Title:      "Test Title",
		Year:       2020,
		Show:       "Test Show",
		Type:       "movie",
		Count:      2,
		RatingKeys: []string{"1", "2"},
	}

	if group.Key != "test-key" {
		t.Error("Key mismatch")
	}
	if group.Title != "Test Title" {
		t.Error("Title mismatch")
	}
	if group.Year != 2020 {
		t.Error("Year mismatch")
	}
	if group.Show != "Test Show" {
		t.Error("Show mismatch")
	}
	if group.Type != "movie" {
		t.Error("Type mismatch")
	}
	if group.Count != 2 {
		t.Error("Count mismatch")
	}
	if len(group.RatingKeys) != 2 {
		t.Error("RatingKeys length mismatch")
	}
}

func intPtr(i int) *int {
	return &i
}

func stringPtr(s string) *string {
	return &s
}
