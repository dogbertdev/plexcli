package cmd

import (
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
)

func TestForEachStream_VisitsAllStreams(t *testing.T) {
	item := &components.Metadata{
		Media: []components.Media{
			{
				Part: []components.Part{
					{
						Stream: []components.Stream{
							{StreamType: components.StreamTypeAudio, Codec: "aac"},
							{StreamType: components.StreamTypeSubtitle, LanguageCode: strPtr("eng"), Codec: "srt"},
						},
					},
				},
			},
			{
				Part: []components.Part{
					{
						Stream: []components.Stream{
							{StreamType: components.StreamTypeAudio, Codec: "ac3"},
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
		if stream.StreamType == components.StreamTypeAudio {
			audioSeen++
		}
		if stream.StreamType == components.StreamTypeSubtitle {
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
