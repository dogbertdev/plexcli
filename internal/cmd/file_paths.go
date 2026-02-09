package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"
	"github.com/user/plexcli/internal/auth"
	"github.com/user/plexcli/internal/config"
	"github.com/user/plexcli/internal/outfmt"
	"github.com/user/plexcli/internal/plexclient"
	"github.com/user/plexcli/internal/ui"
)

type FilePathsCmd struct {
	Title   string `name:"title" help:"Filter by title (substring match)"`
	Section string `name:"section" help:"Filter by library section ID"`
	Limit   int    `help:"Maximum number of items to display" default:"0"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type FilePathInfo struct {
	Title    string `json:"title"`
	FilePath string `json:"file_path"`
	Size     int64  `json:"size"`
}

func (c *FilePathsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	authCtx, cancel := context.WithTimeout(context.Background(), auth.DefaultTimeout)
	defer cancel()

	token, err := auth.GetToken(authCtx, *cfg)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	if cfg.Timeout == 0 {
		timeout = plexclient.DefaultTimeout
	}

	client, err := plexclient.NewClient(cfg.ServerURL, token, plexclient.WithTimeout(timeout))
	if err != nil {
		return fmt.Errorf("failed to create plex client: %w", err)
	}

	fetchCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	items, err := c.fetchItems(fetchCtx, client)
	if err != nil {
		return err
	}

	if len(items) == 0 {
		fmt.Fprintln(u.Err(), "No items found")
		return nil
	}

	filePaths := c.extractFilePaths(items)

	if len(filePaths) == 0 {
		fmt.Fprintln(u.Err(), "No file paths found")
		return nil
	}

	if c.Limit > 0 && len(filePaths) > c.Limit {
		filePaths = filePaths[:c.Limit]
	}

	return c.outputResults(u.Out(), filePaths)
}

func (c *FilePathsCmd) outputResults(w io.Writer, filePaths []FilePathInfo) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"TITLE", "FILE PATH", "SIZE"}
	rows := make([][]string, len(filePaths))

	for i, fp := range filePaths {
		rows[i] = []string{
			fp.Title,
			fp.FilePath,
			formatSize(fp.Size),
		}
	}

	return formatter.Format(w, header, rows, filePaths)
}

func (c *FilePathsCmd) fetchItems(ctx context.Context, client *plexclient.Client) ([]*components.Metadata, error) {
	var items []*components.Metadata

	if c.Section != "" {
		sectionItems, err := client.GetAllLibraryItems(ctx, c.Section)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch library items: %w", err)
		}
		items = sectionItems
	} else {
		sections, err := client.GetSections(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch sections: %w", err)
		}

		for _, section := range sections {
			sectionItems, err := client.GetAllLibraryItems(ctx, section.ID)
			if err != nil {
				continue
			}
			items = append(items, sectionItems...)
		}
	}

	if c.Title != "" {
		filtered := make([]*components.Metadata, 0)
		filterLower := strings.ToLower(c.Title)
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.Title), filterLower) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	return items, nil
}

func (c *FilePathsCmd) extractFilePaths(items []*components.Metadata) []FilePathInfo {
	var filePaths []FilePathInfo

	for _, item := range items {
		if item.Media == nil {
			continue
		}

		for _, media := range item.Media {
			if media.Part == nil {
				continue
			}

			for _, part := range media.Part {
				if part.File == nil {
					continue
				}

				var size int64
				if part.Size != nil {
					size = *part.Size
				}

				filePaths = append(filePaths, FilePathInfo{
					Title:    item.Title,
					FilePath: *part.File,
					Size:     size,
				})
			}
		}
	}

	return filePaths
}

func formatSize(bytes int64) string {
	if bytes <= 0 {
		return "-"
	}

	const unit = 1024.0
	b := float64(bytes)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for b >= unit && i < len(units)-1 {
		b /= unit
		i++
	}

	if i == 0 {
		return fmt.Sprintf("%d B", bytes)
	}
	return fmt.Sprintf("%.1f %s", b, units[i])
}
