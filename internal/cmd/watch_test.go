package cmd

import (
	"testing"
	"time"

	"github.com/dogbertdev/plexcli/internal/plexclient"
)

func TestBuildAccountNameByIDAndDisplayName(t *testing.T) {
	accounts := []plexclient.Account{
		{ID: 1, Name: "Paul"},
		{ID: 2, Name: "Alex"},
	}

	byID := buildAccountNameByID(accounts)
	if byID[1] != "Paul" || byID[2] != "Alex" {
		t.Fatalf("unexpected map contents: %v", byID)
	}

	if got := accountDisplayName(byID, 1); got != "Paul" {
		t.Fatalf("expected Paul, got %q", got)
	}
	if got := accountDisplayName(byID, 99); got != "Account 99" {
		t.Fatalf("expected fallback Account 99, got %q", got)
	}
}

func TestWatchHistoryMatchesFilters(t *testing.T) {
	now := time.Now()
	cutoff := now.Add(-24 * time.Hour)

	entryRecentMovie := plexclient.HistoryEntry{Type: "movie", ViewedAt: now}
	entryOldMovie := plexclient.HistoryEntry{Type: "movie", ViewedAt: now.Add(-48 * time.Hour)}
	entryRecentEpisode := plexclient.HistoryEntry{Type: "episode", ViewedAt: now}

	if !watchHistoryMatchesFilters(entryRecentMovie, cutoff, "all") {
		t.Fatal("expected recent movie to match all")
	}
	if watchHistoryMatchesFilters(entryOldMovie, cutoff, "all") {
		t.Fatal("expected old movie not to match cutoff")
	}
	if watchHistoryMatchesFilters(entryRecentEpisode, cutoff, "movie") {
		t.Fatal("expected episode not to match movie filter")
	}
	if !watchHistoryMatchesFilters(entryRecentEpisode, cutoff, "episode") {
		t.Fatal("expected episode to match episode filter")
	}
}
