package plexclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/LukeHagar/plexgo/models/components"
	"github.com/LukeHagar/plexgo/models/operations"
)

type LibraryBinaryResult struct {
	Action      string `json:"action"`
	Target      string `json:"target"`
	OutputPath  string `json:"output_path"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"bytes"`
}

type LibraryMutationSummary struct {
	Action  string `json:"action"`
	Target  string `json:"target"`
	Outcome string `json:"outcome"`
}

type SectionMutationInput struct {
	Name      string
	Type      int64
	Agent     string
	Scanner   string
	Language  string
	Locations []string
	Prefs     map[string]string
}

type MetadataEditInput struct {
	Set    map[string]string
	Lock   []string
	Unlock []string
}

type BulkUpdateInput struct {
	SectionID  string
	MediaType  int64
	Filter     string
	Set        map[string]string
	Lock       []string
	AddTags    map[string]string
	RemoveTags map[string]string
}

func ParseCSVList(input string) ([]string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, fmt.Errorf("at least one ID is required")
	}

	parts := strings.Split(input, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty ID in %q", input)
		}
		values = append(values, part)
	}
	return values, nil
}

func ParseKeyValuePairs(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one key=value pair is required")
	}

	parsed := make(map[string]string, len(values))
	for _, value := range values {
		key, val, ok := strings.Cut(value, "=")
		if !ok {
			return nil, fmt.Errorf("invalid key=value pair %q", value)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			return nil, fmt.Errorf("invalid key=value pair %q", value)
		}
		parsed[key] = val
	}
	return parsed, nil
}

func RequireConfirmation(confirmed bool, action string) error {
	if confirmed {
		return nil
	}
	return fmt.Errorf("%s requires --yes", action)
}

func RequireOutputPath(outputPath string) error {
	if strings.TrimSpace(outputPath) == "" {
		return fmt.Errorf("--output is required")
	}
	return nil
}

func EncodeMetadataInput(input string) (string, error) {
	return encodeMetadataIDs(input)
}

func (c *Client) LibraryJSON(ctx context.Context, op, method, path string, query url.Values) (any, error) {
	body, err := c.libraryRequest(ctx, op, method, path, query, "application/json")
	if err != nil {
		return nil, err
	}

	if len(body) == 0 {
		return map[string]any{}, nil
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, &PlexError{Op: op, Err: fmt.Errorf("failed to decode JSON response: %w", err)}
	}

	return payload, nil
}

func (c *Client) LibraryAction(ctx context.Context, op, method, path string, query url.Values) error {
	_, err := c.libraryRequest(ctx, op, method, path, query, "*/*")
	return err
}

func (c *Client) LibraryDownload(ctx context.Context, op, method, path string, query url.Values, outputPath string) (*LibraryBinaryResult, error) {
	if err := RequireOutputPath(outputPath); err != nil {
		return nil, err
	}

	resp, err := c.libraryHTTP(ctx, op, method, path, query, "*/*")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if mkdirErr := os.MkdirAll(filepath.Dir(outputPath), 0o755); mkdirErr != nil {
		return nil, &PlexError{Op: op, Err: fmt.Errorf("failed to create output directory: %w", mkdirErr)}
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return nil, &PlexError{Op: op, Err: fmt.Errorf("failed to create output file: %w", err)}
	}
	defer file.Close()

	written, err := io.Copy(file, resp.Body)
	if err != nil {
		return nil, &PlexError{Op: op, Err: fmt.Errorf("failed to write output file: %w", err)}
	}

	return &LibraryBinaryResult{
		Action:      op,
		Target:      path,
		OutputPath:  outputPath,
		ContentType: resp.Header.Get("Content-Type"),
		Bytes:       written,
	}, nil
}

