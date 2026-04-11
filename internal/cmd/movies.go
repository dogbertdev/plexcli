package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

// MoviesCmd lists movies with optional filtering
type MoviesCmd struct {
	Title    []string `help:"Filter by title (case-insensitive, partial match). Repeat for OR matching." short:"t" name:"title"`
	Director []string `help:"Filter by director name (case-insensitive, partial match). Repeat for OR matching." short:"d" name:"director"`
	Actor    []string `help:"Filter by actor name (case-insensitive, partial match). Repeat for OR matching." short:"a" name:"actor"`
	Genre    []string `help:"Filter by genre name (case-insensitive, partial match). Repeat for OR matching." short:"g" name:"genre"`
	Country  []string `help:"Filter by country name (case-insensitive, partial match). Repeat for OR matching." short:"c" name:"country"`
	Section  string   `help:"Library section ID (required)" short:"s" required:""`
	Dedupe   string   `help:"Dedupe mode: guid, title-year, or none" default:"guid" enum:"guid,title-year,none"`
	KeysOnly bool     `help:"Output only rating keys (space-separated, for piping to playlist create)" default:"false"`
	Limit    int      `help:"Maximum number of movies to display" default:"0"`
	Output   string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type MovieItem struct {
	RatingKey     string   `json:"rating_key"`
	GUID          string   `json:"guid,omitempty"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title,omitempty"`
	Year          int      `json:"year,omitempty"`
	Directors     []string `json:"directors,omitempty"`
	Actors        []string `json:"actors,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	Countries     []string `json:"countries,omitempty"`
	Collections   []string `json:"collections,omitempty"`
}

func (c *MoviesCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	movies, err := cc.Client.GetMovies(cc.Ctx, c.Section, plexclient.MovieFilters{
		Title:    c.Title,
		Director: c.Director,
		Actor:    c.Actor,
		Genre:    c.Genre,
		Country:  c.Country,
		Dedupe:   c.Dedupe,
	})
	if err != nil {
		return fmt.Errorf("failed to get movies: %w", err)
	}

	if len(movies) == 0 {
		fmt.Fprintln(u.Err(), "No movies found")
		return nil
	}

	if c.Limit > 0 && len(movies) > c.Limit {
		movies = movies[:c.Limit]
	}

	if c.KeysOnly {
		keys := make([]string, len(movies))
		for i, movie := range movies {
			keys[i] = movie.RatingKey
		}
		fmt.Fprintln(u.Out(), strings.Join(keys, " "))
		return nil
	}

	outputItems := make([]MovieItem, 0, len(movies))
	for _, m := range movies {
		outputItems = append(outputItems, MovieItem{
			RatingKey:     m.RatingKey,
			GUID:          m.GUID,
			Title:         m.Title,
			OriginalTitle: m.OriginalTitle,
			Year:          m.Year,
			Directors:     m.Directors,
			Actors:        m.Actors,
			Genres:        m.Genres,
			Countries:     m.Countries,
			Collections:   m.Collections,
		})
	}

	return c.output(u.Out(), outputItems)
}

func (c *MoviesCmd) output(w io.Writer, items []MovieItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"RATING KEY", "TITLE", "ORIGINAL TITLE", "YEAR", "DIRECTORS", "ACTORS", "GENRES", "COUNTRIES", "COLLECTIONS"}
	rows := make([][]string, 0, len(items))

	for _, item := range items {
		yearStr := ""
		if item.Year > 0 {
			yearStr = fmt.Sprintf("%d", item.Year)
		}
		rows = append(rows, []string{
			item.RatingKey,
			item.Title,
			item.OriginalTitle,
			yearStr,
			strings.Join(item.Directors, ", "),
			strings.Join(item.Actors, ", "),
			strings.Join(item.Genres, ", "),
			strings.Join(item.Countries, ", "),
			strings.Join(item.Collections, ", "),
		})
	}

	return formatter.Format(w, header, rows, items)
}
