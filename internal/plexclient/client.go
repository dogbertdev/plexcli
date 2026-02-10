package plexclient

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/LukeHagar/plexgo"
	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/retry"
)

const (
	DefaultTimeout           = 120 * time.Second
	DefaultMaxRetries        = 3
	DefaultPageSize          = 100
	DefaultConcurrentWorkers = 4
	DefaultHistoryLimit      = 50
	RetryInitialIntervalMS   = 1000
	RetryMaxIntervalMS       = 30000
	RetryExponent            = 2.0
	MaxBackoffDuration       = 30 * time.Second
	DefaultClientID          = "plexcli"
	DefaultProduct           = "plexcli"
	DefaultVersion           = "1.0.0"
	DefaultPlatform          = "Go"
	DefaultUnknownTitle      = "Unknown"
	DefaultLibraryPathPrefix = "/library/sections/"
)

type Library struct {
	ID       string
	Title    *string
	Type     string
	Key      *string
	UUID     string
	Location []string
}

type Client struct {
	sdk        *plexgo.PlexAPI
	serverURL  string
	token      string
	timeout    time.Duration
	maxRetries int
	clientID   string
}

type ClientOption func(*Client)

func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.timeout = timeout
	}
}

func WithMaxRetries(maxRetries int) ClientOption {
	return func(c *Client) {
		c.maxRetries = maxRetries
	}
}

func WithClientID(clientID string) ClientOption {
	return func(c *Client) {
		c.clientID = clientID
	}
}

func NewClient(serverURL, token string, opts ...ClientOption) (*Client, error) {
	if serverURL == "" {
		return nil, fmt.Errorf("server URL is required")
	}
	if token == "" {
		return nil, fmt.Errorf("token is required")
	}

	client := &Client{
		serverURL:  serverURL,
		token:      token,
		timeout:    DefaultTimeout,
		maxRetries: DefaultMaxRetries,
		clientID:   DefaultClientID,
	}

	for _, opt := range opts {
		opt(client)
	}

	retryConfig := retry.Config{
		Strategy: "backoff",
		Backoff: &retry.BackoffStrategy{
			InitialInterval: RetryInitialIntervalMS,
			MaxInterval:     RetryMaxIntervalMS,
			Exponent:        RetryExponent,
		},
		RetryConnectionErrors: true,
	}

	sdk := plexgo.New(
		plexgo.WithServerURL(serverURL),
		plexgo.WithSecurity(token),
		plexgo.WithTimeout(client.timeout),
		plexgo.WithRetryConfig(retryConfig),
		plexgo.WithClientIdentifier(client.clientID),
		plexgo.WithProduct(DefaultProduct),
		plexgo.WithVersion(DefaultVersion),
		plexgo.WithPlatform(DefaultPlatform),
	)

	client.sdk = sdk

	return client, nil
}

type PlexError struct {
	Op      string
	Section string
	Err     error
}

func (e *PlexError) Error() string {
	if e.Section != "" {
		return fmt.Sprintf("plex %s failed for section %s: %v", e.Op, e.Section, e.Err)
	}
	return fmt.Sprintf("plex %s failed: %v", e.Op, e.Err)
}

func (e *PlexError) Unwrap() error {
	return e.Err
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(interface{ Temporary() bool }); ok {
		return true
	}
	if _, ok := err.(interface{ Timeout() bool }); ok {
		return true
	}
	return false
}

func (c *Client) executeWithRetry(ctx context.Context, op string, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s cancelled: %w", op, ctx.Err())
		default:
		}

		if err := fn(); err != nil {
			lastErr = err
			if !isRetryableError(err) || attempt == c.maxRetries {
				return err
			}

			backoff := time.Duration(math.Pow(RetryExponent, float64(attempt))) * time.Second
			if backoff > MaxBackoffDuration {
				backoff = MaxBackoffDuration
			}

			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("%s cancelled during retry: %w", op, ctx.Err())
			case <-timer.C:
			}
			continue
		}

		return nil
	}

	return lastErr
}

