package cmd

import (
	"testing"

	"github.com/dogbertdev/plexcli/internal/plexclient"
)

func TestFindStreamByLanguage(t *testing.T) {
	streams := []plexclient.StreamInfo{
		{ID: 11, StreamType: 2, Language: "Japanese", LanguageCode: "jpn", Title: "Japanese (AAC)"},
		{ID: 21, StreamType: 3, Language: "English", LanguageCode: "eng", Title: "Signs and Songs"},
		{ID: 22, StreamType: 3, Language: "English", LanguageCode: "eng", Title: "English (Full)"},
		{ID: 23, StreamType: 3, Language: "English", LanguageCode: "eng", Title: "English (Full SDH)"},
	}

	tests := []struct {
		name       string
		streamType int
		query      string
		want       int
	}{
		{name: "audio alias match", streamType: 2, query: "japanese", want: 11},
		{name: "audio code match", streamType: 2, query: "jpn", want: 11},
		{name: "subtitle full english prefers full", streamType: 3, query: "full english", want: 22},
		{name: "subtitle english prefers full over signs", streamType: 3, query: "english", want: 22},
		{name: "subtitle code match prefers full", streamType: 3, query: "eng", want: 22},
		{name: "missing stream", streamType: 3, query: "deu", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findStreamByLanguage(streams, tt.streamType, tt.query)
			if got != tt.want {
				t.Fatalf("findStreamByLanguage() = %d, want %d", got, tt.want)
			}
		})
	}
}
