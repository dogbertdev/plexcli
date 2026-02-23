package cmd

import (
	"fmt"
	"io"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type LibrariesCmd struct {
	Type   string `help:"Filter by library type: movie, show, artist, photo, or all" default:"all" enum:"movie,show,artist,photo,all"`
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type LibraryItem struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

func (c *LibrariesCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	sections, err := cc.Client.GetSections(cc.Ctx)
	if err != nil {
		return fmt.Errorf("failed to get sections: %w", err)
	}

	items := make([]LibraryItem, 0, len(sections))
	for _, section := range sections {
		if c.Type != "all" && section.Type != c.Type {
			continue
		}

		title := "Unknown"
		if section.Title != nil && *section.Title != "" {
			title = *section.Title
		}

		items = append(items, LibraryItem{
			ID:    section.ID,
			Title: title,
			Type:  section.Type,
		})
	}

	if len(items) == 0 {
		fmt.Fprintln(u.Err(), "No libraries found")
		return nil
	}

	return c.output(u.Out(), items)
}

func (c *LibrariesCmd) output(w io.Writer, items []LibraryItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"ID", "TITLE", "TYPE"}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, []string{item.ID, item.Title, item.Type})
	}

	return formatter.Format(w, header, rows, items)
}
