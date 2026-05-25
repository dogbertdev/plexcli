package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
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
	Sort   PlaylistSortCmd   `cmd:"" help:"Sort a regular playlist"`
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
	Name      string   `arg:"" help:"Playlist name"`
	Items     []string `arg:"" optional:"" help:"Rating keys to add"`
	Queries   []string `help:"Resolve and append items by title" name:"query"`
	FromFile  []string `help:"Read rating keys from a file (whitespace- or comma-separated)" name:"from-file" type:"path"`
	FromStdin bool     `help:"Read rating keys from stdin" name:"from-stdin" default:"false"`
	DryRun    bool     `help:"Preview resolved items without creating the playlist" name:"dry-run" default:"false"`
	Type      string   `help:"Playlist type: video, audio, photo" default:"video" enum:"video,audio,photo"`
	Output    string   `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
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

	if c.DryRun {
		previewItems, previewErr := previewPlaylistItems(cc.Client, cc.Ctx, items)
		if previewErr != nil {
			return previewErr
		}
		return outputSearchItems(u.Out(), c.Output, previewItems)
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
	RatingKey      string `json:"rating_key"`
	PlaylistItemID string `json:"playlist_item_id,omitempty"`
	Title          string `json:"title"`
	Type           string `json:"type"`
	Year           int    `json:"year,omitempty"`
	Show           string `json:"show,omitempty"`
	Season         int    `json:"season,omitempty"`
	Episode        int    `json:"episode,omitempty"`
}

type PlaylistSortCmd struct {
	Playlist string `arg:"" help:"Playlist ID (rating key)"`
	By       string `help:"Sort field" default:"year" enum:"year,title"`
	Order    string `help:"Sort order" default:"asc" enum:"asc,desc"`
	DryRun   bool   `help:"Preview the sorted order without modifying Plex" name:"dry-run" default:"false"`
	Output   string `help:"Output format: table, json, or tsv" default:"table" enum:"table,json,tsv"`
}

type PlaylistSortResult struct {
	PlaylistID string `json:"playlist_id"`
	SortBy     string `json:"sort_by"`
	Order      string `json:"order"`
	ItemCount  int    `json:"item_count"`
	Moves      int    `json:"moves"`
	Message    string `json:"message"`
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

	return c.output(u.Out(), playlistShowItemsFromResults(items))
}

func (c *PlaylistSortCmd) Run(ctx *kong.Context, u *ui.UI, cfg *config.Config) error {
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

	sortedItems := sortPlaylistItems(items, c.By, c.Order)
	if c.DryRun {
		return outputPlaylistSearchResults(u.Out(), c.Output, sortedItems)
	}

	movePlan, err := planPlaylistMoves(items, sortedItems)
	if err != nil {
		return err
	}
	for _, move := range movePlan {
		if err := cc.Client.MovePlaylistItem(cc.Ctx, c.Playlist, move.Item.PlaylistItemID, move.AfterPlaylistItemID); err != nil {
			return fmt.Errorf("failed to move playlist item %s (%s): %w", move.Item.Title, move.Item.RatingKey, err)
		}
	}

	message := fmt.Sprintf("Sorted %d item(s) in playlist %s by %s %s", len(sortedItems), c.Playlist, c.By, c.Order)
	if len(movePlan) == 0 {
		message = fmt.Sprintf("Playlist %s already sorted by %s %s", c.Playlist, c.By, c.Order)
	}
	result := PlaylistSortResult{
		PlaylistID: c.Playlist,
		SortBy:     c.By,
		Order:      c.Order,
		ItemCount:  len(sortedItems),
		Moves:      len(movePlan),
		Message:    message,
	}

	return c.output(u.Out(), result)
}

func (c *PlaylistSortCmd) output(w io.Writer, result PlaylistSortResult) error {
	formatter := outfmt.NewFormatter(outfmt.Format(c.Output))

	header := []string{"PLAYLIST ID", "SORT BY", "ORDER", "ITEMS", "MOVES", "MESSAGE"}
	rows := [][]string{
		{result.PlaylistID, result.SortBy, result.Order, fmt.Sprintf("%d", result.ItemCount), fmt.Sprintf("%d", result.Moves), result.Message},
	}

	return formatter.Format(w, header, rows, []PlaylistSortResult{result})
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

func outputPlaylistSearchResults(w io.Writer, format string, items []plexclient.SearchResult) error {
	cmd := PlaylistShowCmd{Output: format}
	return cmd.output(w, playlistShowItemsFromResults(items))
}

func sortPlaylistItems(items []plexclient.SearchResult, by string, order string) []plexclient.SearchResult {
	sortedItems := append([]plexclient.SearchResult{}, items...)
	desc := order == "desc"

	sort.SliceStable(sortedItems, func(i, j int) bool {
		cmp := comparePlaylistItems(sortedItems[i], sortedItems[j], by)
		if cmp == 0 {
			return false
		}
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})

	return sortedItems
}

type playlistMove struct {
	Item                plexclient.SearchResult
	AfterPlaylistItemID string
}

func playlistShowItemsFromResults(items []plexclient.SearchResult) []PlaylistShowItem {
	outputItems := make([]PlaylistShowItem, 0, len(items))
	for _, item := range items {
		out := PlaylistShowItem{
			RatingKey:      item.RatingKey,
			PlaylistItemID: item.PlaylistItemID,
			Title:          item.Title,
			Type:           item.Type,
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
	return outputItems
}

func planPlaylistMoves(currentItems []plexclient.SearchResult, desiredItems []plexclient.SearchResult) ([]playlistMove, error) {
	workingIDs := make([]string, len(currentItems))
	for i, item := range currentItems {
		if item.PlaylistItemID == "" {
			return nil, fmt.Errorf("playlist item ID missing for %s (%s)", item.Title, item.RatingKey)
		}
		workingIDs[i] = item.PlaylistItemID
	}

	moves := make([]playlistMove, 0, len(desiredItems))
	for i, item := range desiredItems {
		if item.PlaylistItemID == "" {
			return nil, fmt.Errorf("playlist item ID missing for %s (%s)", item.Title, item.RatingKey)
		}
		if i < len(workingIDs) && workingIDs[i] == item.PlaylistItemID {
			continue
		}

		currentIndex := indexPlaylistItemID(workingIDs, item.PlaylistItemID)
		if currentIndex == -1 {
			return nil, fmt.Errorf("playlist item ID missing from current order for %s (%s)", item.Title, item.RatingKey)
		}

		afterPlaylistItemID := ""
		if i > 0 {
			afterPlaylistItemID = desiredItems[i-1].PlaylistItemID
		}
		moves = append(moves, playlistMove{
			Item:                item,
			AfterPlaylistItemID: afterPlaylistItemID,
		})
		workingIDs = movePlaylistItemID(workingIDs, currentIndex, i)
	}

	return moves, nil
}

func indexPlaylistItemID(ids []string, target string) int {
	for i, id := range ids {
		if id == target {
			return i
		}
	}
	return -1
}

func movePlaylistItemID(ids []string, from, to int) []string {
	if from == to {
		return ids
	}

	moved := ids[from]
	copy(ids[from:], ids[from+1:])
	ids = ids[:len(ids)-1]

	ids = append(ids, "")
	copy(ids[to+1:], ids[to:])
	ids[to] = moved
	return ids
}

func comparePlaylistItems(left plexclient.SearchResult, right plexclient.SearchResult, by string) int {
	switch by {
	case "year":
		leftYear, leftHasYear := playlistItemYear(left)
		rightYear, rightHasYear := playlistItemYear(right)
		switch {
		case leftHasYear && rightHasYear && leftYear != rightYear:
			return compareInts(leftYear, rightYear)
		case leftHasYear != rightHasYear:
			if leftHasYear {
				return -1
			}
			return 1
		}
	}

	return strings.Compare(strings.ToLower(left.Title), strings.ToLower(right.Title))
}

func playlistItemYear(item plexclient.SearchResult) (int, bool) {
	if item.Year == nil || *item.Year <= 0 {
		return 0, false
	}
	return *item.Year, true
}

func compareInts(left int, right int) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func (c *PlaylistCreateCmd) resolveItems(u *ui.UI, client *plexclient.Client, ctx context.Context) ([]string, error) {
	items, err := c.inputItems()
	if err != nil {
		return nil, err
	}
	return resolvePlaylistItems(u, client, ctx, c.Output, items, c.Queries, SearchResolveOptions{
		Limit:        plexclient.DefaultSearchLimit,
		AllowedTypes: playlistAllowedSearchTypes(c.Type),
	})
}

func (c *PlaylistCreateCmd) inputItems() ([]string, error) {
	items := append([]string{}, c.Items...)

	for _, path := range c.FromFile {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read --from-file %s: %w", path, err)
		}
		items = append(items, parsePlaylistItemTokens(string(data))...)
	}

	if c.FromStdin {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read --from-stdin: %w", err)
		}
		items = append(items, parsePlaylistItemTokens(string(data))...)
	}

	return items, nil
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
		return nil, fmt.Errorf("at least one item rating key, --query, --from-file, or --from-stdin is required")
	}

	return resolved, nil
}

func parsePlaylistItemTokens(input string) []string {
	return nonEmptyArgs(strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}))
}

func previewPlaylistItems(client *plexclient.Client, ctx context.Context, ratingKeys []string) ([]SearchItem, error) {
	items := make([]SearchItem, 0, len(ratingKeys))
	for _, ratingKey := range ratingKeys {
		result, err := client.GetMetadataSearchResult(ctx, ratingKey)
		if err != nil {
			return nil, fmt.Errorf("failed to inspect playlist item %s: %w", ratingKey, err)
		}
		items = append(items, searchItemsFromResults([]plexclient.SearchResult{result})...)
	}
	return items, nil
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
