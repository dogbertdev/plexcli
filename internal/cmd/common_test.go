package cmd

import (
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
)

func TestForEachStream_VisitsAllStreams(t *testing.T) {
	streamTypeAudio := int64(2)
	streamTypeSubtitle := int64(3)

	item := &components.Metadata{
		Media: []components.Media{
			{
				Part: []components.Part{
					{
						Stream: []components.Stream{
							{StreamType: &streamTypeAudio, Codec: "aac"},
							{StreamType: &streamTypeSubtitle, LanguageCode: "eng"},
						},
					},
				},
			},
			{
				Part: []components.Part{
					{
						Stream: []components.Stream{
							{StreamType: &streamTypeAudio, Codec: "ac3"},
						},
					},
				},
			},
		},
	}

	visited := 0
	audioSeen := 0
	subtitleSeen := 0
	forEachStream(item, func(_ *components.Media, stream *components.Stream) {
		visited++
		if stream.StreamType != nil && *stream.StreamType == 2 {
			audioSeen++
		}
		if stream.StreamType != nil && *stream.StreamType == 3 {
			subtitleSeen++
		}
	})

	if visited != 3 {
		t.Fatalf("expected 3 streams visited, got %d", visited)
	}
	if audioSeen != 2 {
		t.Fatalf("expected 2 audio streams, got %d", audioSeen)
	}
	if subtitleSeen != 1 {
		t.Fatalf("expected 1 subtitle stream, got %d", subtitleSeen)
	}
}

