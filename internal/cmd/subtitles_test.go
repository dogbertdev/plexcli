package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
)

func TestSubtitlesMissingCmd_extractSubtitleInfo(t *testing.T) {
	tests := []struct {
		name            string
		item            *components.Metadata
		requestedLangs  []string
		expectedTitle   string
		expectedAvail   []string
		expectedMissing []string
	}{
		{
			name: "item with no subtitles",
			item: &components.Metadata{
				Title: "Test Movie",
				Type:  "movie",
				Media: []components.Media{
					{Part: []components.Part{
						{Stream: []components.Stream{}},
					}},
				},
			},
			requestedLangs:  []string{"en", "de"},
			expectedTitle:   "Test Movie",
			expectedAvail:   []string{},
			expectedMissing: []string{"en", "de"},
		},
		{
			name: "item with some subtitles",
			item: &components.Metadata{
				Title: "Test Movie 2",
				Type:  "movie",
				Media: []components.Media{
					{Part: []components.Part{
						{Stream: []components.Stream{
							{StreamType: components.StreamTypeSubtitle, LanguageCode: subtitleStrPtr("en"), Codec: "srt"},
							{StreamType: components.StreamTypeSubtitle, LanguageCode: subtitleStrPtr("fr"), Codec: "srt"},
						}},
					}},
				},
			},
			requestedLangs:  []string{"en", "de"},
			expectedTitle:   "Test Movie 2",
			expectedAvail:   []string{"en", "fr"},
			expectedMissing: []string{"de"},
		},
		{
			name: "item with all requested subtitles",
			item: &components.Metadata{
				Title: "Test Movie 3",
				Type:  "movie",
				Media: []components.Media{
					{Part: []components.Part{
						{Stream: []components.Stream{
							{StreamType: components.StreamTypeSubtitle, LanguageCode: subtitleStrPtr("en"), Codec: "srt"},
							{StreamType: components.StreamTypeSubtitle, LanguageCode: subtitleStrPtr("de"), Codec: "srt"},
						}},
					}},
				},
			},
			requestedLangs:  []string{"en", "de"},
			expectedTitle:   "Test Movie 3",
			expectedAvail:   []string{"en", "de"},
			expectedMissing: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &SubtitlesMissingCmd{}
			result := cmd.extractSubtitleInfo(tt.item, tt.requestedLangs)

			if result.Title != tt.expectedTitle {
				t.Errorf("expected title %q, got %q", tt.expectedTitle, result.Title)
			}

			if len(result.AvailableSubs) != len(tt.expectedAvail) {
				t.Errorf("expected %d available subs, got %d", len(tt.expectedAvail), len(result.AvailableSubs))
			}

			if len(result.MissingSubs) != len(tt.expectedMissing) {
				t.Errorf("expected %d missing subs, got %d", len(tt.expectedMissing), len(result.MissingSubs))
			}
		})
	}
}

func TestSubtitlesMissingCmd_extractSubtitleInfo_withMissing(t *testing.T) {
	items := []*components.Metadata{
		{
			Title: "Movie with subs",
			Type:  "movie",
			Media: []components.Media{
				{Part: []components.Part{
					{Stream: []components.Stream{
						{StreamType: components.StreamTypeSubtitle, LanguageCode: subtitleStrPtr("en"), Codec: "srt"},
					}},
				}},
			},
		},
		{
			Title: "Movie without subs",
			Type:  "movie",
			Media: []components.Media{
				{Part: []components.Part{
					{Stream: []components.Stream{}},
				}},
			},
		},
	}

	cmd := &SubtitlesMissingCmd{Type: "all"}
	var results []SubtitleInfo
	for _, item := range items {
		info := cmd.extractSubtitleInfo(item, []string{"en", "de"})
		if len(info.MissingSubs) > 0 {
			results = append(results, info)
		}
	}

	if len(results) != 2 {
		t.Errorf("expected 2 results with missing subs, got %d", len(results))
	}
}

