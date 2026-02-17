package cmd

import (
	"fmt"
	"io"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/ui"
)

// DirectorsCmd lists all directors in a library
type DirectorsCmd struct {
	Section string `help:"Library section ID (required)" short:"s" required:""`
	Output  string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type DirectorItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *DirectorsCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	directors, err := cc.Client.GetDirectors(cc.Ctx, c.Section)
	if err != nil {
		return fmt.Errorf("failed to get directors: %w", err)
	}

	if len(directors) == 0 {
		fmt.Fprintln(u.Err(), "No directors found in this library")
		return nil
	}

	outputItems := make([]DirectorItem, 0, len(directors))
	for _, d := range directors {
		outputItems = append(outputItems, DirectorItem{
			ID:   d.ID,
			Name: d.Name,
		})
	}

	return c.output(u.Out(), outputItems)
}

func (c *DirectorsCmd) output(w io.Writer, items []DirectorItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"ID", "NAME"}
	rows := make([][]string, 0, len(items))

	for _, item := range items {
		rows = append(rows, []string{
			item.ID,
			item.Name,
		})
	}

	return formatter.Format(w, header, rows, items)
}
