package plexclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LukeHagar/plexgo/models/components"

	"github.com/dogbertdev/plexcli/internal/cache"
)

const (
	DefaultTimeout           = 120 * time.Second
	DefaultMaxRetries        = 3
	DefaultPageSize          = 100
	DefaultConcurrentWorkers = 4
	DefaultHistoryLimit      = 50
	DefaultSearchLimit       = 50
	RetryExponent            = 2.0
	MaxBackoffDuration       = 30 * time.Second
	DefaultClientID          = "plexcli"
	DefaultLibraryPathPrefix = "/library/sections/"
	DefaultLibraryCacheTTL   = 5 * time.Minute
)

// Library represents a Plex library section.
type Library struct {
	ID       string
	Title    *string
	Type     string
	Key      *string
	UUID     string
	Location []string
}

// Client provides access to a Plex server.
type Client struct {
	httpClient          *http.Client
	serverURL           string
	token               string
	maxRetries          int
	libraryCache        *cache.LibraryPayloadCache
	libraryCacheTTL     time.Duration
	libraryCacheRefresh bool
}

type ClientOption func(*Client)

func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

func WithMaxRetries(maxRetries int) ClientOption {
	return func(c *Client) {
		c.maxRetries = maxRetries
	}
}

func WithLibraryCache(cacheStore *cache.LibraryPayloadCache) ClientOption {
	return func(c *Client) {
		c.libraryCache = cacheStore
	}
}

func WithLibraryCacheTTL(ttl time.Duration) ClientOption {
	return func(c *Client) {
		c.libraryCacheTTL = ttl
	}
}

