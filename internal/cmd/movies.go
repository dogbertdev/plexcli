package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/ui"
)

// MoviesCmd lists movies with optional filtering
type MoviesCmd struct {
	Director string `help:"Filter by director name (case-insensitive, partial match)" short:"d"`
	Section  string `help:"Library section ID (required)" short:"s" required:""`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type MovieItem struct {
	RatingKey string `json:"rating_key"`
	Title     string `json:"title"`
	Year      int    `json:"year,omitempty"`
	Director  string `json:"director,omitempty"`
}

func (c *MoviesCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if c.Director == "" {
		return fmt.Errorf("--director flag is required")
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	movies, err := cc.Client.GetMoviesByDirector(cc.Ctx, c.Section, c.Director)
	if err != nil {
		return fmt.Errorf("failed to get movies: %w", err)
	}

	if len(movies) == 0 {
		fmt.Fprintf(u.Err(), "No movies found for director: %s\n", c.Director)
		return nil
	}

	outputItems := make([]MovieItem, 0, len(movies))
	for _, m := range movies {
		outputItems = append(outputItems, MovieItem{
			RatingKey: m.RatingKey,
			Title:     m.Title,
			Year:      m.Year,
			Director:  strings.Join(m.Directors, ", "),
		})
	}

	return c.output(u.Out(), outputItems)
}

func (c *MoviesCmd) output(w io.Writer, items []MovieItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"RATING KEY", "TITLE", "YEAR", "DIRECTOR"}
	rows := make([][]string, 0, len(items))

	for _, item := range items {
		yearStr := ""
		if item.Year > 0 {
			yearStr = fmt.Sprintf("%d", item.Year)
		}
		rows = append(rows, []string{
			item.RatingKey,
			item.Title,
			yearStr,
			item.Director,
		})
	}

	return formatter.Format(w, header, rows, items)
}
