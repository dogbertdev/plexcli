package cmd

import (
	"testing"

	"github.com/LukeHagar/plexgo/models/components"
)

func TestFormatSize(t *testing.T) {
	tests := []struct {
		name     string
		bytes    int64
		expected string
	}{
		{"zero bytes", 0, "-"},
		{"negative bytes", -1, "-"},
		{"bytes", 512, "512 B"},
		{"kilobytes", 1024, "1.0 KB"},
		{"megabytes", 1024 * 1024, "1.0 MB"},
		{"gigabytes", 1024 * 1024 * 1024, "1.0 GB"},
		{"terabytes", 1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{"mixed", 1536, "1.5 KB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSize(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatSize(%d) = %s; want %s", tt.bytes, result, tt.expected)
			}
		})
	}
}

func TestExtractFilePaths(t *testing.T) {
	cmd := &FilePathsCmd{}

	file1 := "/movies/Movie 1.mkv"
	size1 := int64(1024 * 1024 * 1024)
	file2 := "/movies/Movie 2.mp4"
	size2 := int64(2 * 1024 * 1024 * 1024)

	items := []*components.Metadata{
		{
			Title: "Movie 1",
			Media: []components.Media{
				{
					Part: []components.Part{
						{File: &file1, Size: &size1},
					},
				},
			},
		},
		{
			Title: "Movie 2",
			Media: []components.Media{
				{
					Part: []components.Part{
						{File: &file2, Size: &size2},
					},
				},
			},
		},
	}

	result := cmd.extractFilePaths(items)

	if len(result) != 2 {
		t.Errorf("Expected 2 file paths, got %d", len(result))
	}

	if result[0].Title != "Movie 1" {
		t.Errorf("Expected title 'Movie 1', got %s", result[0].Title)
	}

	if result[0].FilePath != file1 {
		t.Errorf("Expected file path '%s', got %s", file1, result[0].FilePath)
	}

	if result[0].Size != size1 {
		t.Errorf("Expected size %d, got %d", size1, result[0].Size)
	}
}

func TestExtractFilePaths_NoMedia(t *testing.T) {
	cmd := &FilePathsCmd{}

	items := []*components.Metadata{
		{Title: "Movie Without Media"},
	}

	result := cmd.extractFilePaths(items)

	if len(result) != 0 {
		t.Errorf("Expected 0 file paths for item without media, got %d", len(result))
	}
}

func TestExtractFilePaths_NoFile(t *testing.T) {
	cmd := &FilePathsCmd{}

	items := []*components.Metadata{
		{
			Title: "Movie Without File",
			Media: []components.Media{
				{
					Part: []components.Part{
						{File: nil, Size: nil},
					},
				},
			},
		},
	}

	result := cmd.extractFilePaths(items)

	if len(result) != 0 {
		t.Errorf("Expected 0 file paths for item without file, got %d", len(result))
	}
}

func TestFilePathInfo_Struct(t *testing.T) {
	info := FilePathInfo{
		Title:    "Test Movie",
		FilePath: "/path/to/movie.mkv",
		Size:     1024,
	}

	if info.Title != "Test Movie" {
		t.Errorf("Expected title 'Test Movie', got %s", info.Title)
	}

	if info.FilePath != "/path/to/movie.mkv" {
		t.Errorf("Expected file path '/path/to/movie.mkv', got %s", info.FilePath)
	}

	if info.Size != 1024 {
		t.Errorf("Expected size 1024, got %d", info.Size)
	}
}