func (c *Client) DownloadPartIndex(ctx context.Context, partID int64, index string, interval *int64, outputPath string) (*LibraryBinaryResult, error) {
	query := url.Values{}
	if interval != nil {
		query.Set("interval", strconv.FormatInt(*interval, 10))
	}
	return c.LibraryDownload(ctx, "GetPartIndex", http.MethodGet, fmt.Sprintf("library/parts/%d/indexes/%s", partID, url.PathEscape(index)), query, outputPath)
}

func (c *Client) CreateSection(ctx context.Context, input SectionMutationInput) error {
	query := url.Values{}
	query.Set("name", input.Name)
	query.Set("type", strconv.FormatInt(input.Type, 10))
	query.Set("agent", input.Agent)
	if input.Scanner != "" {
		query.Set("scanner", input.Scanner)
	}
	if input.Language != "" {
		query.Set("language", input.Language)
	}
	for _, location := range input.Locations {
		if strings.TrimSpace(location) != "" {
			query.Add("location", location)
			query.Add("locations", location)
		}
	}
	addDeepObject(query, "prefs", input.Prefs)
	return c.LibraryAction(ctx, "AddSection", http.MethodPost, "library/sections", query)
}

func (c *Client) EditSection(ctx context.Context, sectionID string, input SectionMutationInput) error {
	resolvedInput, err := c.resolveSectionEditInput(ctx, sectionID, input)
	if err != nil {
		return err
	}

	query := url.Values{}
	if resolvedInput.Name != "" {
		query.Set("name", resolvedInput.Name)
	}
	if resolvedInput.Agent != "" {
		query.Set("agent", resolvedInput.Agent)
	}
	if resolvedInput.Scanner != "" {
		query.Set("scanner", resolvedInput.Scanner)
	}
	if resolvedInput.Language != "" {
		query.Set("language", resolvedInput.Language)
	}
	for _, location := range resolvedInput.Locations {
		if strings.TrimSpace(location) != "" {
			query.Add("locations", location)
		}
	}
	addDeepObject(query, "prefs", resolvedInput.Prefs)
	return c.LibraryAction(ctx, "EditSection", http.MethodPut, fmt.Sprintf("library/sections/%s", url.PathEscape(sectionID)), query)
}

func (c *Client) SetSectionPreferencesDynamic(ctx context.Context, sectionID string, prefs map[string]string) error {
	query := url.Values{}
	for key, value := range prefs {
		if strings.TrimSpace(key) != "" {
			query.Set(key, value)
		}
	}
	return c.LibraryAction(ctx, "SetSectionPreferences", http.MethodPut, fmt.Sprintf("library/sections/%s/prefs", url.PathEscape(sectionID)), query)
}

func (c *Client) EditMetadataDynamic(ctx context.Context, ids string, input MetadataEditInput) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "EditMetadataItem", Err: err}
	}

	query := url.Values{}
	for key, value := range input.Set {
		query.Set(fmt.Sprintf("%s.value", key), value)
	}
	for _, key := range input.Lock {
		if strings.TrimSpace(key) != "" {
			query.Set(fmt.Sprintf("%s.locked", key), "1")
		}
	}
	for _, key := range input.Unlock {
		if strings.TrimSpace(key) != "" {
			query.Set(fmt.Sprintf("%s.locked", key), "0")
		}
	}
	return c.LibraryAction(ctx, "EditMetadataItem", http.MethodPut, fmt.Sprintf("library/metadata/%s", encodedIDs), query)
}

func (c *Client) SetItemPreferencesDynamic(ctx context.Context, ids string, prefs map[string]string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "SetItemPreferences", Err: err}
	}

	query := url.Values{}
	for key, value := range prefs {
		if strings.TrimSpace(key) != "" {
			query.Set(key, value)
		}
	}
	return c.LibraryAction(ctx, "SetItemPreferences", http.MethodPut, fmt.Sprintf("library/metadata/%s/prefs", encodedIDs), query)
}