type rawMediaMetadata struct {
	RatingKey        string  `json:"ratingKey"`
	Key              string  `json:"key"`
	Title            string  `json:"title"`
	Type             string  `json:"type"`
	Year             *int    `json:"year"`
	AddedAt          int64   `json:"addedAt"`
	ViewCount        *int    `json:"viewCount"`
	GrandparentTitle *string `json:"grandparentTitle"`
	EditionTitle     *string `json:"editionTitle"`
	Media            []struct {
		Part []struct {
			File   *string `json:"file"`
			Size   *int64  `json:"size"`
			Stream []struct {
				StreamType   int     `json:"streamType"`
				Language     *string `json:"language"`
				LanguageCode *string `json:"languageCode"`
				Codec        *string `json:"codec"`
				Channels     *int    `json:"channels"`
			} `json:"Stream"`
		} `json:"Part"`
		VideoResolution *string `json:"videoResolution"`
		VideoCodec      *string `json:"videoCodec"`
		AudioCodec      *string `json:"audioCodec"`
		AudioChannels   *int    `json:"audioChannels"`
	} `json:"Media"`
	Summary *string `json:"summary"`
	Thumb   *string `json:"thumb"`
}

type rawLibraryItemsResponse struct {
	MediaContainer struct {
		Metadata []rawMediaMetadata `json:"Metadata"`
	} `json:"MediaContainer"`
}

