package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kong"

	"github.com/dogbertdev/plexcli/internal/config"
	"github.com/dogbertdev/plexcli/internal/outfmt"
	"github.com/dogbertdev/plexcli/internal/plexclient"
	"github.com/dogbertdev/plexcli/internal/ui"
)

// PlaylistCmd is the parent command for playlist subcommands
type PlaylistCmd struct {
	List   PlaylistListCmd   `cmd:"" help:"List all playlists"`
	Create PlaylistCreateCmd `cmd:"" help:"Create a new playlist"`
	Smart  PlaylistSmartCmd  `cmd:"" help:"Create a smart playlist (auto-updates)"`
	Add    PlaylistAddCmd    `cmd:"" help:"Add items to a playlist"`
	Show   PlaylistShowCmd   `cmd:"" help:"Show items in a playlist"`
	Delete PlaylistDeleteCmd `cmd:"" help:"Delete a playlist"`
}

// PlaylistListCmd lists all playlists
type PlaylistListCmd struct {
	Output string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type PlaylistListItem struct {
	RatingKey    string `json:"rating_key"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	PlaylistType string `json:"playlist_type"`
	ItemCount    int    `json:"item_count"`
	Smart        bool   `json:"smart"`
}

func (c *PlaylistListCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	playlists, err := cc.Client.ListPlaylists(cc.Ctx)
	if err != nil {
		return fmt.Errorf("failed to list playlists: %w", err)
	}

	if len(playlists) == 0 {
		fmt.Fprintln(u.Err(), "No playlists found")
		return nil
	}

	outputItems := make([]PlaylistListItem, 0, len(playlists))
	for _, p := range playlists {
		outputItems = append(outputItems, PlaylistListItem{
			RatingKey:    p.RatingKey,
			Title:        p.Title,
			Type:         p.Type,
			PlaylistType: p.PlaylistType,
			ItemCount:    p.LeafCount,
			Smart:        p.Smart,
		})
	}

	return c.output(u.Out(), outputItems)
}

func (c *PlaylistListCmd) output(w io.Writer, items []PlaylistListItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"RATING KEY", "TITLE", "TYPE", "PLAYLIST TYPE", "ITEMS", "SMART"}
	rows := make([][]string, 0, len(items))

	for _, item := range items {
		smartStr := "no"
		if item.Smart {
			smartStr = "yes"
		}
		rows = append(rows, []string{
			item.RatingKey,
			item.Title,
			item.Type,
			item.PlaylistType,
			fmt.Sprintf("%d", item.ItemCount),
			smartStr,
		})
	}

	return formatter.Format(w, header, rows, items)
}

// PlaylistCreateCmd creates a new playlist
type PlaylistCreateCmd struct {
	Name    string   `arg:"" help:"Playlist name"`
	Items   []string `arg:"" optional:"" help:"Rating keys to add"`
	Queries []string `help:"Resolve and append items by title" name:"query"`
	Type    string   `help:"Playlist type: video, audio, photo" default:"video" enum:"video,audio,photo"`
	Output  string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type PlaylistCreateResult struct {
	RatingKey string `json:"rating_key"`
	Title     string `json:"title"`
	ItemCount int    `json:"item_count"`
}

func (c *PlaylistCreateCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	items, err := c.resolveItems(u, cc.Client, cc.Ctx)
	if err != nil {
		return err
	}

	playlist, err := cc.Client.CreatePlaylist(cc.Ctx, c.Name, c.Type, items)
	if err != nil {
		return fmt.Errorf("failed to create playlist: %w", err)
	}

	if playlist == nil {
		return fmt.Errorf("playlist creation returned no result")
	}

	result := PlaylistCreateResult{
		RatingKey: playlist.RatingKey,
		Title:     playlist.Title,
		ItemCount: playlist.LeafCount,
	}

	return c.output(u.Out(), result)
}

func (c *PlaylistCreateCmd) output(w io.Writer, result PlaylistCreateResult) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"RATING KEY", "TITLE", "ITEMS"}
	rows := [][]string{
		{result.RatingKey, result.Title, fmt.Sprintf("%d", result.ItemCount)},
	}

	return formatter.Format(w, header, rows, []PlaylistCreateResult{result})
}

// PlaylistSmartCmd creates a smart playlist
type PlaylistSmartCmd struct {
	Name       string   `arg:"" help:"Playlist name"`
	Section    string   `help:"Library section ID" short:"s" required:""`
	Director   []string `help:"Filter by director name" short:"d" name:"director"`
	Genre      []string `help:"Filter by genre name" name:"genre"`
	Country    []string `help:"Filter by country name" name:"country"`
	Collection []string `help:"Filter by collection name" name:"collection"`
	Studio     []string `help:"Filter by studio name" name:"studio"`
	YearFrom   int      `help:"Inclusive lower year bound" name:"year-from"`
	YearTo     int      `help:"Inclusive upper year bound" name:"year-to"`
	Unwatched  bool     `help:"Only include unwatched items"`
	Type       string   `help:"Playlist type: video, audio" default:"video" enum:"video,audio"`
	Output     string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

func (c *PlaylistSmartCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if err := c.validate(); err != nil {
		return err
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	filters, err := c.resolveFilters(cc.Ctx, cc.Client)
	if err != nil {
		return err
	}

	playlist, err := cc.Client.CreateSmartPlaylistWithFilters(cc.Ctx, c.Name, c.Type, c.Section, filters)
	if err != nil {
		return fmt.Errorf("failed to create smart playlist: %w", err)
	}

	if playlist == nil {
		return fmt.Errorf("playlist creation returned no result")
	}

	result := PlaylistCreateResult{
		RatingKey: playlist.RatingKey,
		Title:     playlist.Title,
		ItemCount: playlist.LeafCount,
	}

	return c.output(u.Out(), result)
}

func (c *PlaylistSmartCmd) output(w io.Writer, result PlaylistCreateResult) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"RATING KEY", "TITLE", "ITEMS", "TYPE"}
	rows := [][]string{
		{result.RatingKey, result.Title, fmt.Sprintf("%d", result.ItemCount), "smart"},
	}

	return formatter.Format(w, header, rows, []PlaylistCreateResult{result})
}

// PlaylistAddCmd adds items to a playlist
type PlaylistAddCmd struct {
	Playlist string   `arg:"" help:"Playlist ID (rating key)"`
	Items    []string `arg:"" optional:"" help:"Rating keys to add"`
	Queries  []string `help:"Resolve and append items by title" name:"query"`
	Output   string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type PlaylistAddResult struct {
	PlaylistID string `json:"playlist_id"`
	ItemsAdded int    `json:"items_added"`
	Message    string `json:"message"`
}

func (c *PlaylistAddCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	items, err := c.resolveItems(u, cc.Client, cc.Ctx)
	if err != nil {
		return err
	}

	err = cc.Client.AddToPlaylist(cc.Ctx, c.Playlist, items)
	if err != nil {
		return fmt.Errorf("failed to add items to playlist: %w", err)
	}

	result := PlaylistAddResult{
		PlaylistID: c.Playlist,
		ItemsAdded: len(items),
		Message:    fmt.Sprintf("Added %d item(s) to playlist %s", len(items), c.Playlist),
	}

	return c.output(u.Out(), result)
}

func (c *PlaylistAddCmd) output(w io.Writer, result PlaylistAddResult) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"PLAYLIST ID", "ITEMS ADDED", "MESSAGE"}
	rows := [][]string{
		{result.PlaylistID, fmt.Sprintf("%d", result.ItemsAdded), result.Message},
	}

	return formatter.Format(w, header, rows, []PlaylistAddResult{result})
}

// PlaylistShowCmd shows items in a playlist
type PlaylistShowCmd struct {
	Playlist string `arg:"" help:"Playlist ID (rating key)"`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type PlaylistShowItem struct {
	RatingKey string `json:"rating_key"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Year      int    `json:"year,omitempty"`
	Show      string `json:"show,omitempty"`
	Season    int    `json:"season,omitempty"`
	Episode   int    `json:"episode,omitempty"`
}

func (c *PlaylistShowCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	items, err := cc.Client.GetPlaylistItems(cc.Ctx, c.Playlist)
	if err != nil {
		return fmt.Errorf("failed to get playlist items: %w", err)
	}

	if len(items) == 0 {
		fmt.Fprintln(u.Err(), "Playlist is empty")
		return nil
	}

	outputItems := make([]PlaylistShowItem, 0, len(items))
	for _, item := range items {
		out := PlaylistShowItem{
			RatingKey: item.RatingKey,
			Title:     item.Title,
			Type:      item.Type,
		}
		if item.Year != nil {
			out.Year = *item.Year
		}
		if item.GrandparentTitle != nil {
			out.Show = *item.GrandparentTitle
		}
		if item.ParentIndex != nil {
			out.Season = *item.ParentIndex
		}
		if item.Index != nil {
			out.Episode = *item.Index
		}
		outputItems = append(outputItems, out)
	}

	return c.output(u.Out(), outputItems)
}

func (c *PlaylistShowCmd) output(w io.Writer, items []PlaylistShowItem) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"RATING KEY", "TITLE", "TYPE", "YEAR", "SHOW", "S", "E"}
	rows := make([][]string, 0, len(items))

	for _, item := range items {
		yearStr := ""
		if item.Year > 0 {
			yearStr = fmt.Sprintf("%d", item.Year)
		}
		seasonStr := ""
		if item.Season > 0 {
			seasonStr = fmt.Sprintf("%d", item.Season)
		}
		episodeStr := ""
		if item.Episode > 0 {
			episodeStr = fmt.Sprintf("%d", item.Episode)
		}
		rows = append(rows, []string{
			item.RatingKey,
			item.Title,
			item.Type,
			yearStr,
			item.Show,
			seasonStr,
			episodeStr,
		})
	}

	return formatter.Format(w, header, rows, items)
}