func (c *Client) UpdateItemsDynamic(ctx context.Context, input BulkUpdateInput) error {
	query := url.Values{}
	if input.MediaType > 0 {
		query.Set("type", strconv.FormatInt(input.MediaType, 10))
	}
	if input.Filter != "" {
		query.Set("filters", input.Filter)
	}
	for key, value := range input.Set {
		query.Set(fmt.Sprintf("%s.value", key), value)
	}
	for _, key := range input.Lock {
		if strings.TrimSpace(key) != "" {
			query.Set(fmt.Sprintf("%s.locked", key), "1")
		}
	}
	for tagType, value := range input.AddTags {
		query.Set(fmt.Sprintf("%s[].tag.tag", tagType), value)
	}
	for tagType, value := range input.RemoveTags {
		query.Set(fmt.Sprintf("%s[].tag.tag-", tagType), value)
	}
	return c.LibraryAction(ctx, "UpdateItems", http.MethodPut, fmt.Sprintf("library/sections/%s/all", url.PathEscape(input.SectionID)), query)
}

func (c *Client) AddSubtitlesByURL(ctx context.Context, ids, subtitleURL, language, title string, mediaItemID int64, format string, forced, hearingImpaired bool) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "AddSubtitles", Err: err}
	}

	query := url.Values{}
	query.Set("url", subtitleURL)
	if language != "" {
		query.Set("language", language)
	}
	if title != "" {
		query.Set("title", title)
	}
	if mediaItemID > 0 {
		query.Set("mediaItemID", strconv.FormatInt(mediaItemID, 10))
	}
	if format != "" {
		query.Set("format", format)
	}
	if forced {
		query.Set("forced", "1")
	}
	if hearingImpaired {
		query.Set("hearingImpaired", "1")
	}
	return c.LibraryAction(ctx, "AddSubtitles", http.MethodGet, fmt.Sprintf("library/metadata/%s/subtitles", encodedIDs), query)
}

func (c *Client) AddSubtitlesFromFile(ctx context.Context, ids string, payload []byte, language, title string, mediaItemID int64, format string, forced, hearingImpaired bool) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "AddSubtitles", Err: err}
	}

	query := url.Values{}
	if language != "" {
		query.Set("language", language)
	}
	if title != "" {
		query.Set("title", title)
	}
	if mediaItemID > 0 {
		query.Set("mediaItemID", strconv.FormatInt(mediaItemID, 10))
	}
	if format != "" {
		query.Set("format", format)
	}
	if forced {
		query.Set("forced", "1")
	}
	if hearingImpaired {
		query.Set("hearingImpaired", "1")
	}

	return c.libraryActionWithBody(ctx, "AddSubtitles", http.MethodPost, fmt.Sprintf("library/metadata/%s/subtitles", encodedIDs), query, payload, subtitleUploadContentType(format, title))
}

func (c *Client) SetItemArtworkByURL(ctx context.Context, ids, element, assetURL string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "SetItemArtwork", Err: err}
	}

	query := url.Values{}
	query.Set("url", assetURL)
	return c.LibraryAction(ctx, "SetItemArtwork", http.MethodPut, fmt.Sprintf("library/metadata/%s/%s", encodedIDs, url.PathEscape(element)), query)
}

func (c *Client) UpdateItemArtworkByURL(ctx context.Context, ids, element, assetURL string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "UpdateItemArtwork", Err: err}
	}

	query := url.Values{}
	query.Set("url", assetURL)
	return c.LibraryAction(ctx, "UpdateItemArtwork", http.MethodPut, fmt.Sprintf("library/metadata/%s/%s", encodedIDs, url.PathEscape(element)), query)
}

func (c *Client) DeleteSection(ctx context.Context, sectionID string) error {
	_, err := c.sdk.Library.DeleteLibrarySection(ctx, operations.DeleteLibrarySectionRequest{SectionID: sectionID})
	return c.wrapSDKError("DeleteLibrarySection", sectionID, err)
}

