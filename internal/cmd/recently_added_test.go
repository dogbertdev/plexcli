package cmd

import (
	"testing"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
)

func TestRecentlyAddedCmd_processItems_SortsByMostRecentAndLimits(t *testing.T) {
	now := time.Now()
	oldest := now.Add(-72 * time.Hour).Unix()
	middle := now.Add(-48 * time.Hour).Unix()
	newest := now.Add(-24 * time.Hour).Unix()

	cmd := &RecentlyAddedCmd{
		Limit: 2,
		Days:  30,
		Type:  "all",
	}

	items := []itemWithSection{
		{
			item: &components.Metadata{Title: "Oldest", Type: "movie", AddedAt: oldest},
		},
		{
			item: &components.Metadata{Title: "Newest", Type: "movie", AddedAt: newest},
		},
		{
			item: &components.Metadata{Title: "Middle", Type: "movie", AddedAt: middle},
		},
	}

	results := cmd.processItems(items)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Title != "Newest" {
		t.Fatalf("expected first item to be Newest, got %q", results[0].Title)
	}
	if results[1].Title != "Middle" {
		t.Fatalf("expected second item to be Middle, got %q", results[1].Title)
	}
}