func (c *PlaylistCreateCmd) resolveItems(u *ui.UI, client *plexclient.Client, ctx context.Context) ([]string, error) {
	return resolvePlaylistItems(u, client, ctx, c.Output, c.Items, c.Queries, SearchResolveOptions{
		Limit:        plexclient.DefaultSearchLimit,
		AllowedTypes: playlistAllowedSearchTypes(c.Type),
	})
}

func (c *PlaylistAddCmd) resolveItems(u *ui.UI, client *plexclient.Client, ctx context.Context) ([]string, error) {
	resolveOpts := SearchResolveOptions{
		Limit: plexclient.DefaultSearchLimit,
	}
	if len(c.Queries) > 0 {
		playlistType, err := resolvePlaylistType(ctx, client, c.Playlist)
		if err != nil {
			return nil, err
		}
		resolveOpts.AllowedTypes = playlistAllowedSearchTypes(playlistType)
	}
	return resolvePlaylistItems(u, client, ctx, c.Output, c.Items, c.Queries, resolveOpts)
}

func resolvePlaylistItems(u *ui.UI, client *plexclient.Client, ctx context.Context, output string, items []string, queries []string, resolveOpts SearchResolveOptions) ([]string, error) {
	resolved := append([]string{}, items...)

	for _, query := range queries {
		resolveOpts.AllowRatingKey = false
		result, err := resolveSingleSearchResult(ctx, client, query, resolveOpts)
		if err != nil {
			if ambErr, ok := err.(*AmbiguousSearchError); ok {
				_ = outputSearchItems(u.Err(), output, searchItemsFromResults(ambErr.Candidates))
			}
			return nil, err
		}
		resolved = append(resolved, result.RatingKey)
	}

	if len(resolved) == 0 {
		return nil, fmt.Errorf("at least one item rating key or --query is required")
	}

	return resolved, nil
}

