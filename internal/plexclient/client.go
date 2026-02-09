package plexclient

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/LukeHagar/plexgo"
	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
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

func (c *Client) GetItemMetadata(ctx context.Context, ratingKey string) (*components.Metadata, error) {
	if ratingKey == "" {
		return nil, &PlexError{
			Op:  "GetItemMetadata",
			Err: fmt.Errorf("rating key is required"),
		}
	}

	var metadata *components.Metadata

	err := c.executeWithRetry(ctx, "GetItemMetadata", func() error {
		req := operations.GetMetadataItemRequest{
			Ids: []string{ratingKey},
		}

		resp, err := c.sdk.Content.GetMetadataItem(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to get metadata: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		if resp.MediaContainerWithMetadata == nil ||
			resp.MediaContainerWithMetadata.MediaContainer == nil ||
			len(resp.MediaContainerWithMetadata.MediaContainer.Metadata) == 0 {
			return fmt.Errorf("no metadata found for rating key: %s", ratingKey)
		}

		meta := resp.MediaContainerWithMetadata.MediaContainer.Metadata[0]
		metadata = &meta

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetItemMetadata",
			Err: err,
		}
	}

	return metadata, nil
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
