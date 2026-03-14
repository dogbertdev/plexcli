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