func resolvePlaylistType(ctx context.Context, client *plexclient.Client, playlistID string) (string, error) {
	playlists, err := client.ListPlaylists(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to inspect playlist %s: %w", playlistID, err)
	}
	for _, playlist := range playlists {
		if playlist.RatingKey == playlistID {
			if strings.TrimSpace(playlist.PlaylistType) == "" {
				return "", fmt.Errorf("playlist %s has no media type", playlistID)
			}
			return playlist.PlaylistType, nil
		}
	}
	return "", fmt.Errorf("playlist %s not found", playlistID)
}

func playlistAllowedSearchTypes(playlistType string) []string {
	switch strings.TrimSpace(playlistType) {
	case "audio":
		return []string{"artist", "album", "track"}
	case "photo":
		return []string{"photo"}
	case "video", "":
		return []string{"movie", "show", "season", "episode", "clip"}
	default:
		return nil
	}
}

func (c *PlaylistSmartCmd) noFilters() bool {
	return len(nonEmptyArgs(c.Director)) == 0 &&
		len(nonEmptyArgs(c.Genre)) == 0 &&
		len(nonEmptyArgs(c.Country)) == 0 &&
		len(nonEmptyArgs(c.Collection)) == 0 &&
		len(nonEmptyArgs(c.Studio)) == 0 &&
		c.YearFrom == 0 &&
		c.YearTo == 0 &&
		!c.Unwatched
}

