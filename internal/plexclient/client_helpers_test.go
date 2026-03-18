package plexclient

import (
	"net/url"
	"strings"
	"testing"
)

func TestEncodeMetadataIDs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "single id", input: "123", want: "123"},
		{name: "multiple ids", input: "1,2,3", want: "1,2,3"},
		{name: "trimmed ids", input: " 1, 2 ,3 ", want: "1,2,3"},
		{name: "escaped segment", input: "abc/def,2", want: "abc%2Fdef,2"},
		{name: "empty input", input: "", wantErr: true},
		{name: "empty segment", input: "1,,2", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeMetadataIDs(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("encodeMetadataIDs(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseOptionalFloat(t *testing.T) {
	got, err := parseOptionalFloat("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for empty input")
	}

	got, err = parseOptionalFloat("12.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || *got != 12.5 {
		t.Fatalf("expected 12.5, got %#v", got)
	}

	if _, err = parseOptionalFloat("abc"); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestParseOptionalInt64(t *testing.T) {
	got, err := parseOptionalInt64("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for empty input")
	}

	got, err = parseOptionalInt64("42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || *got != 42 {
		t.Fatalf("expected 42, got %#v", got)
	}

	if _, err = parseOptionalInt64("abc"); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestBuildSmartPlaylistURI(t *testing.T) {
	uri := buildSmartPlaylistURI("1", "video", SmartPlaylistFilters{
		Directors:   []string{"11", "12"},
		Genres:      []string{"21", "22"},
		Countries:   []string{"31"},
		Collections: []string{"41", "42"},
		Studios:     []string{"51", "52"},
		YearFrom:    1990,
		YearTo:      2000,
		Unwatched:   true,
	})

	if !strings.HasPrefix(uri, "library://x/directory/") {
		t.Fatalf("unexpected URI prefix: %s", uri)
	}

	decoded, err := url.QueryUnescape(strings.TrimPrefix(uri, "library://x/directory/"))
	if err != nil {
		t.Fatalf("QueryUnescape() error = %v", err)
	}

	parts := strings.SplitN(decoded, "?", 2)
	if len(parts) != 2 {
		t.Fatalf("expected filter path and query, got %q", decoded)
	}
	if parts[0] != "/library/sections/1/all" {
		t.Fatalf("unexpected filter path: %s", parts[0])
	}

	values, err := url.ParseQuery(parts[1])
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}

	if values.Get("director") != "11,12" {
		t.Fatalf("unexpected director filter: %q", values.Get("director"))
	}
	if values.Get("genre") != "21,22" {
		t.Fatalf("unexpected genre filter: %q", values.Get("genre"))
	}
	if values.Get("collection") != "41,42" {
		t.Fatalf("unexpected collection filter: %q", values.Get("collection"))
	}
	if values.Get("studio") != "51,52" {
		t.Fatalf("unexpected studio filter: %q", values.Get("studio"))
	}
	if values.Get("year>=") != "1990" || values.Get("year<=") != "2000" {
		t.Fatalf("unexpected year filters: %v", values)
	}
	if values.Get("unwatched") != "1" {
		t.Fatalf("unexpected unwatched filter: %q", values.Get("unwatched"))
	}
}

func TestResolveLibraryTag(t *testing.T) {
	tags := []LibraryTagInfo{
		{ID: "1", Name: "Mamoru Oshii"},
		{ID: "2", Name: "Satoshi Kon"},
		{ID: "3", Name: "Kon Ichikawa"},
		{ID: "4", Name: "Production I.G"},
	}

	match, err := resolveLibraryTag(tags, "satoshi kon")
	if err != nil {
		t.Fatalf("resolveLibraryTag() error = %v", err)
	}
	if match.ID != "2" {
		t.Fatalf("expected ID 2, got %s", match.ID)
	}

	if _, ambErr := resolveLibraryTag(tags, "kon"); ambErr == nil {
		t.Fatal("expected ambiguous partial match error")
	}

	match, err = resolveLibraryTag(tags, "Production IG")
	if err != nil {
		t.Fatalf("resolveLibraryTag() punctuation-insensitive error = %v", err)
	}
	if match.ID != "4" {
		t.Fatalf("expected ID 4, got %s", match.ID)
	}
}

func TestFlattenDiscoveryItems_DedupesAcrossMetadataAndHubs(t *testing.T) {
	sectionID := 9
	sectionTitle := "Anime"
	guid := "plex://movie/123"

	rawResp := rawLibraryItemsResponse{}
	rawResp.MediaContainer.Metadata = []rawMediaMetadata{
		{
			RatingKey:           "42",
			Title:               "Ghost in the Shell",
			Type:                "movie",
			GUID:                &guid,
			LibrarySectionID:    &sectionID,
			LibrarySectionTitle: &sectionTitle,
		},
	}
	rawResp.MediaContainer.Hub = []struct {
		Metadata []rawMediaMetadata `json:"Metadata"`
	}{
		{
			Metadata: []rawMediaMetadata{
				{
					RatingKey: "42",
					Title:     "Ghost in the Shell",
					Type:      "movie",
					GUID:      &guid,
				},
			},
		},
	}

	items := flattenDiscoveryItems(rawResp)
	if len(items) != 1 {
		t.Fatalf("expected 1 deduped item, got %d", len(items))
	}
	if items[0].LibrarySectionID == nil || *items[0].LibrarySectionID != 9 {
		t.Fatalf("expected section metadata to be preserved, got %#v", items[0])
	}
}

func TestFlattenDiscoveryItems_DedupesDistinctRatingKeysSharingGUID(t *testing.T) {
	guid := "plex://movie/123"

	rawResp := rawLibraryItemsResponse{}
	rawResp.MediaContainer.Metadata = []rawMediaMetadata{
		{
			RatingKey: "42",
			Title:     "Ghost in the Shell",
			Type:      "movie",
			GUID:      &guid,
		},
	}
	rawResp.MediaContainer.Hub = []struct {
		Metadata []rawMediaMetadata `json:"Metadata"`
	}{
		{
			Metadata: []rawMediaMetadata{
				{
					RatingKey: "43",
					Title:     "Ghost in the Shell",
					Type:      "movie",
					GUID:      &guid,
				},
			},
		},
	}

	items := flattenDiscoveryItems(rawResp)
	if len(items) != 1 {
		t.Fatalf("expected logical duplicate items to be deduped, got %#v", items)
	}
	if items[0].RatingKey != "42" {
		t.Fatalf("expected first local rating key to be preserved, got %#v", items)
	}
}

func TestFlattenDiscoveryItems_DedupesUsingGuidArrayFallback(t *testing.T) {
	rawResp := rawLibraryItemsResponse{}
	rawResp.MediaContainer.Metadata = []rawMediaMetadata{
		{
			Title: "Ghost in the Shell",
			Type:  "movie",
			Guid: []struct {
				ID string `json:"id"`
			}{
				{ID: "plex://movie/123"},
			},
		},
	}
	rawResp.MediaContainer.Hub = []struct {
		Metadata []rawMediaMetadata `json:"Metadata"`
	}{
		{
			Metadata: []rawMediaMetadata{
				{
					Title: "Ghost in the Shell",
					Type:  "movie",
					Guid: []struct {
						ID string `json:"id"`
					}{
						{ID: "plex://movie/123"},
					},
				},
			},
		},
	}

	items := flattenDiscoveryItems(rawResp)
	if len(items) != 1 {
		t.Fatalf("expected 1 deduped item, got %d", len(items))
	}
	if items[0].GUID == nil || *items[0].GUID != "plex://movie/123" {
		t.Fatalf("expected GUID fallback to be preserved, got %#v", items[0])
	}
}

func TestFlattenDiscoveryItems_DedupesComplementaryIdentifiers(t *testing.T) {
	guid := "plex://movie/123"
	year := 1995

	rawResp := rawLibraryItemsResponse{}
	rawResp.MediaContainer.Metadata = []rawMediaMetadata{
		{
			RatingKey: "42",
			Key:       "/library/metadata/42",
			Title:     "Ghost in the Shell",
			Type:      "movie",
			Year:      &year,
		},
	}
	rawResp.MediaContainer.Hub = []struct {
		Metadata []rawMediaMetadata `json:"Metadata"`
	}{
		{
			Metadata: []rawMediaMetadata{
				{
					Title: "Ghost in the Shell",
					Type:  "movie",
					Year:  &year,
					GUID:  &guid,
				},
			},
		},
	}

	items := flattenDiscoveryItems(rawResp)
	if len(items) != 1 {
		t.Fatalf("expected complementary identifiers to dedupe, got %#v", items)
	}
	if items[0].RatingKey != "42" {
		t.Fatalf("expected local identifier to survive merge, got %#v", items[0])
	}
	if items[0].GUID == nil || *items[0].GUID != guid {
		t.Fatalf("expected GUID to be filled from duplicate result, got %#v", items[0])
	}
}

func TestFlattenDiscoveryItems_PreservesDistinctEpisodesSharingTitle(t *testing.T) {
	year := 2024
	showOne := "Show One"
	showTwo := "Show Two"
	seasonOne := "Season 1"
	parentIndex := 1
	episodeIndex := 1
	firstGUID := "plex://episode/101"

	rawResp := rawLibraryItemsResponse{}
	rawResp.MediaContainer.Metadata = []rawMediaMetadata{
		{
			RatingKey:        "101",
			Key:              "/library/metadata/101",
			Title:            "Pilot",
			Type:             "episode",
			Year:             &year,
			GrandparentTitle: &showOne,
			ParentTitle:      &seasonOne,
			ParentIndex:      &parentIndex,
			Index:            &episodeIndex,
		},
	}
	rawResp.MediaContainer.Hub = []struct {
		Metadata []rawMediaMetadata `json:"Metadata"`
	}{
		{
			Metadata: []rawMediaMetadata{
				{
					Title:            "Pilot",
					Type:             "episode",
					Year:             &year,
					GrandparentTitle: &showTwo,
					ParentTitle:      &seasonOne,
					ParentIndex:      &parentIndex,
					Index:            &episodeIndex,
					GUID:             &firstGUID,
				},
			},
		},
	}

	items := flattenDiscoveryItems(rawResp)
	if len(items) != 2 {
		t.Fatalf("expected distinct episodic results to stay separate, got %#v", items)
	}
}