func WithLibraryCacheRefresh(refresh bool) ClientOption {
	return func(c *Client) {
		c.libraryCacheRefresh = refresh
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
		httpClient:      &http.Client{Timeout: DefaultTimeout},
		serverURL:       serverURL,
		token:           token,
		maxRetries:      DefaultMaxRetries,
		libraryCacheTTL: DefaultLibraryCacheTTL,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client, nil
}

// PlexError provides structured error information for Plex API operations.
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
			return fmt.Errorf("%s canceled: %w", op, ctx.Err())
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
				return fmt.Errorf("%s canceled during retry: %w", op, ctx.Err())
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
	Director         []struct {
		Tag string `json:"tag"`
	} `json:"Director"`
	Media []struct {
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

	body, cacheHit, err := c.loadLibrarySectionFromCache(sectionID)
	if err != nil {
		body = nil
		cacheHit = false
	}

	if !cacheHit {
		err = c.executeWithRetry(ctx, "GetAllLibraryItems", func() error {
			url := fmt.Sprintf("%s/library/sections/%s/all?X-Plex-Container-Start=0&X-Plex-Container-Size=1000&X-Plex-Token=%s", c.serverURL, sectionID, c.token)
			req, reqErr := http.NewRequestWithContext(ctx, "GET", url, nil)
			if reqErr != nil {
				return fmt.Errorf("failed to create request: %w", reqErr)
			}

			req.Header.Set("Accept", "application/json")

			resp, doErr := c.httpClient.Do(req)
			if doErr != nil {
				return fmt.Errorf("failed to make request: %w", doErr)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
			}

			body, doErr = io.ReadAll(resp.Body)
			if doErr != nil {
				return fmt.Errorf("failed to read response body: %w", doErr)
			}

			return nil
		})
	}

	if err != nil {
		return nil, &PlexError{
			Op:      "GetAllLibraryItems",
			Section: sectionID,
			Err:     err,
		}
	}

	var rawResp rawLibraryItemsResponse
	if err := json.Unmarshal(body, &rawResp); err != nil {
		return nil, &PlexError{
			Op:      "GetAllLibraryItems",
			Section: sectionID,
			Err:     fmt.Errorf("failed to unmarshal response: %w", err),
		}
	}

	if !cacheHit {
		_ = c.saveLibrarySectionToCache(sectionID, body)
	}

	allItems := make([]*components.Metadata, 0, len(rawResp.MediaContainer.Metadata))
	for _, item := range rawResp.MediaContainer.Metadata {
		meta := convertRawToMetadata(item)
		allItems = append(allItems, meta)
	}

	return allItems, nil
}

func (c *Client) loadLibrarySectionFromCache(sectionID string) ([]byte, bool, error) {
	if c.libraryCache == nil || c.libraryCacheRefresh || c.libraryCacheTTL <= 0 {
		return nil, false, nil
	}

	return c.libraryCache.Load(c.libraryCacheKey(sectionID), c.libraryCacheTTL)
}

func (c *Client) saveLibrarySectionToCache(sectionID string, body []byte) error {
	if c.libraryCache == nil || len(body) == 0 {
		return nil
	}

	return c.libraryCache.Save(c.libraryCacheKey(sectionID), body)
}

func (c *Client) libraryCacheKey(sectionID string) string {
	rawKey := c.serverURL + "|" + c.token + "|" + sectionID
	hash := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(hash[:])
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

		resp, err := c.httpClient.Do(req)
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
		AddedAt:          int64Ptr(raw.AddedAt),
		Year:             intToInt64Ptr(raw.Year),
		ViewCount:        intToInt64Ptr(raw.ViewCount),
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
				AudioChannels:   intToInt64Ptr(m.AudioChannels),
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
							streamType := int64(s.StreamType)
							streams[k] = components.Stream{
								StreamType:   &streamType,
								Language:     s.Language,
								LanguageCode: s.LanguageCode,
								Codec:        s.Codec,
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

func int64Ptr(v int64) *int64 {
	return &v
}

func intToInt64Ptr(v *int) *int64 {
	if v == nil {
		return nil
	}
	val := int64(*v)
	return &val
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

		resp, err := c.httpClient.Do(req)
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

		resp, err := c.httpClient.Do(req)
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

func (c *Client) RefreshSection(ctx context.Context, sectionID string) error {
	return c.runLibrarySectionAction(ctx, "RefreshSection", sectionID, "refresh", http.MethodPost)
}

func (c *Client) RefreshAllSections(ctx context.Context) error {
	return c.runLibraryGlobalAction(ctx, "RefreshAllSections", "library/sections/refresh", http.MethodPost)
}

func (c *Client) CancelRefresh(ctx context.Context, sectionID string) error {
	return c.runLibrarySectionAction(ctx, "CancelRefresh", sectionID, "refresh", http.MethodDelete)
}

func (c *Client) StartAnalysis(ctx context.Context, sectionID string) error {
	return c.runLibrarySectionAction(ctx, "StartAnalysis", sectionID, "analyze", http.MethodPut)
}

func (c *Client) AnalyzeMetadata(ctx context.Context, ids string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{
			Op:  "AnalyzeMetadata",
			Err: err,
		}
	}

	return c.runLibraryGlobalAction(ctx, "AnalyzeMetadata", fmt.Sprintf("library/metadata/%s/analyze", encodedIDs), http.MethodPut)
}

func (c *Client) GenerateThumbs(ctx context.Context, ids string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{
			Op:  "GenerateThumbs",
			Err: err,
		}
	}

	return c.runLibraryGlobalAction(ctx, "GenerateThumbs", fmt.Sprintf("library/metadata/%s/chapterThumbs", encodedIDs), http.MethodPut)
}

func (c *Client) EmptyTrash(ctx context.Context, sectionID string) error {
	return c.runLibrarySectionAction(ctx, "EmptyTrash", sectionID, "emptyTrash", http.MethodPut)
}

func (c *Client) CleanSection(ctx context.Context, sectionID string) error {
	return c.EmptyTrash(ctx, sectionID)
}

func (c *Client) OptimizeDatabase(ctx context.Context) error {
	return c.runLibraryGlobalAction(ctx, "OptimizeDatabase", "library/optimize", http.MethodPut)
}

func (c *Client) CleanBundles(ctx context.Context) error {
	return c.runLibraryGlobalAction(ctx, "CleanBundles", "library/clean/bundles", http.MethodPut)
}

func (c *Client) DeleteCaches(ctx context.Context) error {
	return c.runLibraryGlobalAction(ctx, "DeleteCaches", "library/caches", http.MethodDelete)
}

func (c *Client) runLibraryGlobalAction(ctx context.Context, op, path, method string) error {
	err := c.executeWithRetry(ctx, op, func() error {
		urlStr := fmt.Sprintf("%s/%s?X-Plex-Token=%s", c.serverURL, strings.TrimPrefix(path, "/"), c.token)
		req, reqErr := http.NewRequestWithContext(ctx, method, urlStr, nil)
		if reqErr != nil {
			return fmt.Errorf("failed to create request: %w", reqErr)
		}

		resp, doErr := c.httpClient.Do(req)
		if doErr != nil {
			return fmt.Errorf("failed to make request: %w", doErr)
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusNoContent:
			return nil
		default:
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	})
	if err != nil {
		return &PlexError{
			Op:  op,
			Err: err,
		}
	}

	return nil
}

func (c *Client) RefreshLibrarySection(ctx context.Context, sectionID string) error {
	return c.RefreshSection(ctx, sectionID)
}

func (c *Client) EmptyTrashLibrarySection(ctx context.Context, sectionID string) error {
	return c.runLibrarySectionAction(ctx, "EmptyTrashLibrarySection", sectionID, "emptyTrash", http.MethodPut)
}

func (c *Client) runLibrarySectionAction(ctx context.Context, op, sectionID, actionPath, method string) error {
	if sectionID == "" {
		return &PlexError{
			Op:  op,
			Err: fmt.Errorf("section ID is required"),
		}
	}

	err := c.executeWithRetry(ctx, op, func() error {
		urlStr := fmt.Sprintf("%s/library/sections/%s/%s?X-Plex-Token=%s", c.serverURL, sectionID, actionPath, c.token)
		req, reqErr := http.NewRequestWithContext(ctx, method, urlStr, nil)
		if reqErr != nil {
			return fmt.Errorf("failed to create request: %w", reqErr)
		}

		resp, doErr := c.httpClient.Do(req)
		if doErr != nil {
			return fmt.Errorf("failed to make request: %w", doErr)
		}
		defer resp.Body.Close()

		switch resp.StatusCode {
		case http.StatusOK, http.StatusAccepted, http.StatusNoContent:
			return nil
		default:
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}
	})
	if err != nil {
		return &PlexError{
			Op:      op,
			Section: sectionID,
			Err:     err,
		}
	}

	return nil
}

func (c *Client) GetServerURL() string {
	return c.serverURL
}

func (c *Client) GetTimeout() time.Duration {
	return c.httpClient.Timeout
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

		resp, err := c.httpClient.Do(req)
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

		resp, err := c.httpClient.Do(req)
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
			playlists = append(playlists, PlaylistInfo(p))
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

		resp, err := c.httpClient.Do(req)
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
	serverUUID, uuidErr := c.GetServerUUID(ctx)
	if uuidErr != nil {
		return nil, &PlexError{
			Op:  "CreatePlaylist",
			Err: fmt.Errorf("failed to get server UUID: %w", uuidErr),
		}
	}

	var playlist *PlaylistInfo

	retryErr := c.executeWithRetry(ctx, "CreatePlaylist", func() error {
		urlStr := fmt.Sprintf("%s/playlists?title=%s&type=%s&smart=0&X-Plex-Token=%s",
			c.serverURL, url.QueryEscape(title), playlistType, c.token)

		// If we have rating keys, construct the URI
		if len(ratingKeys) > 0 {
			// URI format: server://{uuid}/com.plexapp.plugins.library/library/metadata/{key1},{key2},...
			uri := fmt.Sprintf("server://%s/com.plexapp.plugins.library/library/metadata/%s",
				serverUUID, strings.Join(ratingKeys, ","))
			urlStr += fmt.Sprintf("&uri=%s", url.QueryEscape(uri))
		}

		req, err := http.NewRequestWithContext(ctx, "POST", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
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

	if retryErr != nil {
		return nil, &PlexError{
			Op:  "CreatePlaylist",
			Err: retryErr,
		}
	}

	return playlist, nil
}

// CreateSmartPlaylist creates a smart playlist based on a filter (e.g., by director)
// sectionID is the library section, filterType is the filter category (e.g., "director"),
// filterValue is the filter ID (e.g., director ID from GetDirectors)
func (c *Client) CreateSmartPlaylist(ctx context.Context, title string, playlistType string, sectionID string, filterType string, filterValue string) (*PlaylistInfo, error) {
	if title == "" {
		return nil, &PlexError{
			Op:  "CreateSmartPlaylist",
			Err: fmt.Errorf("title is required"),
		}
	}

	if sectionID == "" {
		return nil, &PlexError{
			Op:  "CreateSmartPlaylist",
			Err: fmt.Errorf("section ID is required"),
		}
	}

	if filterType == "" || filterValue == "" {
		return nil, &PlexError{
			Op:  "CreateSmartPlaylist",
			Err: fmt.Errorf("filter type and value are required"),
		}
	}

	if playlistType == "" {
		playlistType = "video"
	}

	var playlist *PlaylistInfo

	err := c.executeWithRetry(ctx, "CreateSmartPlaylist", func() error {
		// Smart playlist URI format (from working example):
		// library://x/directory/%2Flibrary%2Fsections%2F1%2Fall%3Ftype%3D1%26sort%3DtitleSort%26director%3D2690
		// Which decodes to: library://x/directory//library/sections/1/all?type=1&sort=titleSort&director=2690
		mediaType := "1" // movies by default
		if playlistType == "audio" {
			mediaType = "10" // tracks
		}

		filterPath := fmt.Sprintf("/library/sections/%s/all?type=%s&sort=titleSort&%s=%s",
			sectionID, mediaType, filterType, filterValue)

		// URI format: library://x/directory/{url_encoded_path}
		uri := fmt.Sprintf("library://x/directory/%s", url.QueryEscape(filterPath))

		urlStr := fmt.Sprintf("%s/playlists?title=%s&type=%s&smart=1&uri=%s&X-Plex-Token=%s",
			c.serverURL, url.QueryEscape(title), playlistType, url.QueryEscape(uri), c.token)

		req, err := http.NewRequestWithContext(ctx, "POST", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
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
			Op:  "CreateSmartPlaylist",
			Err: err,
		}
	}

	return playlist, nil
}

// GetDirectorID looks up a director by name and returns their ID
func (c *Client) GetDirectorID(ctx context.Context, sectionID string, directorName string) (string, error) {
	directors, err := c.GetDirectors(ctx, sectionID)
	if err != nil {
		return "", err
	}

	directorLower := strings.ToLower(directorName)
	for _, d := range directors {
		if strings.Contains(strings.ToLower(d.Name), directorLower) {
			return d.ID, nil
		}
	}

	return "", &PlexError{
		Op:  "GetDirectorID",
		Err: fmt.Errorf("director not found: %s", directorName),
	}
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
	serverUUID, uuidErr := c.GetServerUUID(ctx)
	if uuidErr != nil {
		return &PlexError{
			Op:  "AddToPlaylist",
			Err: fmt.Errorf("failed to get server UUID: %w", uuidErr),
		}
	}

	retryErr := c.executeWithRetry(ctx, "AddToPlaylist", func() error {
		// URI format: server://{uuid}/com.plexapp.plugins.library/library/metadata/{key1},{key2},...
		uri := fmt.Sprintf("server://%s/com.plexapp.plugins.library/library/metadata/%s",
			serverUUID, strings.Join(ratingKeys, ","))

		urlStr := fmt.Sprintf("%s/playlists/%s/items?uri=%s&X-Plex-Token=%s",
			c.serverURL, playlistID, url.QueryEscape(uri), c.token)

		req, err := http.NewRequestWithContext(ctx, "PUT", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
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

	if retryErr != nil {
		return &PlexError{
			Op:  "AddToPlaylist",
			Err: retryErr,
		}
	}

	return nil
}

// DeletePlaylist deletes a playlist by ID
func (c *Client) DeletePlaylist(ctx context.Context, playlistID string) error {
	if playlistID == "" {
		return &PlexError{
			Op:  "DeletePlaylist",
			Err: fmt.Errorf("playlist ID is required"),
		}
	}

	err := c.executeWithRetry(ctx, "DeletePlaylist", func() error {
		urlStr := fmt.Sprintf("%s/playlists/%s?X-Plex-Token=%s",
			c.serverURL, playlistID, c.token)

		req, err := http.NewRequestWithContext(ctx, "DELETE", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		}

		return nil
	})

	if err != nil {
		return &PlexError{
			Op:  "DeletePlaylist",
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

		resp, err := c.httpClient.Do(req)
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

		resp, err := c.httpClient.Do(req)
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

// MovieInfo represents a movie with director info
type MovieInfo struct {
	RatingKey string   `json:"ratingKey"`
	Title     string   `json:"title"`
	Year      int      `json:"year"`
	Directors []string `json:"directors"`
}

// GetMoviesByDirector returns all movies by a given director from a library section
// The director name matching is case-insensitive and supports partial matches
func (c *Client) GetMoviesByDirector(ctx context.Context, sectionID string, directorName string) ([]MovieInfo, error) {
	if sectionID == "" {
		return nil, &PlexError{
			Op:  "GetMoviesByDirector",
			Err: fmt.Errorf("section ID is required"),
		}
	}
	if directorName == "" {
		return nil, &PlexError{
			Op:  "GetMoviesByDirector",
			Err: fmt.Errorf("director name is required"),
		}
	}

	var movies []MovieInfo

	err := c.executeWithRetry(ctx, "GetMoviesByDirector", func() error {
		// Use the director filter endpoint
		urlStr := fmt.Sprintf("%s/library/sections/%s/all?type=1&X-Plex-Container-Start=0&X-Plex-Container-Size=1000&X-Plex-Token=%s",
			c.serverURL, sectionID, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
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

		// Filter by director name (case-insensitive, partial match)
		directorLower := strings.ToLower(directorName)
		for _, item := range rawResp.MediaContainer.Metadata {
			for _, d := range item.Director {
				if strings.Contains(strings.ToLower(d.Tag), directorLower) {
					year := 0
					if item.Year != nil {
						year = *item.Year
					}
					directors := make([]string, len(item.Director))
					for i, dir := range item.Director {
						directors[i] = dir.Tag
					}
					movies = append(movies, MovieInfo{
						RatingKey: item.RatingKey,
						Title:     item.Title,
						Year:      year,
						Directors: directors,
					})
					break // Don't add the same movie twice
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:      "GetMoviesByDirector",
			Section: sectionID,
			Err:     err,
		}
	}

	return movies, nil
}

// DirectorInfo represents a director in the library
type DirectorInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"` // Number of movies by this director
}

// GetDirectors returns all directors in a library section
func (c *Client) GetDirectors(ctx context.Context, sectionID string) ([]DirectorInfo, error) {
	if sectionID == "" {
		return nil, &PlexError{
			Op:  "GetDirectors",
			Err: fmt.Errorf("section ID is required"),
		}
	}

	var directors []DirectorInfo

	err := c.executeWithRetry(ctx, "GetDirectors", func() error {
		urlStr := fmt.Sprintf("%s/library/sections/%s/director?X-Plex-Token=%s",
			c.serverURL, sectionID, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Accept", "application/json")

		resp, err := c.httpClient.Do(req)
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
				Directory []struct {
					Key   string `json:"key"`
					Title string `json:"title"`
				} `json:"Directory"`
			} `json:"MediaContainer"`
		}
		if err := json.Unmarshal(body, &rawResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		for _, d := range rawResp.MediaContainer.Directory {
			directors = append(directors, DirectorInfo{
				ID:   d.Key,
				Name: d.Title,
			})
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:      "GetDirectors",
			Section: sectionID,
			Err:     err,
		}
	}

	return directors, nil
}

// MatchResult represents a potential metadata match for an item
type MatchResult struct {
	GUID    string `json:"guid"`
	Name    string `json:"name"`
	Year    int    `json:"year,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// SearchMatches searches for metadata matches for an item (like Plex's "Fix Match" feature)
func (c *Client) SearchMatches(ctx context.Context, ratingKey string, title string, year int) ([]MatchResult, error) {
	if ratingKey == "" {
		return nil, &PlexError{
			Op:  "SearchMatches",
			Err: fmt.Errorf("rating key is required"),
		}
	}

	var results []MatchResult

	err := c.executeWithRetry(ctx, "SearchMatches", func() error {
		urlStr := fmt.Sprintf("%s/library/metadata/%s/matches?manual=1&X-Plex-Token=%s",
			c.serverURL, ratingKey, c.token)

		if title != "" {
			urlStr += "&title=" + url.QueryEscape(title)
		}
		if year > 0 {
			urlStr += fmt.Sprintf("&year=%d", year)
		}

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
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

		// Parse XML response
		var container struct {
			XMLName xml.Name `xml:"MediaContainer"`
			Results []struct {
				GUID    string `xml:"guid,attr"`
				Name    string `xml:"name,attr"`
				Year    int    `xml:"year,attr"`
				Summary string `xml:"summary,attr"`
			} `xml:"SearchResult"`
		}

		if err := xml.Unmarshal(body, &container); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		results = make([]MatchResult, len(container.Results))
		for i, r := range container.Results {
			results[i] = MatchResult{
				GUID:    r.GUID,
				Name:    r.Name,
				Year:    r.Year,
				Summary: r.Summary,
			}
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "SearchMatches",
			Err: err,
		}
	}

	return results, nil
}

// ApplyMatch applies a metadata match to an item (like selecting a match in Plex's "Fix Match")
func (c *Client) ApplyMatch(ctx context.Context, ratingKey string, guid string, name string) error {
	if ratingKey == "" {
		return &PlexError{
			Op:  "ApplyMatch",
			Err: fmt.Errorf("rating key is required"),
		}
	}
	if guid == "" {
		return &PlexError{
			Op:  "ApplyMatch",
			Err: fmt.Errorf("guid is required"),
		}
	}

	return c.executeWithRetry(ctx, "ApplyMatch", func() error {
		urlStr := fmt.Sprintf("%s/library/metadata/%s/match?guid=%s&X-Plex-Token=%s",
			c.serverURL, ratingKey, url.QueryEscape(guid), c.token)

		if name != "" {
			urlStr += "&name=" + url.QueryEscape(name)
		}

		req, err := http.NewRequestWithContext(ctx, "PUT", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		return nil
	})
}

// StreamInfo represents an audio or subtitle stream
type StreamInfo struct {
	ID           int    `json:"id"`
	StreamType   int    `json:"stream_type"`
	Language     string `json:"language"`
	LanguageCode string `json:"language_code"`
	Codec        string `json:"codec"`
	Title        string `json:"title,omitempty"`
	DisplayTitle string `json:"display_title,omitempty"`
	Channels     int    `json:"channels,omitempty"`
	Selected     bool   `json:"selected"`
	Default      bool   `json:"default"`
}

// EpisodeInfo represents basic episode information
type EpisodeInfo struct {
	RatingKey string
	PartID    string
	Title     string
	Season    int
	Episode   int
}

// GetStreams returns all streams (video, audio, subtitle) for an item
func (c *Client) GetStreams(ctx context.Context, ratingKey string) ([]StreamInfo, error) {
	if ratingKey == "" {
		return nil, &PlexError{
			Op:  "GetStreams",
			Err: fmt.Errorf("rating key is required"),
		}
	}

	var streams []StreamInfo

	err := c.executeWithRetry(ctx, "GetStreams", func() error {
		urlStr := fmt.Sprintf("%s/library/metadata/%s?X-Plex-Token=%s",
			c.serverURL, ratingKey, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
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

		var container struct {
			XMLName xml.Name `xml:"MediaContainer"`
			Video   []struct {
				Media []struct {
					Part []struct {
						Stream []struct {
							ID           int    `xml:"id,attr"`
							StreamType   int    `xml:"streamType,attr"`
							Language     string `xml:"language,attr"`
							LanguageCode string `xml:"languageCode,attr"`
							Codec        string `xml:"codec,attr"`
							Title        string `xml:"title,attr"`
							DisplayTitle string `xml:"displayTitle,attr"`
							Channels     int    `xml:"channels,attr"`
							Selected     int    `xml:"selected,attr"`
							Default      int    `xml:"default,attr"`
						} `xml:"Stream"`
					} `xml:"Part"`
				} `xml:"Media"`
			} `xml:"Video"`
		}

		if err := xml.Unmarshal(body, &container); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		streams = nil
		for _, video := range container.Video {
			for _, media := range video.Media {
				for _, part := range media.Part {
					for _, s := range part.Stream {
						streams = append(streams, StreamInfo{
							ID:           s.ID,
							StreamType:   s.StreamType,
							Language:     s.Language,
							LanguageCode: s.LanguageCode,
							Codec:        s.Codec,
							Title:        s.Title,
							DisplayTitle: s.DisplayTitle,
							Channels:     s.Channels,
							Selected:     s.Selected == 1,
							Default:      s.Default == 1,
						})
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetStreams",
			Err: err,
		}
	}

	return streams, nil
}

// SetStreams sets the default audio and/or subtitle stream for a part.
// Pass nil to leave a stream type unchanged. For subtitles, 0 means disable.
func (c *Client) SetStreams(ctx context.Context, partID string, audioStreamID, subtitleStreamID *int) error {
	if partID == "" {
		return &PlexError{
			Op:  "SetStreams",
			Err: fmt.Errorf("part ID is required"),
		}
	}

	return c.executeWithRetry(ctx, "SetStreams", func() error {
		urlStr := fmt.Sprintf("%s/library/parts/%s?X-Plex-Token=%s",
			c.serverURL, partID, c.token)

		if audioStreamID != nil && *audioStreamID > 0 {
			urlStr += fmt.Sprintf("&audioStreamID=%d", *audioStreamID)
		}
		if subtitleStreamID != nil && *subtitleStreamID > 0 {
			urlStr += fmt.Sprintf("&subtitleStreamID=%d", *subtitleStreamID)
		} else if subtitleStreamID != nil && *subtitleStreamID == 0 {
			// 0 means disable subtitles
			urlStr += "&subtitleStreamID=0"
		}

		req, err := http.NewRequestWithContext(ctx, "PUT", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		return nil
	})
}

// GetSeasonEpisodes returns all episodes for a specific season of a show
func (c *Client) GetSeasonEpisodes(ctx context.Context, showRatingKey string, seasonNum int) ([]EpisodeInfo, error) {
	if showRatingKey == "" {
		return nil, &PlexError{
			Op:  "GetSeasonEpisodes",
			Err: fmt.Errorf("show rating key is required"),
		}
	}

	var episodes []EpisodeInfo

	err := c.executeWithRetry(ctx, "GetSeasonEpisodes", func() error {
		// First get seasons
		urlStr := fmt.Sprintf("%s/library/metadata/%s/children?X-Plex-Token=%s",
			c.serverURL, showRatingKey, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
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

		var seasonsContainer struct {
			XMLName xml.Name `xml:"MediaContainer"`
			Seasons []struct {
				RatingKey string `xml:"ratingKey,attr"`
				Index     int    `xml:"index,attr"`
			} `xml:"Directory"`
		}

		if unmarshalErr := xml.Unmarshal(body, &seasonsContainer); unmarshalErr != nil {
			return fmt.Errorf("failed to parse seasons response: %w", unmarshalErr)
		}

		// Find the matching season
		var seasonKey string
		for _, s := range seasonsContainer.Seasons {
			if s.Index == seasonNum {
				seasonKey = s.RatingKey
				break
			}
		}

		if seasonKey == "" {
			return fmt.Errorf("season %d not found", seasonNum)
		}

		// Get episodes for the season
		urlStr = fmt.Sprintf("%s/library/metadata/%s/children?X-Plex-Token=%s",
			c.serverURL, seasonKey, c.token)

		req, err = http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err = c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		}

		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response body: %w", err)
		}

		var episodesContainer struct {
			XMLName  xml.Name `xml:"MediaContainer"`
			Episodes []struct {
				RatingKey   string `xml:"ratingKey,attr"`
				Title       string `xml:"title,attr"`
				ParentIndex int    `xml:"parentIndex,attr"`
				Index       int    `xml:"index,attr"`
				Media       []struct {
					Part []struct {
						ID string `xml:"id,attr"`
					} `xml:"Part"`
				} `xml:"Media"`
			} `xml:"Video"`
		}

		if err := xml.Unmarshal(body, &episodesContainer); err != nil {
			return fmt.Errorf("failed to parse episodes response: %w", err)
		}

		episodes = nil
		for _, ep := range episodesContainer.Episodes {
			partID := ""
			if len(ep.Media) > 0 && len(ep.Media[0].Part) > 0 {
				partID = ep.Media[0].Part[0].ID
			}

			episodes = append(episodes, EpisodeInfo{
				RatingKey: ep.RatingKey,
				PartID:    partID,
				Title:     ep.Title,
				Season:    ep.ParentIndex,
				Episode:   ep.Index,
			})
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetSeasonEpisodes",
			Err: err,
		}
	}

	return episodes, nil
}

// ActiveSession represents a currently playing session
type ActiveSession struct {
	User     string `json:"user"`
	Title    string `json:"title"`
	Type     string `json:"type"`
	Show     string `json:"show,omitempty"`
	Progress string `json:"progress"`
	Device   string `json:"device"`
	State    string `json:"state"`
}

// HistoryEntry represents a watch history entry
type HistoryEntry struct {
	Title            string    `json:"title"`
	Type             string    `json:"type"`
	GrandparentTitle string    `json:"grandparent_title,omitempty"`
	ParentIndex      int       `json:"parent_index,omitempty"`
	Index            int       `json:"index,omitempty"`
	AccountID        int       `json:"account_id"`
	DeviceID         int       `json:"device_id"`
	ViewedAt         time.Time `json:"viewed_at"`
}

// Account represents a Plex user account
type Account struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// ActivityTask represents an in-progress Plex activity.
type ActivityTask struct {
	UUID        string   `json:"uuid"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Subtitle    string   `json:"subtitle"`
	Progress    *float64 `json:"progress,omitempty"`
	Cancellable bool     `json:"cancellable"`
}

// BackgroundTask represents an in-progress background task.
type BackgroundTask struct {
	Type      string   `json:"type"`
	Title     string   `json:"title"`
	Progress  *float64 `json:"progress,omitempty"`
	Remaining *int64   `json:"remaining,omitempty"`
	Speed     *float64 `json:"speed,omitempty"`
}

// GetActiveSessions returns currently active playback sessions
func (c *Client) GetActiveSessions(ctx context.Context) ([]ActiveSession, error) {
	var sessions []ActiveSession

	err := c.executeWithRetry(ctx, "GetActiveSessions", func() error {
		urlStr := fmt.Sprintf("%s/status/sessions?X-Plex-Token=%s", c.serverURL, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
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

		var container struct {
			XMLName xml.Name `xml:"MediaContainer"`
			Videos  []struct {
				Title            string `xml:"title,attr"`
				Type             string `xml:"type,attr"`
				GrandparentTitle string `xml:"grandparentTitle,attr"`
				ViewOffset       int    `xml:"viewOffset,attr"`
				Duration         int    `xml:"duration,attr"`
				User             struct {
					Title string `xml:"title,attr"`
				} `xml:"User"`
				Player struct {
					Device string `xml:"device,attr"`
					State  string `xml:"state,attr"`
				} `xml:"Player"`
			} `xml:"Video"`
		}

		if err := xml.Unmarshal(body, &container); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		sessions = nil
		for _, v := range container.Videos {
			progress := ""
			if v.Duration > 0 {
				pct := float64(v.ViewOffset) / float64(v.Duration) * 100
				progress = fmt.Sprintf("%.0f%%", pct)
			}

			sessions = append(sessions, ActiveSession{
				User:     v.User.Title,
				Title:    v.Title,
				Type:     v.Type,
				Show:     v.GrandparentTitle,
				Progress: progress,
				Device:   v.Player.Device,
				State:    v.Player.State,
			})
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetActiveSessions",
			Err: err,
		}
	}

	return sessions, nil
}

// GetWatchHistory returns the watch history
func (c *Client) GetWatchHistory(ctx context.Context) ([]HistoryEntry, error) {
	var history []HistoryEntry

	err := c.executeWithRetry(ctx, "GetWatchHistory", func() error {
		urlStr := fmt.Sprintf("%s/status/sessions/history/all?X-Plex-Token=%s", c.serverURL, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
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

		var container struct {
			XMLName xml.Name `xml:"MediaContainer"`
			Videos  []struct {
				Title            string `xml:"title,attr"`
				Type             string `xml:"type,attr"`
				GrandparentTitle string `xml:"grandparentTitle,attr"`
				ParentIndex      int    `xml:"parentIndex,attr"`
				Index            int    `xml:"index,attr"`
				AccountID        int    `xml:"accountID,attr"`
				DeviceID         int    `xml:"deviceID,attr"`
				ViewedAt         int64  `xml:"viewedAt,attr"`
			} `xml:"Video"`
		}

		if err := xml.Unmarshal(body, &container); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		history = nil
		for _, v := range container.Videos {
			history = append(history, HistoryEntry{
				Title:            v.Title,
				Type:             v.Type,
				GrandparentTitle: v.GrandparentTitle,
				ParentIndex:      v.ParentIndex,
				Index:            v.Index,
				AccountID:        v.AccountID,
				DeviceID:         v.DeviceID,
				ViewedAt:         time.Unix(v.ViewedAt, 0),
			})
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetWatchHistory",
			Err: err,
		}
	}

	return history, nil
}

// GetAccounts returns all user accounts on the server
func (c *Client) GetAccounts(ctx context.Context) ([]Account, error) {
	var accounts []Account

	err := c.executeWithRetry(ctx, "GetAccounts", func() error {
		urlStr := fmt.Sprintf("%s/accounts?X-Plex-Token=%s", c.serverURL, c.token)

		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
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

		var container struct {
			XMLName  xml.Name `xml:"MediaContainer"`
			Accounts []struct {
				ID   int    `xml:"id,attr"`
				Name string `xml:"name,attr"`
			} `xml:"Account"`
		}

		if err := xml.Unmarshal(body, &container); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		accounts = nil
		for _, a := range container.Accounts {
			if a.Name != "" { // Skip empty accounts
				accounts = append(accounts, Account{
					ID:   a.ID,
					Name: a.Name,
				})
			}
		}

		return nil
	})

	if err != nil {
		return nil, &PlexError{
			Op:  "GetAccounts",
			Err: err,
		}
	}

	return accounts, nil
}

// GetActivities returns currently running server activities.
func (c *Client) GetActivities(ctx context.Context) ([]ActivityTask, error) {
	var activities []ActivityTask

	err := c.executeWithRetry(ctx, "GetActivities", func() error {
		urlStr := fmt.Sprintf("%s/activities?X-Plex-Token=%s", c.serverURL, c.token)
		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
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

		var container struct {
			XMLName    xml.Name `xml:"MediaContainer"`
			Activities []struct {
				UUID        string `xml:"uuid,attr"`
				Type        string `xml:"type,attr"`
				Title       string `xml:"title,attr"`
				Subtitle    string `xml:"subtitle,attr"`
				ProgressRaw string `xml:"progress,attr"`
				CancelRaw   string `xml:"cancellable,attr"`
			} `xml:"Activity"`
		}

		if err := xml.Unmarshal(body, &container); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		activities = nil
		for _, a := range container.Activities {
			progress, parseErr := parseOptionalFloat(a.ProgressRaw)
			if parseErr != nil {
				return fmt.Errorf("invalid activity progress %q: %w", a.ProgressRaw, parseErr)
			}

			activities = append(activities, ActivityTask{
				UUID:        a.UUID,
				Type:        a.Type,
				Title:       a.Title,
				Subtitle:    a.Subtitle,
				Progress:    progress,
				Cancellable: parseBoolAttr(a.CancelRaw),
			})
		}

		return nil
	})
	if err != nil {
		return nil, &PlexError{
			Op:  "GetActivities",
			Err: err,
		}
	}

	return activities, nil
}

// GetBackgroundTasks returns currently running background tasks.
func (c *Client) GetBackgroundTasks(ctx context.Context) ([]BackgroundTask, error) {
	var tasks []BackgroundTask

	err := c.executeWithRetry(ctx, "GetBackgroundTasks", func() error {
		urlStr := fmt.Sprintf("%s/status/sessions/background?X-Plex-Token=%s", c.serverURL, c.token)
		req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
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

		type rawTask struct {
			Type         string `xml:"type,attr"`
			Title        string `xml:"title,attr"`
			ProgressRaw  string `xml:"progress,attr"`
			RemainingRaw string `xml:"remaining,attr"`
			SpeedRaw     string `xml:"speed,attr"`
		}

		var container struct {
			XMLName          xml.Name  `xml:"MediaContainer"`
			TranscodeJobs    []rawTask `xml:"TranscodeJob"`
			TranscodeSession []rawTask `xml:"TranscodeSession"`
		}

		if err := xml.Unmarshal(body, &container); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		appendTask := func(rt rawTask) error {
			progress, parseErr := parseOptionalFloat(rt.ProgressRaw)
			if parseErr != nil {
				return fmt.Errorf("invalid task progress %q: %w", rt.ProgressRaw, parseErr)
			}
			remaining, parseErr := parseOptionalInt64(rt.RemainingRaw)
			if parseErr != nil {
				return fmt.Errorf("invalid task remaining %q: %w", rt.RemainingRaw, parseErr)
			}
			speed, parseErr := parseOptionalFloat(rt.SpeedRaw)
			if parseErr != nil {
				return fmt.Errorf("invalid task speed %q: %w", rt.SpeedRaw, parseErr)
			}

			tasks = append(tasks, BackgroundTask{
				Type:      rt.Type,
				Title:     rt.Title,
				Progress:  progress,
				Remaining: remaining,
				Speed:     speed,
			})
			return nil
		}

		tasks = nil
		for _, t := range container.TranscodeJobs {
			if appendErr := appendTask(t); appendErr != nil {
				return appendErr
			}
		}
		for _, t := range container.TranscodeSession {
			if appendErr := appendTask(t); appendErr != nil {
				return appendErr
			}
		}

		return nil
	})
	if err != nil {
		return nil, &PlexError{
			Op:  "GetBackgroundTasks",
			Err: err,
		}
	}

	return tasks, nil
}

func parseOptionalFloat(raw string) (*float64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func parseOptionalInt64(raw string) (*int64, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func parseBoolAttr(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func encodeMetadataIDs(ids string) (string, error) {
	trimmed := strings.TrimSpace(ids)
	if trimmed == "" {
		return "", fmt.Errorf("ids are required")
	}

	parts := strings.Split(trimmed, ",")
	encoded := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			return "", fmt.Errorf("ids contain an empty segment")
		}
		encoded = append(encoded, url.PathEscape(id))
	}

	return strings.Join(encoded, ","), nil
}
