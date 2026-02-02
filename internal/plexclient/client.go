package plexclient

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/LukeHagar/plexgo"
	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
	"github.com/LukeHagar/plexgo/retry"
)

const (
	DefaultTimeout    = 30 * time.Second
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
	offset := 0
	pageSize := DefaultPageSize

	for {
		var items []*components.Metadata
		var err error

		if sectionID == "" {
			items, err = c.fetchLibraryItems(ctx, "", offset, pageSize)
		} else {
			items, err = c.fetchLibraryItems(ctx, sectionID, offset, pageSize)
		}

		if err != nil {
			return nil, &PlexError{
				Op:      "GetAllLibraryItems",
				Section: sectionID,
				Err:     err,
			}
		}

		if len(items) == 0 {
			break
		}

		allItems = append(allItems, items...)

		if len(items) < pageSize {
			break
		}

		offset += pageSize
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

func (c *Client) GetHistory(ctx context.Context, limit int) ([]*components.Metadata, error) {
	if limit <= 0 {
		limit = 50
	}

	var historyItems []*components.Metadata

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
			meta := &components.Metadata{}
			if hist.Title != nil {
				meta.Title = *hist.Title
			}
			if hist.Type != nil {
				meta.Type = *hist.Type
			}
			meta.RatingKey = hist.RatingKey
			if hist.Key != nil {
				meta.Key = *hist.Key
			}
			meta.Thumb = hist.Thumb
			historyItems = append(historyItems, meta)
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

func (c *Client) GetSections(ctx context.Context) ([]Library, error) {
	var libraries []Library

	err := c.executeWithRetry(ctx, "GetSections", func() error {
		resp, err := c.sdk.Library.GetSections(ctx)
		if err != nil {
			return fmt.Errorf("failed to get library sections: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		if resp.Object == nil ||
			resp.Object.MediaContainer == nil ||
			resp.Object.MediaContainer.Directory == nil {
			return nil
		}

		for _, section := range resp.Object.MediaContainer.Directory {
			lib := Library{
				ID:    section.GetUUID(),
				Title: section.GetTitle(),
				Type:  string(section.GetType()),
				Key:   section.GetKey(),
				UUID:  section.GetUUID(),
			}

			if lib.Title == nil || *lib.Title == "" {
				title := "Unknown"
				lib.Title = &title
			}
			if lib.Key == nil || *lib.Key == "" {
				key := "/library/sections/" + lib.ID
				lib.Key = &key
			}

			for _, loc := range section.GetLocation() {
				if path := loc.GetPath(); path != nil {
					lib.Location = append(lib.Location, fmt.Sprintf("%v", path))
				}
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
