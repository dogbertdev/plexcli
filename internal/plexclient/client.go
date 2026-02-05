package plexclient

import (
	"context"
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
	DefaultTimeout    = 120 * time.Second
	DefaultMaxRetries = 3
	DefaultPageSize   = 100
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
		clientID:   "plexcli",
	}

	for _, opt := range opts {
		opt(client)
	}

	retryConfig := retry.Config{
		Strategy: "backoff",
		Backoff: &retry.BackoffStrategy{
			InitialInterval: 1000,
			MaxInterval:     30000,
			Exponent:        2.0,
		},
		RetryConnectionErrors: true,
	}

	sdk := plexgo.New(
		plexgo.WithServerURL(serverURL),
		plexgo.WithSecurity(token),
		plexgo.WithTimeout(client.timeout),
		plexgo.WithRetryConfig(retryConfig),
		plexgo.WithClientIdentifier(client.clientID),
		plexgo.WithProduct("plexcli"),
		plexgo.WithVersion("1.0.0"),
		plexgo.WithPlatform("Go"),
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

			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
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

func (c *Client) GetAllLibraryItems(ctx context.Context, sectionID string) ([]*components.Metadata, error) {
	var allItems []*components.Metadata

	err := c.executeWithRetry(ctx, "GetAllLibraryItems", func() error {
		req := operations.GetLibraryItemsRequest{}

		resp, err := c.sdk.Library.GetLibraryItems(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to get library items: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		if resp.MediaContainerWithMetadata == nil ||
			resp.MediaContainerWithMetadata.MediaContainer == nil ||
			resp.MediaContainerWithMetadata.MediaContainer.Metadata == nil {
			return nil
		}

		container := resp.MediaContainerWithMetadata.MediaContainer
		for i := range container.Metadata {
			allItems = append(allItems, &container.Metadata[i])
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetAllLibraryItems",
			Err: err,
		}
	}

	return allItems, nil
}

func (c *Client) GetLibraryItemsConcurrent(ctx context.Context, sectionIDs []string, maxConcurrent int) ([]*components.Metadata, error) {
	if maxConcurrent <= 0 {
		maxConcurrent = 4
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

func (c *Client) fetchLibraryItems(ctx context.Context, sectionID string, offset, limit int) ([]*components.Metadata, error) {
	var items []*components.Metadata

	err := c.executeWithRetry(ctx, "fetchLibraryItems", func() error {
		req := operations.GetLibraryItemsRequest{}

		resp, err := c.sdk.Library.GetLibraryItems(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to get library items: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		if resp.MediaContainerWithMetadata == nil ||
			resp.MediaContainerWithMetadata.MediaContainer == nil {
			return nil
		}

		container := resp.MediaContainerWithMetadata.MediaContainer
		if container.Metadata == nil {
			return nil
		}

		for i := range container.Metadata {
			items = append(items, &container.Metadata[i])
		}

		return nil
	})

	return items, err
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
	Type             string
	RatingKey        *string
	LibrarySectionID *string
	ViewedAt         int64
	Thumb            *string
}

func (c *Client) GetHistory(ctx context.Context, limit int) ([]HistoryItem, error) {
	if limit <= 0 {
		limit = 50
	}

	var historyItems []HistoryItem

	err := c.executeWithRetry(ctx, "GetHistory", func() error {
		req := operations.ListPlaybackHistoryRequest{}

		resp, err := c.sdk.Status.ListPlaybackHistory(ctx, req)
		if err != nil {
			return fmt.Errorf("failed to get playback history: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		if resp.Object == nil ||
			resp.Object.MediaContainer == nil ||
			resp.Object.MediaContainer.Metadata == nil {
			return nil
		}

		for _, hist := range resp.Object.MediaContainer.Metadata {
			item := HistoryItem{
				RatingKey:        hist.RatingKey,
				LibrarySectionID: hist.LibrarySectionID,
				Thumb:            hist.Thumb,
			}
			if hist.Title != nil {
				item.Title = *hist.Title
			}
			if hist.Type != nil {
				item.Type = *hist.Type
			}
			if hist.ViewedAt != nil {
				item.ViewedAt = *hist.ViewedAt
			}
			historyItems = append(historyItems, item)
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
		url := fmt.Sprintf("%s/library/sections", c.serverURL)
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
				key = "/library/sections/" + section.UUID
			}

			lib := Library{
				ID:    section.UUID,
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