func (c *Client) StopAllRefreshes(ctx context.Context) error {
	_, err := c.sdk.Library.StopAllRefreshes(ctx)
	return c.wrapSDKError("StopAllRefreshes", "", err)
}

func (c *Client) GetSectionsPrefs(ctx context.Context, metadataType int64, agent string) (any, error) {
	query := url.Values{}
	query.Set("type", strconv.FormatInt(metadataType, 10))
	if strings.TrimSpace(agent) != "" {
		query.Set("agent", agent)
	}
	return c.LibraryJSON(ctx, "GetSectionsPrefs", http.MethodGet, "library/sections/prefs", query)
}

func (c *Client) resolveSectionEditInput(ctx context.Context, sectionID string, input SectionMutationInput) (SectionMutationInput, error) {
	if input.Agent != "" && input.Scanner != "" {
		return input, nil
	}

	payload, err := c.LibraryJSON(ctx, "GetSections", http.MethodGet, "library/sections", nil)
	if err != nil {
		return input, err
	}

	container, ok := payload.(map[string]any)
	if !ok {
		return input, &PlexError{Op: "EditSection", Section: sectionID, Err: fmt.Errorf("unexpected section payload shape")}
	}
	mediaContainer, ok := container["MediaContainer"].(map[string]any)
	if !ok {
		return input, &PlexError{Op: "EditSection", Section: sectionID, Err: fmt.Errorf("missing MediaContainer in section payload")}
	}
	directories, ok := mediaContainer["Directory"].([]any)
	if !ok {
		return input, &PlexError{Op: "EditSection", Section: sectionID, Err: fmt.Errorf("missing Directory list in section payload")}
	}

	for _, entry := range directories {
		directory, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		key, _ := directory["key"].(string)
		if key != sectionID {
			continue
		}

		if input.Agent == "" {
			if agent, _ := directory["agent"].(string); agent != "" {
				input.Agent = agent
			}
		}
		if input.Scanner == "" {
			if scanner, _ := directory["scanner"].(string); scanner != "" {
				input.Scanner = scanner
			}
		}
		return input, nil
	}

	return input, &PlexError{Op: "EditSection", Section: sectionID, Err: fmt.Errorf("section not found")}
}

func (c *Client) DeleteMetadata(ctx context.Context, ids string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "DeleteMetadataItem", Err: err}
	}
	_, err = c.sdk.Library.DeleteMetadataItem(ctx, operations.DeleteMetadataItemRequest{Ids: encodedIDs})
	return c.wrapSDKError("DeleteMetadataItem", encodedIDs, err)
}

func (c *Client) UnmatchMetadata(ctx context.Context, ids string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "Unmatch", Err: err}
	}
	_, err = c.sdk.Library.Unmatch(ctx, operations.UnmatchRequest{Ids: encodedIDs})
	return c.wrapSDKError("Unmatch", encodedIDs, err)
}

func (c *Client) SplitMetadata(ctx context.Context, ids string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "SplitItem", Err: err}
	}
	_, err = c.sdk.Library.SplitItem(ctx, operations.SplitItemRequest{Ids: encodedIDs})
	return c.wrapSDKError("SplitItem", encodedIDs, err)
}

func (c *Client) MergeMetadata(ctx context.Context, ids string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "MergeItems", Err: err}
	}
	mergeIDs, err := ParseCSVList(ids)
	if err != nil {
		return &PlexError{Op: "MergeItems", Err: err}
	}
	_, err = c.sdk.Library.MergeItems(ctx, operations.MergeItemsRequest{
		IdsPathParameter:  encodedIDs,
		IdsQueryParameter: mergeIDs,
	})
	return c.wrapSDKError("MergeItems", encodedIDs, err)
}

func (c *Client) DetectMetadataAds(ctx context.Context, ids string) error {
	return c.runMetadataSDKAction(ctx, "DetectAds", ids, func(ctx context.Context, encodedIDs string) error {
		_, err := c.sdk.Library.DetectAds(ctx, operations.DetectAdsRequest{Ids: encodedIDs})
		return err
	})
}

