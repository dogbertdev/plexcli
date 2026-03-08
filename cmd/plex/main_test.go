package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/dogbertdev/plexcli/internal/config"
)

func TestHasCLIFlag(t *testing.T) {
	args := []string{"--cache-ttl=0", "--no-cache", "recently-watched"}

	if !hasCLIFlag(args, "cache-ttl") {
		t.Fatalf("expected cache-ttl to be detected")
	}
	if !hasCLIFlag(args, "no-cache") {
		t.Fatalf("expected no-cache to be detected")
	}
	if hasCLIFlag(args, "refresh-cache") {
		t.Fatalf("did not expect refresh-cache to be detected")
	}
}

func TestLoadConfigPreservesFileCacheTTLWhenFlagUnset(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	cfg := &config.Config{
		ServerURL: "http://localhost:32400",
		Token:     "token",
		Timeout:   120,
		CacheTTL:  42,
	}
	if err := config.WriteConfig(cfg); err != nil {
		t.Fatalf("WriteConfig() error = %v", err)
	}

	got, err := loadConfig("", "", "", 0, 300, false, false, false, false, false)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	if got.CacheTTL != 42 {
		t.Fatalf("expected CacheTTL=42 from file, got %d", got.CacheTTL)
	}
}

func TestLoadConfigAppliesCacheFlagsWhenSet(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	// Ensure config path exists to avoid accidental dependence on host filesystem.
	configPath := filepath.Join(tmpHome, ".config", "plexcli", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(configPath, []byte(`{"server_url":"http://localhost:32400","token":"token","cache_ttl":60}`), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := loadConfig("", "", "", 0, 0, true, true, true, true, true)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}

	if got.CacheTTL != 0 {
		t.Fatalf("expected CacheTTL=0, got %d", got.CacheTTL)
	}
	if !got.CacheDisabled {
		t.Fatalf("expected CacheDisabled=true")
	}
	if !got.CacheRefresh {
		t.Fatalf("expected CacheRefresh=true")
	}
}

func TestApplyOutputFormatDoesNotOverwritePartIndexPath(t *testing.T) {
	cli := &CLI{}
	cli.Library.Media.PartIndex.Output = "/tmp/index.bif"

	applyOutputFormat(cli, "json")

	if cli.Library.Media.PartIndex.Output != "/tmp/index.bif" {
		t.Fatalf("expected part-index output path to be preserved, got %q", cli.Library.Media.PartIndex.Output)
	}
}

func TestLibraryMediaFileParsesDirectLibraryPathWithoutIDs(t *testing.T) {
	var cli CLI
	parser, err := kong.New(&cli)
	if err != nil {
		t.Fatalf("kong.New() error = %v", err)
	}

	_, err = parser.Parse([]string{
		"library", "media", "file",
		"--url", "/library/metadata/27017/theme/1771256497",
		"--output", "/tmp/theme.mp3",
	})
	if err != nil {
		t.Fatalf("parser.Parse() error = %v", err)
	}
}
