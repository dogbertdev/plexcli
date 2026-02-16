package cmd

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/dogbertdev/plexcli/internal/plexclient"
)

func TestRecentlyWatchedCmd_processHistory(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	lastWeek := now.AddDate(0, 0, -7)

	history := []plexclient.HistoryItem{
		{Title: "Movie 1", Type: "movie", ViewedAt: now.Unix(), LibrarySectionID: strPtr("1")},
		{Title: "TV Show 1", Type: "episode", ViewedAt: yesterday.Unix(), LibrarySectionID: strPtr("2")},
		{Title: "Movie 2", Type: "movie", ViewedAt: lastWeek.Unix(), LibrarySectionID: strPtr("1")},
		{Title: "TV Show 2", Type: "episode", ViewedAt: now.Unix(), LibrarySectionID: strPtr("2")},
	}

	tests := []struct {
		name          string
		cmd           RecentlyWatchedCmd
		expectedCount int
		expectedTypes map[string]bool
	}{
		{
			name:          "filter by type movie",
			cmd:           RecentlyWatchedCmd{Type: "movie", Limit: 50},
			expectedCount: 2,
			expectedTypes: map[string]bool{"movie": true},
		},
		{
			name:          "filter by type tv",
			cmd:           RecentlyWatchedCmd{Type: "tv", Limit: 50},
			expectedCount: 2,
			expectedTypes: map[string]bool{"episode": true, "show": true, "season": true},
		},
		{
			name:          "all types",
			cmd:           RecentlyWatchedCmd{Type: "all", Limit: 50},
			expectedCount: 4,
			expectedTypes: map[string]bool{"movie": true, "episode": true},
		},
		{
			name:          "filter by days",
			cmd:           RecentlyWatchedCmd{Type: "all", Days: 3, Limit: 50},
			expectedCount: 3,
			expectedTypes: map[string]bool{"movie": true, "episode": true},
		},
		{
			name:          "limit results",
			cmd:           RecentlyWatchedCmd{Type: "all", Limit: 2},
			expectedCount: 2,
			expectedTypes: map[string]bool{"movie": true, "episode": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			items := tt.cmd.processHistory(ctx, history, nil)

			if len(items) != tt.expectedCount {
				t.Errorf("expected %d items, got %d", tt.expectedCount, len(items))
			}

			for _, item := range items {
				if tt.expectedTypes != nil {
					if !tt.expectedTypes[item.Type] {
						t.Errorf("unexpected type: %s", item.Type)
					}
				}
			}
		})
	}
}

func TestRecentlyWatchedCmd_output(t *testing.T) {
	cmd := &RecentlyWatchedCmd{Output: "table"}
	items := []RecentlyWatchedItem{
		{Title: "Test Movie", Type: "movie", Library: "Movies", WatchedAt: time.Now()},
	}

	var buf bytes.Buffer
	err := cmd.output(&buf, items)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if buf.Len() == 0 {
		t.Error("expected output, got empty buffer")
	}
}

func TestRecentlyWatchedCmd_outputJSON(t *testing.T) {
	cmd := &RecentlyWatchedCmd{Output: "json"}
	items := []RecentlyWatchedItem{
		{Title: "Test Movie", Type: "movie", Library: "Movies", WatchedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)},
	}

	var buf bytes.Buffer
	err := cmd.output(&buf, items)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("expected JSON output, got empty buffer")
	}

	if !bytes.Contains(buf.Bytes(), []byte("Test Movie")) {
		t.Error("expected output to contain movie title")
	}
}

func TestRecentlyWatchedCmd_outputTSV(t *testing.T) {
	cmd := &RecentlyWatchedCmd{Output: "tsv"}
	items := []RecentlyWatchedItem{
		{Title: "Test Movie", Type: "movie", Library: "Movies", WatchedAt: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)},
	}

	var buf bytes.Buffer
	err := cmd.output(&buf, items)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	output := buf.String()
	if output == "" {
		t.Error("expected TSV output, got empty buffer")
	}

	if !bytes.Contains(buf.Bytes(), []byte("Test Movie")) {
		t.Error("expected output to contain movie title")
	}
}

func strPtr(s string) *string {
	return &s
}