func (c *Client) DetectMetadataCredits(ctx context.Context, ids string, force bool, manual bool) error {
	return c.runMetadataSDKAction(ctx, "DetectCredits", ids, func(ctx context.Context, encodedIDs string) error {
		req := operations.DetectCreditsRequest{Ids: encodedIDs}
		if force {
			req.Force = BoolIntPointer(true)
		}
		if manual {
			req.Manual = BoolIntPointer(true)
		}
		_, err := c.sdk.Library.DetectCredits(ctx, req)
		return err
	})
}

func (c *Client) DetectMetadataIntros(ctx context.Context, ids string, force bool, threshold *float64) error {
	return c.runMetadataSDKAction(ctx, "DetectIntros", ids, func(ctx context.Context, encodedIDs string) error {
		req := operations.DetectIntrosRequest{Ids: encodedIDs}
		if force {
			req.Force = BoolIntPointer(true)
		}
		if threshold != nil {
			req.Threshold = threshold
		}
		_, err := c.sdk.Library.DetectIntros(ctx, req)
		return err
	})
}

func (c *Client) DetectMetadataVoiceActivity(ctx context.Context, ids string, force bool, manual bool) error {
	return c.runMetadataSDKAction(ctx, "DetectVoiceActivity", ids, func(ctx context.Context, encodedIDs string) error {
		req := operations.DetectVoiceActivityRequest{Ids: encodedIDs}
		if force {
			req.Force = BoolIntPointer(true)
		}
		if manual {
			req.Manual = BoolIntPointer(true)
		}
		_, err := c.sdk.Library.DetectVoiceActivity(ctx, req)
		return err
	})
}

func (c *Client) GenerateBIF(ctx context.Context, ids string, force bool) error {
	return c.runMetadataSDKAction(ctx, "StartBifGeneration", ids, func(ctx context.Context, encodedIDs string) error {
		req := operations.StartBifGenerationRequest{Ids: encodedIDs}
		if force {
			req.Force = BoolIntPointer(true)
		}
		_, err := c.sdk.Library.StartBifGeneration(ctx, req)
		return err
	})
}

func (c *Client) DeleteSectionIntros(ctx context.Context, sectionID string) error {
	parsedSectionID, err := strconv.ParseInt(sectionID, 10, 64)
	if err != nil {
		return &PlexError{Op: "DeleteIntros", Section: sectionID, Err: fmt.Errorf("section ID must be numeric: %w", err)}
	}
	_, err = c.sdk.Library.DeleteIntros(ctx, operations.DeleteIntrosRequest{SectionID: parsedSectionID})
	return c.wrapSDKError("DeleteIntros", sectionID, err)
}

func (c *Client) DeleteStreamByID(ctx context.Context, streamID int64, ext string) error {
	pathWithExt := fmt.Sprintf("library/streams/%d.%s", streamID, ext)
	err := c.LibraryAction(ctx, "DeleteStream", http.MethodDelete, pathWithExt, nil)
	if err == nil {
		return nil
	}

	var plexErr *PlexError
	if errors.As(err, &plexErr) && strings.Contains(plexErr.Err.Error(), "unexpected status code: 400") {
		return c.LibraryAction(ctx, "DeleteStream", http.MethodDelete, fmt.Sprintf("library/streams/%d", streamID), nil)
	}

	return err
}

func (c *Client) SetStreamOffsetByID(ctx context.Context, streamID int64, ext string, offset int64) error {
	offsetValue := offset
	_, err := c.sdk.Library.SetStreamOffset(ctx, operations.SetStreamOffsetRequest{
		StreamID: streamID,
		Ext:      ext,
		Offset:   &offsetValue,
	})
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "Status 400") {
		query := url.Values{}
		query.Set("offset", strconv.FormatInt(offset, 10))
		return c.LibraryAction(ctx, "SetStreamOffset", http.MethodPut, fmt.Sprintf("library/streams/%d", streamID), query)
	}

	return c.wrapSDKError("SetStreamOffset", strconv.FormatInt(streamID, 10), err)
}

