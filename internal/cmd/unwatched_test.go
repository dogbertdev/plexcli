package cmd

import (
	"bytes"
	"testing"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
)

func intPtrUnwatched(i int) *int {
	return &i
}

func TestUnwatchedCmd_filterUnwatched(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		items    []*components.Metadata
		expected int
	}{
		{
			name: "all unwatched items",
			items: []*components.Metadata{
				{Title: "Movie 1", Type: "movie", ViewCount: nil, AddedAt: now.Unix()},
				{Title: "Movie 2", Type: "movie", ViewCount: intPtrUnwatched(0), AddedAt: now.Unix()},
			},
			expected: 2,
		},
		{
			name: "mixed watched and unwatched",
			items: []*components.Metadata{
				{Title: "Watched Movie", Type: "movie", ViewCount: intPtrUnwatched(1), AddedAt: now.Unix()},
				{Title: "Unwatched Movie", Type: "movie", ViewCount: nil, AddedAt: now.Unix()},
				{Title: "Partially Watched", Type: "movie", ViewCount: intPtrUnwatched(2), AddedAt: now.Unix()},
			},
			expected: 1,
		},
		{
			name:     "empty list",
			items:    []*components.Metadata{},
			expected: 0,
		},
		{
			name: "nil items are skipped",
			items: []*components.Metadata{
				nil,
				{Title: "Valid", Type: "movie", ViewCount: nil, AddedAt: now.Unix()},
				nil,
			},
			expected: 1,
		},
	}

	cmd := &UnwatchedCmd{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cmd.filterUnwatched(tt.items)
			if len(result) != tt.expected {
				t.Errorf("expected %d items, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestUnwatchedCmd_filterByType(t *testing.T) {
	now := time.Now()

	items := []*components.Metadata{
		{Title: "Movie 1", Type: "movie", ViewCount: nil, AddedAt: now.Unix()},
		{Title: "TV Show", Type: "show", ViewCount: nil, AddedAt: now.Unix()},
		{Title: "Season 1", Type: "season", ViewCount: nil, AddedAt: now.Unix()},
		{Title: "Episode 1", Type: "episode", ViewCount: nil, AddedAt: now.Unix()},
	}

	tests := []struct {
		name     string
		cmd      UnwatchedCmd
		expected int
		types    map[string]bool
	}{
		{
			name:     "all types",
			cmd:      UnwatchedCmd{Type: "all"},
			expected: 4,
			types:    map[string]bool{"movie": true, "show": true, "season": true, "episode": true},
		},
		{
			name:     "movies only",
			cmd:      UnwatchedCmd{Type: "movie"},
			expected: 1,
			types:    map[string]bool{"movie": true},
		},
		{
			name:     "episode only",
			cmd:      UnwatchedCmd{Type: "episode"},
			expected: 2,
			types:    map[string]bool{"show": true, "episode": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cmd.filterByType(items)
			if len(result) != tt.expected {
				t.Errorf("expected %d items, got %d", tt.expected, len(result))
			}
			for _, item := range result {
				if !tt.types[anyToString(item.Type)] {
					t.Errorf("unexpected type: %v", item.Type)
				}
			}
		})
	}
}

func TestUnwatchedCmd_sortByAddedDate(t *testing.T) {
	now := time.Now()

	items := []*components.Metadata{
		{Title: "Old Movie", Type: "movie", ViewCount: nil, AddedAt: now.AddDate(0, 0, -7).Unix()},
		{Title: "New Movie", Type: "movie", ViewCount: nil, AddedAt: now.Unix()},
		{Title: "Medium Movie", Type: "movie", ViewCount: nil, AddedAt: now.AddDate(0, 0, -3).Unix()},
	}

	cmd := &UnwatchedCmd{}
	result := cmd.sortByAddedDate(items)

	if len(result) != 3 {
		t.Fatalf("expected 3 items, got %d", len(result))
	}

	if anyToString(result[0].Title) != "New Movie" {
		t.Errorf("expected first item to be 'New Movie', got '%v'", result[0].Title)
	}
	if anyToString(result[1].Title) != "Medium Movie" {
		t.Errorf("expected second item to be 'Medium Movie', got '%v'", result[1].Title)
	}
	if anyToString(result[2].Title) != "Old Movie" {
		t.Errorf("expected third item to be 'Old Movie', got '%v'", result[2].Title)
	}
}

func TestUnwatchedCmd_toOutputItems(t *testing.T) {
	now := time.Now()

	items := []*components.Metadata{
		{
			Title:   "Test Movie",
			Type:    "movie",
			Year:    intPtrUnwatched(2023),
			AddedAt: now.Unix(),
		},
		{
			Title:   "No Year Movie",
			Type:    "movie",
			Year:    nil,
			AddedAt: 0,
		},
	}

	cmd := &UnwatchedCmd{}
	result := cmd.toOutputItems(items)

	if len(result) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result))
	}

	if result[0].Title != "Test Movie" {
		t.Errorf("expected title 'Test Movie', got '%s'", result[0].Title)
	}
	if result[0].Year != 2023 {
		t.Errorf("expected year 2023, got %d", result[0].Year)
	}
	if result[0].Type != "movie" {
		t.Errorf("expected type 'movie', got '%s'", result[0].Type)
	}
	if result[0].AddedAt == "" {
		t.Error("expected AddedAt to be set")
	}

	if result[1].Year != 0 {
		t.Errorf("expected year 0 for nil year, got %d", result[1].Year)
	}
	if result[1].AddedAt != "" {
		t.Error("expected empty AddedAt for zero timestamp")
	}
}

func TestUnwatchedCmd_output(t *testing.T) {
	items := []UnwatchedItem{
		{Title: "Test Movie", Year: 2023, Type: "movie", AddedAt: "2024-01-15"},
		{Title: "Test Show", Year: 0, Type: "episode", AddedAt: "2024-01-10"},
	}

	tests := []struct {
		name   string
		output string
	}{
		{
			name:   "table format",
			output: "table",
		},
		{
			name:   "json format",
			output: "json",
		},
		{
			name:   "tsv format",
			output: "tsv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &UnwatchedCmd{Output: tt.output}
			var buf bytes.Buffer
			err := cmd.output(&buf, items)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if buf.Len() == 0 {
				t.Error("expected output, got empty buffer")
			}
		})
	}
}

func TestUnwatchedCmd_outputJSON(t *testing.T) {
	items := []UnwatchedItem{
		{Title: "Test Movie", Year: 2023, Type: "movie", AddedAt: "2024-01-15"},
	}

	cmd := &UnwatchedCmd{Output: "json"}
	var buf bytes.Buffer
	err := cmd.output(&buf, items)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("Test Movie")) {
		t.Error("expected output to contain movie title")
	}
	if !bytes.Contains([]byte(output), []byte("2023")) {
		t.Error("expected output to contain year")
	}
}

func TestUnwatchedCmd_outputTSV(t *testing.T) {
	items := []UnwatchedItem{
		{Title: "Test Movie", Year: 2023, Type: "movie", AddedAt: "2024-01-15"},
	}

	cmd := &UnwatchedCmd{Output: "tsv"}
	var buf bytes.Buffer
	err := cmd.output(&buf, items)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("Test Movie")) {
		t.Error("expected output to contain movie title")
	}
	if !bytes.Contains([]byte(output), []byte("movie")) {
		t.Error("expected output to contain type")
	}
}
