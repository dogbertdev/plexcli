package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dogbertdev/plexcli/internal/plexclient"
)

func TestValidateSearchCommandOptions_RejectsConflictingFlags(t *testing.T) {
	err := validateSearchCommandOptions(true, true)
	if err == nil {
		t.Fatal("expected conflicting flags to fail")
	}
	if !strings.Contains(err.Error(), "--first and --fail-ambiguous") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateResolvedSearchResults_FailAmbiguous(t *testing.T) {
	results := []plexclient.SearchResult{
		{RatingKey: "1", Title: "Ghost in the Shell"},
		{RatingKey: "2", Title: "Ghost in the Shell 2"},
	}

	err := validateResolvedSearchResults("Ghost", results, true)
	if err == nil {
		t.Fatal("expected ambiguous results to fail")
	}
	if !strings.Contains(err.Error(), `search returned 2 results for "Ghost"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHandleResolvedSearchResults_FailAmbiguousPrintsCandidates(t *testing.T) {
	var errBuf bytes.Buffer
	results := []plexclient.SearchResult{
		{RatingKey: "1", Title: "Ghost in the Shell", Type: "movie"},
		{RatingKey: "2", Title: "Ghost in the Shell 2", Type: "movie"},
	}

	err := handleResolvedSearchResults(&errBuf, "tsv", "Ghost", results, true)
	if err == nil {
		t.Fatal("expected ambiguous results to fail")
	}
	if !strings.Contains(errBuf.String(), "1\tGhost in the Shell\tmovie") {
		t.Fatalf("expected first candidate in stderr, got %q", errBuf.String())
	}
	if !strings.Contains(errBuf.String(), "2\tGhost in the Shell 2\tmovie") {
		t.Fatalf("expected second candidate in stderr, got %q", errBuf.String())
	}
}

func TestHandleSearchResolveError_NoResultsPrintsMessage(t *testing.T) {
	var errBuf bytes.Buffer

	err := handleSearchResolveError(&errBuf, &NoSearchResultsError{Query: "missing"})
	if err != nil {
		t.Fatalf("expected no-results error to be handled, got %v", err)
	}
	if strings.TrimSpace(errBuf.String()) != "No results found" {
		t.Fatalf("unexpected no-results output: %q", errBuf.String())
	}
}