func (c *Client) CreateMarkerDynamic(ctx context.Context, ids string, markerType string, start int64, end *int64, attrs map[string]string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "CreateMarker", Err: err}
	}
	query := url.Values{}
	query.Set("type", markerType)
	query.Set("startTimeOffset", strconv.FormatInt(start, 10))
	if end != nil {
		query.Set("endTimeOffset", strconv.FormatInt(*end, 10))
	}
	addDeepObject(query, "attributes", attrs)
	return c.LibraryAction(ctx, "CreateMarker", http.MethodPost, fmt.Sprintf("library/metadata/%s/marker", encodedIDs), query)
}

func (c *Client) EditMarkerDynamic(ctx context.Context, ids, marker string, markerType string, start int64, end *int64, attrs map[string]string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "EditMarker", Err: err}
	}
	query := url.Values{}
	query.Set("type", markerType)
	query.Set("startTimeOffset", strconv.FormatInt(start, 10))
	if end != nil {
		query.Set("endTimeOffset", strconv.FormatInt(*end, 10))
	}
	addDeepObject(query, "attributes", attrs)
	return c.LibraryAction(ctx, "EditMarker", http.MethodPut, fmt.Sprintf("library/metadata/%s/marker/%s", encodedIDs, url.PathEscape(marker)), query)
}

func (c *Client) DeleteMarkerByID(ctx context.Context, ids, marker string) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: "DeleteMarker", Err: err}
	}
	_, err = c.sdk.Library.DeleteMarker(ctx, operations.DeleteMarkerRequest{Ids: encodedIDs, Marker: marker})
	return c.wrapSDKError("DeleteMarker", marker, err)
}

func (c *Client) libraryRequest(ctx context.Context, op, method, path string, query url.Values, accept string) ([]byte, error) {
	resp, err := c.libraryHTTPWithBody(ctx, op, method, path, query, accept, nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &PlexError{Op: op, Err: fmt.Errorf("failed to read response body: %w", err)}
	}

	return body, nil
}

func (c *Client) libraryHTTP(ctx context.Context, op, method, path string, query url.Values, accept string) (*http.Response, error) {
	return c.libraryHTTPWithBody(ctx, op, method, path, query, accept, nil, "")
}

func (c *Client) libraryActionWithBody(ctx context.Context, op, method, path string, query url.Values, body []byte, contentType string) error {
	resp, err := c.libraryHTTPWithBody(ctx, op, method, path, query, "*/*", body, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) libraryHTTPWithBody(ctx context.Context, op, method, path string, query url.Values, accept string, body []byte, contentType string) (*http.Response, error) {
	var resp *http.Response
	err := c.executeWithRetry(ctx, op, func() error {
		urlStr := c.libraryURL(path, query)
		var requestBody io.Reader
		if len(body) > 0 {
			requestBody = bytes.NewReader(body)
		}
		req, reqErr := http.NewRequestWithContext(ctx, method, urlStr, requestBody)
		if reqErr != nil {
			return fmt.Errorf("failed to create request: %w", reqErr)
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		httpResp, doErr := c.httpClient.Do(req)
		if doErr != nil {
			return fmt.Errorf("failed to make request: %w", doErr)
		}

		switch {
		case httpResp.StatusCode >= 200 && httpResp.StatusCode < 300:
			resp = httpResp
			return nil
		default:
			defer httpResp.Body.Close()
			body, _ := io.ReadAll(httpResp.Body)
			return libraryHTTPStatusError(op, httpResp.StatusCode, strings.TrimSpace(string(body)))
		}
	})
	if err != nil {
		return nil, &PlexError{Op: op, Err: err}
	}
	return resp, nil
}

func (c *Client) libraryURL(path string, query url.Values) string {
	base := strings.TrimSuffix(c.serverURL, "/")
	path = strings.TrimPrefix(path, "/")
	urlStr := fmt.Sprintf("%s/%s", base, path)
	params := url.Values{}
	for key, values := range query {
		for _, value := range values {
			params.Add(key, value)
		}
	}
	params.Set("X-Plex-Token", c.token)
	return urlStr + "?" + params.Encode()
}

func subtitleUploadContentType(format, title string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(title), "."))
	if ext == "" {
		ext = strings.ToLower(strings.TrimSpace(format))
	}

	switch ext {
	case "srt", "vtt", "ssa", "ass", "sub", "txt":
		return "text/plain;charset=UTF-8"
	default:
		return "application/octet-stream"
	}
}