func TestSubtitlesMissingCmd_extractSubtitleInfo_UsesLanguageFallback(t *testing.T) {
	cmd := &SubtitlesMissingCmd{}
	item := &components.Metadata{
		Title: "Fallback Language",
		Type:  "movie",
		Media: []components.Media{
			{
				Part: []components.Part{
					{
						Stream: []components.Stream{
							{StreamType: components.StreamTypeSubtitle, Language: subtitleStrPtr("eng"), Codec: "srt"},
						},
					},
				},
			},
		},
	}

	result := cmd.extractSubtitleInfo(item, []string{"en"})
	if len(result.MissingSubs) != 0 {
		t.Fatalf("expected no missing subtitles, got %v", result.MissingSubs)
	}
	if len(result.AvailableSubs) != 1 || result.AvailableSubs[0] != "eng" {
		t.Fatalf("expected available subtitle eng, got %v", result.AvailableSubs)
	}
}

func TestSubtitlesMissingCmd_extractSubtitleInfo_filterByType(t *testing.T) {
	items := []*components.Metadata{
		{
			Title: "Movie",
			Type:  "movie",
			Media: []components.Media{},
		},
		{
			Title: "Episode",
			Type:  "episode",
			Media: []components.Media{},
		},
	}

	cmd := &SubtitlesMissingCmd{Type: "movie"}
	var results []SubtitleInfo
	for _, item := range items {
		if cmd.Type != "all" && anyToString(item.Type) != cmd.Type {
			continue
		}
		info := cmd.extractSubtitleInfo(item, []string{"en"})
		if len(info.MissingSubs) > 0 {
			results = append(results, info)
		}
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result for movie type, got %d", len(results))
	}

	if len(results) > 0 && results[0].Type != "movie" {
		t.Errorf("expected type movie, got %q", results[0].Type)
	}
}

func TestParseLangList(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"en", []string{"en"}},
		{"en,de,fr", []string{"en", "de", "fr"}},
		{"EN, DE , FR", []string{"en", "de", "fr"}},
		{"", []string{}},
		{"a, b, , c", []string{"a", "b", "c"}},
	}

	for _, tt := range tests {
		result := parseLangList(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseLangList(%q): expected %v, got %v", tt.input, tt.expected, result)
			continue
		}
		for i, lang := range result {
			if lang != tt.expected[i] {
				t.Errorf("parseLangList(%q)[%d]: expected %q, got %q", tt.input, i, tt.expected[i], lang)
			}
		}
	}
}

func TestSubtitlesMissingCmd_outputResults(t *testing.T) {
	cmd := &SubtitlesMissingCmd{Output: "table"}
	results := []SubtitleInfo{
		{
			Title:         "Test Movie",
			Year:          2023,
			Type:          "movie",
			AvailableSubs: []string{"en"},
			MissingSubs:   []string{"de", "fr"},
		},
	}

	var buf bytes.Buffer
	err := cmd.outputResults(&buf, results)
	if err != nil {
		t.Fatalf("outputResults failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Test Movie") {
		t.Errorf("expected output to contain 'Test Movie', got: %s", output)
	}
	if !strings.Contains(output, "de,fr") {
		t.Errorf("expected output to contain missing subs 'de,fr', got: %s", output)
	}
}

func TestGetTitle(t *testing.T) {
	tests := []struct {
		item     *components.Metadata
		expected string
	}{
		{&components.Metadata{Title: "Test Movie"}, "Test Movie"},
		{&components.Metadata{Title: ""}, "Unknown"},
	}

	for _, tt := range tests {
		result := getTitle(tt.item)
		if result != tt.expected {
			t.Errorf("getTitle(%v): expected %q, got %q", tt.item, tt.expected, result)
		}
	}
}

func subtitleStrPtr(s string) *string {
	return &s
}
