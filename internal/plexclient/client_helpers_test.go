package plexclient

import "testing"

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
