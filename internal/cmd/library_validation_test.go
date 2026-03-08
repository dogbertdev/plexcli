package cmd

import "testing"

func TestLibrarySubtitleAddCmdRequiresExactlyOneSource(t *testing.T) {
	tests := []struct {
		name    string
		cmd     LibrarySubtitleAddCmd
		wantErr bool
	}{
		{
			name:    "missing url and file",
			cmd:     LibrarySubtitleAddCmd{IDs: "1"},
			wantErr: true,
		},
		{
			name:    "both url and file",
			cmd:     LibrarySubtitleAddCmd{IDs: "1", URL: "https://example.com/en.srt", File: "/tmp/en.srt"},
			wantErr: true,
		},
		{
			name:    "url only valid",
			cmd:     LibrarySubtitleAddCmd{IDs: "1", URL: "https://example.com/en.srt"},
			wantErr: false,
		},
		{
			name:    "file only valid",
			cmd:     LibrarySubtitleAddCmd{IDs: "1", File: "/tmp/en.srt"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cmd.validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLibrarySectionDeleteCmdRequiresYes(t *testing.T) {
	cmd := LibrarySectionDeleteCmd{Section: "1", Yes: false}
	if err := cmd.validate(); err == nil {
		t.Fatalf("expected validate() error without --yes")
	}
}

func TestLibraryRefreshStopAllCmdRequiresYes(t *testing.T) {
	cmd := LibraryRefreshStopAllCmd{Yes: false}
	if err := cmd.validate(); err == nil {
		t.Fatalf("expected validate() error without --yes")
	}
}

func TestLibraryMediaBinaryCommandsRequireOutput(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "section image",
			err:  (&LibrarySectionImageCmd{}).validate(),
		},
		{
			name: "artwork get",
			err:  (&LibraryArtworkGetCmd{}).validate(),
		},
		{
			name: "media file",
			err:  (&LibraryMediaFileCmd{}).validate(),
		},
		{
			name: "part index",
			err:  (&LibraryMediaPartIndexCmd{}).validate(),
		},
		{
			name: "stream get",
			err:  (&LibraryMediaStreamGetCmd{}).validate(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatalf("expected validate() error for %s", tt.name)
			}
		})
	}
}
