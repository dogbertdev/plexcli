package cmd

import (
	"fmt"
	"io"

	"github.com/alecthomas/kong"
	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

type MatchCmd struct {
	Search MatchSearchCmd `cmd:"" help:"Search for metadata matches for an item"`
	Apply  MatchApplyCmd  `cmd:"" help:"Apply a metadata match to an item"`
}

type MatchSearchCmd struct {
	RatingKey string `arg:"" help:"Rating key of the item to search matches for"`
	Title     string `help:"Override title for search" short:"t"`
	Year      int    `help:"Override year for search" short:"y"`
	Output    string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type MatchApplyCmd struct {
	RatingKey string `arg:"" help:"Rating key of the item to update"`
	GUID      string `arg:"" help:"GUID of the match to apply (from match search results)"`
	Name      string `help:"Name hint for the match" short:"n"`
}

func (c *MatchSearchCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	results, err := cc.Client.SearchMatches(cc.Ctx, c.RatingKey, c.Title, c.Year)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Fprintln(u.Err(), "No matches found")
		return nil
	}

	return c.outputResults(u.Out(), results)
}

func (c *MatchSearchCmd) outputResults(w io.Writer, results []plexclient.MatchResult) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"GUID", "NAME", "YEAR", "SUMMARY"}
	rows := make([][]string, len(results))

	for i, r := range results {
		yearStr := ""
		if r.Year > 0 {
			yearStr = fmt.Sprintf("%d", r.Year)
		}

		summary := r.Summary
		if len(summary) > 80 {
			summary = summary[:77] + "..."
		}

		rows[i] = []string{
			r.GUID,
			r.Name,
			yearStr,
			summary,
		}
	}

	return formatter.Format(w, header, rows, results)
}

func (c *MatchApplyCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	err = cc.Client.ApplyMatch(cc.Ctx, c.RatingKey, c.GUID, c.Name)
	if err != nil {
		return err
	}

	fmt.Fprintf(u.Out(), "Successfully applied match %s to item %s\n", c.GUID, c.RatingKey)
	return nil
}
