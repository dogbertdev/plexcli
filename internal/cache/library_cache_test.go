package cache

import (
	"path/filepath"
	"testing"
	"time"
)

func TestLibraryPayloadCache_SaveAndLoad(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cache, err := NewLibraryPayloadCache(tmpDir)
	if err != nil {
		t.Fatalf("NewLibraryPayloadCache() error = %v", err)
	}

	payload := []byte(`{"MediaContainer":{"Metadata":[{"title":"Example"}]}}`)
	if saveErr := cache.Save("section-key", payload); saveErr != nil {
		t.Fatalf("Save() error = %v", saveErr)
	}

	got, hit, err := cache.Load("section-key", 5*time.Minute)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !hit {
		t.Fatalf("expected cache hit")
	}
	if string(got) != string(payload) {
		t.Fatalf("unexpected payload: got %q want %q", string(got), string(payload))
	}
}

func TestLibraryPayloadCache_LoadMissWhenExpired(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cache, err := NewLibraryPayloadCache(tmpDir)
	if err != nil {
		t.Fatalf("NewLibraryPayloadCache() error = %v", err)
	}

	payload := []byte(`{"MediaContainer":{"Metadata":[]}}`)
	if saveErr := cache.Save("expired-key", payload); saveErr != nil {
		t.Fatalf("Save() error = %v", saveErr)
	}

	cache.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	_, hit, err := cache.Load("expired-key", 1*time.Minute)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if hit {
		t.Fatalf("expected cache miss for expired payload")
	}
}

func TestLibraryPayloadCache_LoadMissWhenMissing(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cache, err := NewLibraryPayloadCache(tmpDir)
	if err != nil {
		t.Fatalf("NewLibraryPayloadCache() error = %v", err)
	}

	_, hit, err := cache.Load("missing-key", 1*time.Minute)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if hit {
		t.Fatalf("expected cache miss for missing payload")
	}
}

func TestNewDefaultLibraryPayloadCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "xdg-cache"))
	cache, err := NewDefaultLibraryPayloadCache()
	if err != nil {
		t.Fatalf("NewDefaultLibraryPayloadCache() error = %v", err)
	}
	if cache == nil {
		t.Fatalf("expected cache instance")
	}
}
