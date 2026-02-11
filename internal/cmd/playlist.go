package cmd

import (
	"fmt"
	"io"

	"github.com/alecthomas/kong"
	"github.com/user/plexcli/internal/config"
	"github.com/user/plexcli/internal/outfmt"
	"github.com/user/plexcli/internal/ui"
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
	Name   string   `arg:"" help:"Playlist name"`
	Items  []string `arg:"" help:"Rating keys to add (at least one required)"`
	Type   string   `help:"Playlist type: video, audio, photo" default:"video" enum:"video,audio,photo"`
	Output string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
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

	playlist, err := cc.Client.CreatePlaylist(cc.Ctx, c.Name, c.Type, c.Items)
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
	Name     string `arg:"" help:"Playlist name"`
	Section  string `help:"Library section ID" short:"s" required:""`
	Director string `help:"Filter by director name" short:"d"`
	Type     string `help:"Playlist type: video, audio" default:"video" enum:"video,audio"`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

func (c *PlaylistSmartCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if c.Director == "" {
		return fmt.Errorf("at least one filter is required (e.g., --director)")
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	directorID, err := cc.Client.GetDirectorID(cc.Ctx, c.Section, c.Director)
	if err != nil {
		return fmt.Errorf("failed to find director: %w", err)
	}

	playlist, err := cc.Client.CreateSmartPlaylist(cc.Ctx, c.Name, c.Type, c.Section, "director", directorID)
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
	Items    []string `arg:"" help:"Rating keys to add"`
	Output   string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type PlaylistAddResult struct {
	PlaylistID string `json:"playlist_id"`
	ItemsAdded int    `json:"items_added"`
	Message    string `json:"message"`
}

func (c *PlaylistAddCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
	if len(c.Items) == 0 {
		return fmt.Errorf("at least one item rating key is required")
	}

	cc, err := NewClientContext(cfg)
	if err != nil {
		return err
	}
	defer cc.Cancel()

	err = cc.Client.AddToPlaylist(cc.Ctx, c.Playlist, c.Items)
	if err != nil {
		return fmt.Errorf("failed to add items to playlist: %w", err)
	}

	result := PlaylistAddResult{
		PlaylistID: c.Playlist,
		ItemsAdded: len(c.Items),
		Message:    fmt.Sprintf("Added %d item(s) to playlist %s", len(c.Items), c.Playlist),
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