func (c *Client) GetAllLibraryItems(ctx context.Context, sectionID string) ([]*components.Metadata, error) {
	if sectionID == "" {
		return nil, &PlexError{
			Op:  "GetAllLibraryItems",
			Err: fmt.Errorf("section ID is required"),
		}
	}

	var allItems []*components.Metadata

	err := c.executeWithRetry(ctx, "GetAllLibraryItems", func() error {
		url := fmt.Sprintf("%s/library/sections/%s/all?X-Plex-Container-Start=0&X-Plex-Container-Size=1000&X-Plex-Token=%s", c.serverURL, sectionID, c.token)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var rawResp rawLibraryItemsResponse
		if err := json.Unmarshal(body, &rawResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		for _, item := range rawResp.MediaContainer.Metadata {
			meta := convertRawToMetadata(item)
			allItems = append(allItems, meta)
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:      "GetAllLibraryItems",
			Section: sectionID,
			Err:     err,
		}
	}

	return allItems, nil
}

// GetItemMetadata fetches detailed metadata for a single item including streams
func (c *Client) GetItemMetadata(ctx context.Context, ratingKey string) (*components.Metadata, error) {
	var result *components.Metadata

	err := c.executeWithRetry(ctx, "GetItemMetadata", func() error {
		url := fmt.Sprintf("%s/library/metadata/%s?X-Plex-Token=%s", c.serverURL, ratingKey, c.token)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var rawResp rawLibraryItemsResponse
		if err := json.Unmarshal(body, &rawResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if len(rawResp.MediaContainer.Metadata) > 0 {
			result = convertRawToMetadata(rawResp.MediaContainer.Metadata[0])
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetItemMetadata",
			Err: err,
		}
	}

	return result, nil
}

func convertRawToMetadata(raw rawMediaMetadata) *components.Metadata {
	ratingKey := raw.RatingKey
	key := raw.Key

	meta := &components.Metadata{
		RatingKey:        &ratingKey,
		Key:              key,
		Title:            raw.Title,
		Type:             raw.Type,
		AddedAt:          raw.AddedAt,
		Year:             raw.Year,
		ViewCount:        raw.ViewCount,
		Summary:          raw.Summary,
		Thumb:            raw.Thumb,
		GrandparentTitle: raw.GrandparentTitle,
	}

	if raw.EditionTitle != nil {
		meta.AdditionalProperties = map[string]any{
			"editionTitle": *raw.EditionTitle,
		}
	}

	if len(raw.Media) > 0 {
		media := make([]components.Media, len(raw.Media))
		for i, m := range raw.Media {
			media[i] = components.Media{
				VideoResolution: m.VideoResolution,
				VideoCodec:      m.VideoCodec,
				AudioCodec:      m.AudioCodec,
				AudioChannels:   m.AudioChannels,
			}
			if len(m.Part) > 0 {
				parts := make([]components.Part, len(m.Part))
				for j, p := range m.Part {
					parts[j] = components.Part{
						File: p.File,
						Size: p.Size,
					}
					if len(p.Stream) > 0 {
						streams := make([]components.Stream, len(p.Stream))
						for k, s := range p.Stream {
							codec := ""
							if s.Codec != nil {
								codec = *s.Codec
							}
							streams[k] = components.Stream{
								StreamType:   components.StreamType(s.StreamType),
								Language:     s.Language,
								LanguageCode: s.LanguageCode,
								Codec:        codec,
								Channels:     s.Channels,
							}
						}
						parts[j].Stream = streams
					}
				}
				media[i].Part = parts
			}
		}
		meta.Media = media
	}

	return meta
}

func (c *Client) GetLibraryItemsConcurrent(ctx context.Context, sectionIDs []string, maxConcurrent int) ([]*components.Metadata, error) {
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultConcurrentWorkers
	}

	if len(sectionIDs) == 0 {
		return []*components.Metadata{}, nil
	}

	semaphore := make(chan struct{}, maxConcurrent)

	type sectionResult struct {
		items []*components.Metadata
		err   error
		id    string
	}
	results := make(chan sectionResult, len(sectionIDs))

	var wg sync.WaitGroup

	for _, sectionID := range sectionIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			select {
			case semaphore <- struct{}{}:
				items, err := c.GetAllLibraryItems(ctx, id)
				<-semaphore
				results <- sectionResult{items: items, err: err, id: id}
			case <-ctx.Done():
				results <- sectionResult{err: ctx.Err(), id: id}
			}
		}(sectionID)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var allItems []*components.Metadata
	var mu sync.Mutex
	var firstErr error

	for result := range results {
		if result.err != nil {
			if firstErr == nil {
				firstErr = &PlexError{
					Op:      "GetLibraryItemsConcurrent",
					Section: result.id,
					Err:     result.err,
				}
			}
		} else {
			mu.Lock()
			allItems = append(allItems, result.items...)
			mu.Unlock()
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}

	return allItems, nil
}

type HistoryItem struct {
	Title            string
	GrandparentTitle string
	Type             string
	RatingKey        *string
	LibrarySectionID *string
	ViewedAt         int64
	Thumb            *string
}

type rawHistoryMetadata struct {
	Title            string  `json:"title"`
	GrandparentTitle string  `json:"grandparentTitle"`
	Type             string  `json:"type"`
	RatingKey        *string `json:"ratingKey"`
	LibrarySectionID *string `json:"librarySectionID"`
	ViewedAt         *int64  `json:"viewedAt"`
	Thumb            *string `json:"thumb"`
}

type rawHistoryContainer struct {
	Metadata []rawHistoryMetadata `json:"Metadata"`
}

type rawHistoryResponse struct {
	MediaContainer rawHistoryContainer `json:"MediaContainer"`
}

func (c *Client) GetHistory(ctx context.Context, limit int) ([]HistoryItem, error) {
	if limit <= 0 {
		limit = DefaultHistoryLimit
	}

	var historyItems []HistoryItem

	err := c.executeWithRetry(ctx, "GetHistory", func() error {
		url := fmt.Sprintf("%s/status/sessions/history/all?sort=viewedAt:desc&X-Plex-Token=%s", c.serverURL, c.token)
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var rawResp rawHistoryResponse
		if err := json.Unmarshal(body, &rawResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		for _, hist := range rawResp.MediaContainer.Metadata {
			item := HistoryItem{
				Title:            hist.Title,
				GrandparentTitle: hist.GrandparentTitle,
				Type:             hist.Type,
				RatingKey:        hist.RatingKey,
				LibrarySectionID: hist.LibrarySectionID,
				Thumb:            hist.Thumb,
			}
			if hist.ViewedAt != nil {
				item.ViewedAt = *hist.ViewedAt
			}
			historyItems = append(historyItems, item)

			if len(historyItems) >= limit {
				break
			}
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetHistory",
			Err: err,
		}
	}

	return historyItems, nil
}

// rawSection is a minimal struct to parse the Plex sections response
type rawSection struct {
	UUID  string `xml:"uuid,attr"`
	Title string `xml:"title,attr"`
	Type  string `xml:"type,attr"`
	Key   string `xml:"key,attr"`
}

type rawSectionsResponse struct {
	XMLName   xml.Name     `xml:"MediaContainer"`
	Directory []rawSection `xml:"Directory"`
}

func (c *Client) GetSections(ctx context.Context) ([]Library, error) {
	var libraries []Library

	err := c.executeWithRetry(ctx, "GetSections", func() error {
		// Make raw HTTP request to avoid SDK unmarshaling bug with 'hidden' field
		url := c.serverURL + "/library/sections"
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("X-Plex-Token", c.token)

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var rawResp rawSectionsResponse
		if err := xml.Unmarshal(body, &rawResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if rawResp.Directory == nil {
			return nil
		}

		for _, section := range rawResp.Directory {
			title := section.Title
			if title == "" {
				title = "Unknown"
			}
			key := section.Key
			if key == "" {
				key = DefaultLibraryPathPrefix + section.UUID
			}

			lib := Library{
				ID:    section.Key,
				Title: &title,
				Type:  section.Type,
				Key:   &key,
				UUID:  section.UUID,
			}

			libraries = append(libraries, lib)
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetSections",
			Err: err,
		}
	}

	return libraries, nil
}

func (c *Client) GetServerURL() string {
	return c.serverURL
}

func (c *Client) GetTimeout() time.Duration {
	return c.timeout
}

func (c *Client) GetMaxRetries() int {
	return c.maxRetries
}

// SearchResult represents a single search result item
type SearchResult struct {
	RatingKey        string  `json:"ratingKey"`
	Key              string  `json:"key"`
	Title            string  `json:"title"`
	Type             string  `json:"type"`
	Year             *int    `json:"year"`
	GrandparentTitle *string `json:"grandparentTitle"`
	ParentTitle      *string `json:"parentTitle"`
	ParentIndex      *int    `json:"parentIndex"`
	Index            *int    `json:"index"`
	Thumb            *string `json:"thumb"`
}

type rawSearchHub struct {
	Type     string `json:"type"`
	HubKey   string `json:"hubKey"`
	Title    string `json:"title"`
	Size     int    `json:"size"`
	Metadata []struct {
		RatingKey        string  `json:"ratingKey"`
		Key              string  `json:"key"`
		Title            string  `json:"title"`
		Type             string  `json:"type"`
		Year             *int    `json:"year"`
		GrandparentTitle *string `json:"grandparentTitle"`
		ParentTitle      *string `json:"parentTitle"`
		ParentIndex      *int    `json:"parentIndex"`
		Index            *int    `json:"index"`
		Thumb            *string `json:"thumb"`
	} `json:"Metadata"`
}

type rawSearchResponse struct {
	MediaContainer struct {
		Hub []rawSearchHub `json:"Hub"`
	} `json:"MediaContainer"`
}

// SearchLibrary searches the Plex library for items matching the query
func (c *Client) SearchLibrary(ctx context.Context, query string, sectionID *string, limit int) ([]SearchResult, error) {
	if query == "" {
		return nil, &PlexError{
			Op:  "SearchLibrary",
			Err: fmt.Errorf("query is required"),
		}
	}

	if limit <= 0 {
		limit = 50
	}

	var results []SearchResult

	err := c.executeWithRetry(ctx, "SearchLibrary", func() error {
		urlStr := fmt.Sprintf("%s/hubs/search?query=%s&limit=%d&X-Plex-Token=%s",
			c.serverURL, url.QueryEscape(query), limit, c.token)
		if sectionID != nil && *sectionID != "" {
			urlStr += fmt.Sprintf("&sectionId=%s", *sectionID)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var rawResp rawSearchResponse
		if err := json.Unmarshal(body, &rawResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		for _, hub := range rawResp.MediaContainer.Hub {
			for _, item := range hub.Metadata {
				results = append(results, SearchResult{
					RatingKey:        item.RatingKey,
					Key:              item.Key,
					Title:            item.Title,
					Type:             item.Type,
					Year:             item.Year,
					GrandparentTitle: item.GrandparentTitle,
					ParentTitle:      item.ParentTitle,
					ParentIndex:      item.ParentIndex,
					Index:            item.Index,
					Thumb:            item.Thumb,
				})
			}
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "SearchLibrary",
			Err: err,
		}
	}

	return results, nil
}

// PlaylistInfo represents a playlist
type PlaylistInfo struct {
	RatingKey    string `json:"ratingKey"`
	Key          string `json:"key"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	PlaylistType string `json:"playlistType"`
	LeafCount    int    `json:"leafCount"`
	Smart        bool   `json:"smart"`
	Duration     int64  `json:"duration"`
}

type rawPlaylistMetadata struct {
	RatingKey    string `json:"ratingKey"`
	Key          string `json:"key"`
	Title        string `json:"title"`
	Type         string `json:"type"`
	PlaylistType string `json:"playlistType"`
	LeafCount    int    `json:"leafCount"`
	Smart        bool   `json:"smart"`
	Duration     int64  `json:"duration"`
}

type rawPlaylistsResponse struct {
	MediaContainer struct {
		Metadata []rawPlaylistMetadata `json:"Metadata"`
	} `json:"MediaContainer"`
}

// ListPlaylists returns all playlists
func (c *Client) ListPlaylists(ctx context.Context) ([]PlaylistInfo, error) {
	var playlists []PlaylistInfo

	err := c.executeWithRetry(ctx, "ListPlaylists", func() error {
		urlStr := fmt.Sprintf("%s/playlists?X-Plex-Token=%s", c.serverURL, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var rawResp rawPlaylistsResponse
		if err := json.Unmarshal(body, &rawResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		for _, p := range rawResp.MediaContainer.Metadata {
			playlists = append(playlists, PlaylistInfo{
				RatingKey:    p.RatingKey,
				Key:          p.Key,
				Title:        p.Title,
				Type:         p.Type,
				PlaylistType: p.PlaylistType,
				LeafCount:    p.LeafCount,
				Smart:        p.Smart,
				Duration:     p.Duration,
			})
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "ListPlaylists",
			Err: err,
		}
	}

	return playlists, nil
}

// rawServerIdentity is used to parse the server identity response
type rawServerIdentity struct {
	MediaContainer struct {
		MachineIdentifier string `json:"machineIdentifier"`
	} `json:"MediaContainer"`
}

// GetServerUUID returns the server's machine identifier (UUID)
func (c *Client) GetServerUUID(ctx context.Context) (string, error) {
	var uuid string

	err := c.executeWithRetry(ctx, "GetServerUUID", func() error {
		urlStr := fmt.Sprintf("%s/identity?X-Plex-Token=%s", c.serverURL, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var rawResp rawServerIdentity
		if err := json.Unmarshal(body, &rawResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		uuid = rawResp.MediaContainer.MachineIdentifier
		return nil
	})

	if err != nil {
		return "", &PlexError{
			Op:  "GetServerUUID",
			Err: err,
		}
	}

	return uuid, nil
}

// CreatePlaylist creates a new playlist with the given name and optional initial items
// ratingKeys should be a slice of rating keys to add to the playlist
func (c *Client) CreatePlaylist(ctx context.Context, title string, playlistType string, ratingKeys []string) (*PlaylistInfo, error) {
	if title == "" {
		return nil, &PlexError{
			Op:  "CreatePlaylist",
			Err: fmt.Errorf("title is required"),
		}
	}

	if playlistType == "" {
		playlistType = "video"
	}

	// Get server UUID for constructing the URI
	serverUUID, err := c.GetServerUUID(ctx)
	if err != nil {
		return nil, &PlexError{
			Op:  "CreatePlaylist",
			Err: fmt.Errorf("failed to get server UUID: %w", err),
		}
	}

	var playlist *PlaylistInfo

	err = c.executeWithRetry(ctx, "CreatePlaylist", func() error {
		urlStr := fmt.Sprintf("%s/playlists?title=%s&type=%s&smart=0&X-Plex-Token=%s",
			c.serverURL, url.QueryEscape(title), playlistType, c.token)

		// If we have rating keys, construct the URI
		if len(ratingKeys) > 0 {
			// URI format: server://{uuid}/com.plexapp.plugins.library/library/metadata/{key1},{key2},...
			uri := fmt.Sprintf("server://%s/com.plexapp.plugins.library/library/metadata/%s",
				serverUUID, joinStrings(ratingKeys, ","))
			urlStr += fmt.Sprintf("&uri=%s", url.QueryEscape(uri))
		}

		req, err := http.NewRequestWithContext(ctx, "POST", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var rawResp rawPlaylistsResponse
		if err := json.Unmarshal(body, &rawResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if len(rawResp.MediaContainer.Metadata) > 0 {
			p := rawResp.MediaContainer.Metadata[0]
			playlist = &PlaylistInfo{
				RatingKey:    p.RatingKey,
				Key:          p.Key,
				Title:        p.Title,
				Type:         p.Type,
				PlaylistType: p.PlaylistType,
				LeafCount:    p.LeafCount,
				Smart:        p.Smart,
				Duration:     p.Duration,
			}
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "CreatePlaylist",
			Err: err,
		}
	}

	return playlist, nil
}

// AddToPlaylist adds items to an existing playlist
func (c *Client) AddToPlaylist(ctx context.Context, playlistID string, ratingKeys []string) error {
	if playlistID == "" {
		return &PlexError{
			Op:  "AddToPlaylist",
			Err: fmt.Errorf("playlist ID is required"),
		}
	}

	if len(ratingKeys) == 0 {
		return &PlexError{
			Op:  "AddToPlaylist",
			Err: fmt.Errorf("at least one rating key is required"),
		}
	}

	// Get server UUID for constructing the URI
	serverUUID, err := c.GetServerUUID(ctx)
	if err != nil {
		return &PlexError{
			Op:  "AddToPlaylist",
			Err: fmt.Errorf("failed to get server UUID: %w", err),
		}
	}

	err = c.executeWithRetry(ctx, "AddToPlaylist", func() error {
		// URI format: server://{uuid}/com.plexapp.plugins.library/library/metadata/{key1},{key2},...
		uri := fmt.Sprintf("server://%s/com.plexapp.plugins.library/library/metadata/%s",
			serverUUID, joinStrings(ratingKeys, ","))

		urlStr := fmt.Sprintf("%s/playlists/%s/items?uri=%s&X-Plex-Token=%s",
			c.serverURL, playlistID, url.QueryEscape(uri), c.token)

		req, err := http.NewRequestWithContext(ctx, "PUT", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		}

		return nil
	})

	if err != nil {
		return &PlexError{
			Op:  "AddToPlaylist",
			Err: err,
		}
	}

	return nil
}

// GetPlaylistItems returns the items in a playlist
func (c *Client) GetPlaylistItems(ctx context.Context, playlistID string) ([]SearchResult, error) {
	if playlistID == "" {
		return nil, &PlexError{
			Op:  "GetPlaylistItems",
			Err: fmt.Errorf("playlist ID is required"),
		}
	}

	var items []SearchResult

	err := c.executeWithRetry(ctx, "GetPlaylistItems", func() error {
		urlStr := fmt.Sprintf("%s/playlists/%s/items?X-Plex-Token=%s",
			c.serverURL, playlistID, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		// Playlist items response has Metadata directly in MediaContainer (not in Hub)
		var altResp struct {
			MediaContainer struct {
				Metadata []struct {
					RatingKey        string  `json:"ratingKey"`
					Key              string  `json:"key"`
					Title            string  `json:"title"`
					Type             string  `json:"type"`
					Year             *int    `json:"year"`
					GrandparentTitle *string `json:"grandparentTitle"`
					ParentTitle      *string `json:"parentTitle"`
					ParentIndex      *int    `json:"parentIndex"`
					Index            *int    `json:"index"`
					Thumb            *string `json:"thumb"`
				} `json:"Metadata"`
			} `json:"MediaContainer"`
		}
		if err := json.Unmarshal(body, &altResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
		for _, item := range altResp.MediaContainer.Metadata {
			items = append(items, SearchResult{
				RatingKey:        item.RatingKey,
				Key:              item.Key,
				Title:            item.Title,
				Type:             item.Type,
				Year:             item.Year,
				GrandparentTitle: item.GrandparentTitle,
				ParentTitle:      item.ParentTitle,
				ParentIndex:      item.ParentIndex,
				Index:            item.Index,
				Thumb:            item.Thumb,
			})
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetPlaylistItems",
			Err: err,
		}
	}

	return items, nil
}

// joinStrings joins a slice of strings with a separator
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// Episode represents a TV episode with season/episode info
type Episode struct {
	RatingKey   string `json:"ratingKey"`
	Title       string `json:"title"`
	ShowTitle   string `json:"showTitle"`
	SeasonNum   int    `json:"season"`
	EpisodeNum  int    `json:"episode"`
	SeasonTitle string `json:"seasonTitle,omitempty"`
}

// FindShow searches for a show by name and returns its rating key
func (c *Client) FindShow(ctx context.Context, showName string) (string, string, error) {
	results, err := c.SearchLibrary(ctx, showName, nil, 10)
	if err != nil {
		return "", "", err
	}

	for _, r := range results {
		if r.Type == "show" {
			return r.RatingKey, r.Title, nil
		}
	}

	return "", "", &PlexError{
		Op:  "FindShow",
		Err: fmt.Errorf("show not found: %s", showName),
	}
}

// GetShowEpisodes returns all episodes for a show given its rating key
func (c *Client) GetShowEpisodes(ctx context.Context, showRatingKey string) ([]Episode, error) {
	if showRatingKey == "" {
		return nil, &PlexError{
			Op:  "GetShowEpisodes",
			Err: fmt.Errorf("show rating key is required"),
		}
	}

	var episodes []Episode

	err := c.executeWithRetry(ctx, "GetShowEpisodes", func() error {
		urlStr := fmt.Sprintf("%s/library/metadata/%s/allLeaves?X-Plex-Token=%s",
			c.serverURL, showRatingKey, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		httpClient := &http.Client{Timeout: c.timeout}
		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var rawResp struct {
			MediaContainer struct {
				Metadata []struct {
					RatingKey        string `json:"ratingKey"`
					Title            string `json:"title"`
					GrandparentTitle string `json:"grandparentTitle"`
					ParentTitle      string `json:"parentTitle"`
					ParentIndex      int    `json:"parentIndex"`
					Index            int    `json:"index"`
				} `json:"Metadata"`
			} `json:"MediaContainer"`
		}

		if err := json.Unmarshal(body, &rawResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		for _, ep := range rawResp.MediaContainer.Metadata {
			episodes = append(episodes, Episode{
				RatingKey:   ep.RatingKey,
				Title:       ep.Title,
				ShowTitle:   ep.GrandparentTitle,
				SeasonNum:   ep.ParentIndex,
				EpisodeNum:  ep.Index,
				SeasonTitle: ep.ParentTitle,
			})
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetShowEpisodes",
			Err: err,
		}
	}

	return episodes, nil
}
