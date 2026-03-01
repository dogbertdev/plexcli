package cmd

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/cache"
	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type CacheCmd struct {
	Clear CacheClearCmd `cmd:"" help:"Clear local library cache"`
}

type CacheClearCmd struct {
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type CacheClearResult struct {
	Path    string `json:"path"`
	Cleared bool   `json:"cleared"`
}

func (c *CacheClearCmd) Run(ctx *kong.Context, u *ui.UI, _ *config.Config) error {
	cacheDir, err := cache.DefaultLibraryPayloadCacheDir()
	if err != nil {
		return fmt.Errorf("failed to resolve cache directory: %w", err)
	}

	_, statErr := os.Stat(cacheDir)
	existed := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return fmt.Errorf("failed to access cache directory: %w", statErr)
	}

	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("failed to clear cache directory: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("failed to recreate cache directory: %w", err)
	}

	result := CacheClearResult{
		Path:    cacheDir,
		Cleared: existed,
	}

	return c.output(u.Out(), result)
}

func (c *CacheClearCmd) output(w io.Writer, result CacheClearResult) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"FIELD", "VALUE"}
	rows := [][]string{
		{"Path", result.Path},
		{"Cleared", strconv.FormatBool(result.Cleared)},
	}

	return formatter.Format(w, header, rows, result)
}
