package cmd

import (
	"testing"

	"github.com/dogbertdev/plexcli/internal/plexclient"
)

func TestTargetLibrarySections_All(t *testing.T) {
	sections := []plexclient.Library{
		{ID: "1"},
		{ID: "2"},
	}

	got, err := targetLibrarySections(sections, "")
	if err != nil {
		t.Fatalf("targetLibrarySections() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(got))
	}

	got, err = targetLibrarySections(sections, "all")
	if err != nil {
		t.Fatalf("targetLibrarySections() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(got))
	}
}

func TestTargetLibrarySections_SpecificID(t *testing.T) {
	sections := []plexclient.Library{
		{ID: "1"},
		{ID: "2"},
	}

	got, err := targetLibrarySections(sections, "2")
	if err != nil {
		t.Fatalf("targetLibrarySections() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 section, got %d", len(got))
	}
	if got[0].ID != "2" {
		t.Fatalf("expected section 2, got %s", got[0].ID)
	}
}

func TestTargetLibrarySections_NotFound(t *testing.T) {
	sections := []plexclient.Library{
		{ID: "1"},
	}

	if _, err := targetLibrarySections(sections, "9"); err == nil {
		t.Fatalf("expected not found error")
	}
}
