package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type FilePathsCmd struct {
	Title   string `name:"title" help:"Filter by title (substring match)"`
	Section string `name:"section" help:"Filter by library section ID"`
	Limit   int    `help:"Maximum number of items to display" default:"0"`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type FilePathInfo struct {
	Title    string `json:"title"`
	Section  string `json:"section"`
	FilePath string `json:"file_path"`
	Size     int64  `json:"size"`
}

type fileItemWithSection struct {
	item    *components.Metadata
	section string
}

func (c *FilePathsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	items, err := c.fetchItems(cc.Ctx, cc.Client)
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

	header := []string{"TITLE", "SECTION", "FILE PATH", "SIZE"}
	rows := make([][]string, len(filePaths))

	for i, fp := range filePaths {
		rows[i] = []string{
			fp.Title,
			fp.Section,
			fp.FilePath,
			formatSize(fp.Size),
		}
	}

	return formatter.Format(w, header, rows, filePaths)
}

func (c *FilePathsCmd) fetchItems(ctx context.Context, client *plexclient.Client) ([]fileItemWithSection, error) {
	sections, err := client.GetSections(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sections: %w", err)
	}

	sectionNameByID := make(map[string]string, len(sections))
	for _, section := range sections {
		if section.Title != nil {
			sectionNameByID[section.ID] = *section.Title
		}
	}

	targetSectionIDs := c.targetSectionIDs(sections)
	items := make([]fileItemWithSection, 0)
	for _, sectionID := range targetSectionIDs {
		sectionItems, err := client.GetAllLibraryItems(ctx, sectionID)
		if err != nil {
			if c.Section != "" {
				return nil, fmt.Errorf("failed to fetch library items: %w", err)
			}
			continue
		}

		sectionName := sectionNameByID[sectionID]
		if sectionName == "" {
			sectionName = sectionID
		}
		for _, item := range sectionItems {
			items = append(items, fileItemWithSection{item: item, section: sectionName})
		}
	}

	return c.filterItemsByTitle(items), nil
}

func (c *FilePathsCmd) targetSectionIDs(sections []plexclient.Library) []string {
	if c.Section != "" {
		return []string{c.Section}
	}

	sectionIDs := make([]string, 0, len(sections))
	for _, section := range sections {
		sectionIDs = append(sectionIDs, section.ID)
	}
	return sectionIDs
}

func (c *FilePathsCmd) filterItemsByTitle(items []fileItemWithSection) []fileItemWithSection {
	if c.Title == "" {
		return items
	}

	filterTitle := strings.ToLower(c.Title)
	filtered := make([]fileItemWithSection, 0, len(items))
	for _, itemWithSection := range items {
		title := strings.ToLower(anyToString(itemWithSection.item.Title))
		if strings.Contains(title, filterTitle) {
			filtered = append(filtered, itemWithSection)
		}
	}
	return filtered
}

func (c *FilePathsCmd) extractFilePaths(items []fileItemWithSection) []FilePathInfo {
	var filePaths []FilePathInfo

	for _, iws := range items {
		item := iws.item
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
					Title:    anyToString(item.Title),
					Section:  iws.section,
					FilePath: anyToString(part.File),
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
