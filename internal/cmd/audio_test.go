package cmd

import (
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
)

func TestAudioCheckCmd_extractAudioInfo(t *testing.T) {
	channels := 6
	year := 2024

	item := &components.Metadata{
		Title: "Sample Movie",
		Type:  "movie",
		Year:  &year,
		Media: []components.Media{
			{
				AudioChannels: &channels,
				Part: []components.Part{
					{
						Stream: []components.Stream{
							{StreamType: components.StreamTypeAudio, Codec: "dts"},
							{StreamType: components.StreamTypeSubtitle, LanguageCode: audioStrPtr("eng"), Codec: "srt"},
						},
					},
				},
			},
		},
	}

	cmd := &AudioCheckCmd{}
	infos := cmd.extractAudioInfo(item)
	if len(infos) != 1 {
		t.Fatalf("expected 1 audio info, got %d", len(infos))
	}

	info := infos[0]
	if info.Title != "Sample Movie" {
		t.Fatalf("expected title Sample Movie, got %q", info.Title)
	}
	if info.Type != "movie" {
		t.Fatalf("expected type movie, got %q", info.Type)
	}
	if info.Codec != "dts" {
		t.Fatalf("expected codec dts, got %q", info.Codec)
	}
	if info.Channels != 6 {
		t.Fatalf("expected channels 6, got %d", info.Channels)
	}
}

func TestAudioCheckCmd_checkAudio_FilterByType(t *testing.T) {
	channels := 2
	year := 2024

	items := []*components.Metadata{
		{
			Title: "Movie",
			Type:  "movie",
			Year:  &year,
			Media: []components.Media{
				{
					AudioChannels: &channels,
					Part:          []components.Part{{Stream: []components.Stream{{StreamType: components.StreamTypeAudio, Codec: "aac"}}}},
				},
			},
		},
		{
			Title: "Episode",
			Type:  "episode",
			Year:  &year,
			Media: []components.Media{
				{
					AudioChannels: &channels,
					Part:          []components.Part{{Stream: []components.Stream{{StreamType: components.StreamTypeAudio, Codec: "aac"}}}},
				},
			},
		},
	}

	cmd := &AudioCheckCmd{Type: "movie", MinChannels: 6}
	results := cmd.checkAudio(items, nil)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for movie filter, got %d", len(results))
	}
	if results[0].Type != "movie" {
		t.Fatalf("expected result type movie, got %q", results[0].Type)
	}
}

func audioStrPtr(s string) *string {
	return &s
}