func (c *PlaylistSmartCmd) resolveFilters(ctx context.Context, client *plexclient.Client) (plexclient.SmartPlaylistFilters, error) {
	resolve := func(tagType string, values []string) ([]string, error) {
		values = nonEmptyArgs(values)
		if len(values) == 0 {
			return nil, nil
		}
		resolved := make([]string, 0, len(values))
		for _, value := range values {
			tagID, err := client.ResolveLibraryTagID(ctx, c.Section, tagType, value)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, tagID)
		}
		return dedupeRatingKeys(resolved), nil
	}

	directors, err := resolve("director", c.Director)
	if err != nil {
		return plexclient.SmartPlaylistFilters{}, err
	}
	genres, err := resolve("genre", c.Genre)
	if err != nil {
		return plexclient.SmartPlaylistFilters{}, err
	}
	countries, err := resolve("country", c.Country)
	if err != nil {
		return plexclient.SmartPlaylistFilters{}, err
	}
	collections, err := resolve("collection", c.Collection)
	if err != nil {
		return plexclient.SmartPlaylistFilters{}, err
	}
	studios, err := resolve("studio", c.Studio)
	if err != nil {
		return plexclient.SmartPlaylistFilters{}, err
	}

	return plexclient.SmartPlaylistFilters{
		Directors:   directors,
		Genres:      genres,
		Countries:   countries,
		Collections: collections,
		Studios:     studios,
		YearFrom:    c.YearFrom,
		YearTo:      c.YearTo,
		Unwatched:   c.Unwatched,
	}, nil
}

func (c *PlaylistSmartCmd) validate() error {
	if c.noFilters() {
		return fmt.Errorf("at least one filter is required")
	}
	if err := validateSinglePlaylistSmartArg("director", c.Director); err != nil {
		return err
	}
	if err := validateSinglePlaylistSmartArg("genre", c.Genre); err != nil {
		return err
	}
	if err := validateSinglePlaylistSmartArg("country", c.Country); err != nil {
		return err
	}
	if err := validateSinglePlaylistSmartArg("collection", c.Collection); err != nil {
		return err
	}
	if err := validateSinglePlaylistSmartArg("studio", c.Studio); err != nil {
		return err
	}
	if c.YearFrom < 0 {
		return fmt.Errorf("--year-from cannot be negative")
	}
	if c.YearTo < 0 {
		return fmt.Errorf("--year-to cannot be negative")
	}
	if c.YearFrom > 0 && c.YearTo > 0 && c.YearFrom > c.YearTo {
		return fmt.Errorf("--year-from cannot be greater than --year-to")
	}
	return nil
}

func nonEmptyArgs(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}

func validateSinglePlaylistSmartArg(name string, values []string) error {
	filtered := nonEmptyArgs(values)
	seen := make(map[string]struct{}, len(filtered))
	distinct := 0
	for _, value := range filtered {
		normalized := strings.ToLower(value)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		distinct++
	}
	if distinct > 1 {
		return fmt.Errorf("multiple --%s values are not supported", name)
	}
	return nil
}

// PlaylistDeleteCmd deletes a playlist
type PlaylistDeleteCmd struct {
	Playlist string `arg:"" help:"Playlist ID (rating key)"`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type PlaylistDeleteResult struct {
	PlaylistID string `json:"playlist_id"`
	Message    string `json:"message"`
}

func (c *PlaylistDeleteCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	err = cc.Client.DeletePlaylist(cc.Ctx, c.Playlist)
	if err != nil {
		return fmt.Errorf("failed to delete playlist: %w", err)
	}

	result := PlaylistDeleteResult{
		PlaylistID: c.Playlist,
		Message:    fmt.Sprintf("Deleted playlist %s", c.Playlist),
	}

	return c.output(u.Out(), result)
}

func (c *PlaylistDeleteCmd) output(w io.Writer, result PlaylistDeleteResult) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"PLAYLIST ID", "MESSAGE"}
	rows := [][]string{
		{result.PlaylistID, result.Message},
	}

	return formatter.Format(w, header, rows, []PlaylistDeleteResult{result})
}