func (c *Client) runMetadataSDKAction(ctx context.Context, op, ids string, fn func(context.Context, string) error) error {
	encodedIDs, err := encodeMetadataIDs(ids)
	if err != nil {
		return &PlexError{Op: op, Err: err}
	}
	err = fn(ctx, encodedIDs)
	if isAcceptedSDKResponse(err) {
		return nil
	}
	return c.wrapSDKError(op, encodedIDs, err)
}

func (c *Client) wrapSDKError(op, section string, err error) error {
	if err == nil {
		return nil
	}
	if friendly := librarySDKFriendlyError(op, err); friendly != nil {
		err = friendly
	}
	return &PlexError{Op: op, Section: section, Err: err}
}

func isAcceptedSDKResponse(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Status 202")
}

func libraryHTTPStatusError(op string, statusCode int, body string) error {
	if friendly := libraryHTTPFriendlyError(op, statusCode); friendly != "" {
		if body != "" {
			return fmt.Errorf("%s (%s)", friendly, body)
		}
		return errors.New(friendly)
	}
	return fmt.Errorf("unexpected status code: %d: %s", statusCode, body)
}

func libraryHTTPFriendlyError(op string, statusCode int) string {
	switch {
	case op == "GetStream" && statusCode == http.StatusNotImplemented:
		return "stream is not a downloadable sidecar subtitle stream"
	case op == "GetStreamLevels" && statusCode == http.StatusNotFound:
		return "stream levels are not available for this stream"
	case op == "GetStreamLoudness" && statusCode == http.StatusNotFound:
		return "stream loudness is not available for this stream"
	case op == "GetChapterImage" && statusCode == http.StatusNotFound:
		return "chapter image is not available for this media item and chapter"
	case op == "GetPartIndex" && statusCode == http.StatusNotFound:
		return "BIF index is not available for this media part"
	case op == "GetImageFromBif" && statusCode == http.StatusNotFound:
		return "BIF image is not available for this media part, index, or offset"
	case op == "GetFile" && statusCode == http.StatusBadRequest:
		return "bundle file URL is invalid for this metadata item"
	case op == "GetFile" && statusCode == http.StatusNotFound:
		return "bundle file is not available at that path"
	default:
		return ""
	}
}

func librarySDKFriendlyError(op string, err error) error {
	if err == nil {
		return nil
	}
	switch {
	case op == "SetStreamOffset" && strings.Contains(err.Error(), "Status 400"):
		return errors.New("stream offset update was rejected by Plex")
	case op == "GetStream" && strings.Contains(err.Error(), "Status 501"):
		return errors.New("stream is not a downloadable sidecar subtitle stream")
	default:
		return nil
	}
}

func addDeepObject(values url.Values, prefix string, fields map[string]string) {
	if len(fields) == 0 {
		return
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		values.Set(fmt.Sprintf("%s[%s]", prefix, key), fields[key])
	}
}

func BoolIntPointer(value bool) *components.BoolInt {
	if value {
		return components.BoolIntTrue.ToPointer()
	}
	return components.BoolIntFalse.ToPointer()
}
