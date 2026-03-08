package plexclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (t *trackingReadCloser) Close() error {
	t.closed = true
	return nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestParseCSVList(t *testing.T) {
	got, err := ParseCSVList("1, 2,3")
	if err != nil {
		t.Fatalf("ParseCSVList() error = %v", err)
	}
	if len(got) != 3 || got[0] != "1" || got[1] != "2" || got[2] != "3" {
		t.Fatalf("unexpected ParseCSVList() result: %#v", got)
	}

	if _, err := ParseCSVList("1,,2"); err == nil {
		t.Fatalf("expected ParseCSVList() to reject empty segments")
	}
}

func TestParseKeyValuePairs(t *testing.T) {
	got, err := ParseKeyValuePairs([]string{"title=Movie", "summary=Hello"})
	if err != nil {
		t.Fatalf("ParseKeyValuePairs() error = %v", err)
	}
	if got["title"] != "Movie" || got["summary"] != "Hello" {
		t.Fatalf("unexpected ParseKeyValuePairs() result: %#v", got)
	}

	if _, err := ParseKeyValuePairs([]string{"broken"}); err == nil {
		t.Fatalf("expected ParseKeyValuePairs() to reject invalid input")
	}
}

func TestRequireGuards(t *testing.T) {
	if err := RequireConfirmation(false, "delete"); err == nil {
		t.Fatalf("expected RequireConfirmation() error")
	}
	if err := RequireOutputPath(""); err == nil {
		t.Fatalf("expected RequireOutputPath() error")
	}
}

func TestIsAcceptedSDKResponse(t *testing.T) {
	if !isAcceptedSDKResponse(errors.New("unknown status code returned: Status 202")) {
		t.Fatalf("expected Status 202 to be treated as success")
	}
	if isAcceptedSDKResponse(errors.New("unknown status code returned: Status 400")) {
		t.Fatalf("did not expect Status 400 to be treated as success")
	}
	if isAcceptedSDKResponse(nil) {
		t.Fatalf("did not expect nil to be treated as success")
	}
}

func TestLibraryHTTPFriendlyError(t *testing.T) {
	tests := []struct {
		op     string
		status int
		want   string
	}{
		{op: "GetStreamLevels", status: http.StatusNotFound, want: "stream levels are not available for this stream"},
		{op: "GetStreamLoudness", status: http.StatusNotFound, want: "stream loudness is not available for this stream"},
		{op: "GetChapterImage", status: http.StatusNotFound, want: "chapter image is not available for this media item and chapter"},
		{op: "GetPartIndex", status: http.StatusNotFound, want: "BIF index is not available for this media part"},
		{op: "GetImageFromBif", status: http.StatusNotFound, want: "BIF image is not available for this media part, index, or offset"},
		{op: "GetStream", status: http.StatusNotImplemented, want: "stream is not a downloadable sidecar subtitle stream"},
	}

	for _, tt := range tests {
		if got := libraryHTTPFriendlyError(tt.op, tt.status); got != tt.want {
			t.Fatalf("%s/%d: expected %q, got %q", tt.op, tt.status, tt.want, got)
		}
	}
}

func TestLibrarySDKFriendlyError(t *testing.T) {
	if got := librarySDKFriendlyError("DetectCredits", errors.New("API error occurred: Status 400")); got != nil {
		t.Fatalf("did not expect friendly error for DetectCredits: %v", got)
	}
	if got := librarySDKFriendlyError("DetectIntros", errors.New("API error occurred: Status 400")); got != nil {
		t.Fatalf("did not expect friendly error for DetectIntros: %v", got)
	}
}

func TestLibraryActionWithBodyClosesResponseBody(t *testing.T) {
	client, err := NewClient("http://example.com", "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	body := &trackingReadCloser{Reader: strings.NewReader("ok")}
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Header:     make(http.Header),
		}, nil
	})

	err = client.libraryActionWithBody(context.Background(), "AddSubtitles", http.MethodPost, "library/metadata/1/subtitles", nil, []byte("payload"), "text/plain")
	if err != nil {
		t.Fatalf("libraryActionWithBody() error = %v", err)
	}
	if !body.closed {
		t.Fatalf("expected response body to be closed")
	}
}

func TestEditMetadataDynamicBuildsExpectedQuery(t *testing.T) {
	var seenPath string
	var seenQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.EditMetadataDynamic(context.Background(), "1,2", MetadataEditInput{
		Set:    map[string]string{"title": "Updated"},
		Lock:   []string{"title"},
		Unlock: []string{"summary"},
	})
	if err != nil {
		t.Fatalf("EditMetadataDynamic() error = %v", err)
	}

	if seenPath != "/library/metadata/1,2" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got := seenQuery.Get("title.value"); got != "Updated" {
		t.Fatalf("expected title.value query, got %q", got)
	}
	if got := seenQuery.Get("title.locked"); got != "1" {
		t.Fatalf("expected title.locked=1, got %q", got)
	}
	if got := seenQuery.Get("summary.locked"); got != "0" {
		t.Fatalf("expected summary.locked=0, got %q", got)
	}
}

func TestCreateSectionBuildsExpectedQuery(t *testing.T) {
	var seenPath string
	var seenQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.CreateSection(context.Background(), SectionMutationInput{
		Name:      "Movies",
		Type:      1,
		Agent:     "com.plexapp.agents.imdb",
		Language:  "en-US",
		Locations: []string{"/media/movies"},
		Prefs:     map[string]string{"scanner": "plex"},
	})
	if err != nil {
		t.Fatalf("CreateSection() error = %v", err)
	}

	if seenPath != "/library/sections" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got := seenQuery.Get("name"); got != "Movies" {
		t.Fatalf("expected name query, got %q", got)
	}
	if got := seenQuery.Get("locations"); got != "/media/movies" {
		t.Fatalf("expected locations query, got %q", got)
	}
	if got := seenQuery.Get("prefs[scanner]"); got != "plex" {
		t.Fatalf("expected prefs[scanner], got %q", got)
	}
}

func TestDetectCreditsIncludesForceAndManual(t *testing.T) {
	var seenPath string
	var seenQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.DetectMetadataCredits(context.Background(), "47648", true, true)
	if err != nil {
		t.Fatalf("DetectMetadataCredits() error = %v", err)
	}

	if seenPath != "/library/metadata/47648/credits" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got := seenQuery.Get("force"); got != "1" {
		t.Fatalf("expected force=1, got %q", got)
	}
	if got := seenQuery.Get("manual"); got != "1" {
		t.Fatalf("expected manual=1, got %q", got)
	}
}

func TestDetectIntrosIncludesForceAndThreshold(t *testing.T) {
	var seenPath string
	var seenQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	threshold := 0.42
	err = client.DetectMetadataIntros(context.Background(), "50490", true, &threshold)
	if err != nil {
		t.Fatalf("DetectMetadataIntros() error = %v", err)
	}

	if seenPath != "/library/metadata/50490/intro" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got := seenQuery.Get("force"); got != "1" {
		t.Fatalf("expected force=1, got %q", got)
	}
	if got := seenQuery.Get("threshold"); got != "0.42" {
		t.Fatalf("expected threshold=0.42, got %q", got)
	}
}

func TestSetStreamOffsetFallsBackToPathWithoutExtension(t *testing.T) {
	requests := make([]string, 0, 2)
	var fallbackQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		switch r.URL.Path {
		case "/library/streams/281565.srt":
			http.Error(w, "bad request", http.StatusBadRequest)
		case "/library/streams/281565":
			fallbackQuery = r.URL.Query()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.SetStreamOffsetByID(context.Background(), 281565, "srt", 0)
	if err != nil {
		t.Fatalf("SetStreamOffsetByID() error = %v", err)
	}

	if len(requests) != 2 {
		t.Fatalf("expected two requests, got %d", len(requests))
	}
	if requests[0] != "/library/streams/281565.srt" {
		t.Fatalf("unexpected first request: %s", requests[0])
	}
	if requests[1] != "/library/streams/281565" {
		t.Fatalf("unexpected fallback request: %s", requests[1])
	}
	if got := fallbackQuery.Get("offset"); got != "0" {
		t.Fatalf("expected offset=0 on fallback, got %q", got)
	}
}

func TestAddSubtitlesFromFileUsesPostBody(t *testing.T) {
	var seenMethod string
	var seenPath string
	var seenBody []byte
	var seenQuery url.Values
	var seenContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		seenContentType = r.Header.Get("Content-Type")
		seenBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.AddSubtitlesFromFile(context.Background(), "1", []byte("subtitle body"), "eng", "English", 0, "srt", true, false)
	if err != nil {
		t.Fatalf("AddSubtitlesFromFile() error = %v", err)
	}

	if seenMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", seenMethod)
	}
	if seenPath != "/library/metadata/1/subtitles" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if string(seenBody) != "subtitle body" {
		t.Fatalf("unexpected body: %q", string(seenBody))
	}
	if got := seenQuery.Get("language"); got != "eng" {
		t.Fatalf("expected language query, got %q", got)
	}
	if got := seenQuery.Get("title"); got != "English" {
		t.Fatalf("expected title query, got %q", got)
	}
	if seenContentType != "text/plain;charset=UTF-8" {
		t.Fatalf("expected text subtitle content-type, got %q", seenContentType)
	}
}

func TestGetSectionsPrefsBuildsExpectedQuery(t *testing.T) {
	var seenPath string
	var seenQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"MediaContainer":{"Setting":[]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if _, err := client.GetSectionsPrefs(context.Background(), 1, "agent"); err != nil {
		t.Fatalf("GetSectionsPrefs() error = %v", err)
	}

	if seenPath != "/library/sections/prefs" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got := seenQuery.Get("type"); got != "1" {
		t.Fatalf("expected type=1, got %q", got)
	}
	if got := seenQuery.Get("agent"); got != "agent" {
		t.Fatalf("expected agent query, got %q", got)
	}
}

func TestSetSectionPreferencesDynamicUsesFlatQuery(t *testing.T) {
	var seenPath string
	var seenQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.SetSectionPreferencesDynamic(context.Background(), "10", map[string]string{"hidden": "0"})
	if err != nil {
		t.Fatalf("SetSectionPreferencesDynamic() error = %v", err)
	}

	if seenPath != "/library/sections/10/prefs" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got := seenQuery.Get("hidden"); got != "0" {
		t.Fatalf("expected hidden query, got %q", got)
	}
	if got := seenQuery.Get("prefs[hidden]"); got != "" {
		t.Fatalf("expected no nested prefs query, got %q", got)
	}
}

func TestSetItemPreferencesDynamicUsesFlatQuery(t *testing.T) {
	var seenPath string
	var seenQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.SetItemPreferencesDynamic(context.Background(), "47648", map[string]string{"useOriginalTitle": "-1"})
	if err != nil {
		t.Fatalf("SetItemPreferencesDynamic() error = %v", err)
	}

	if seenPath != "/library/metadata/47648/prefs" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got := seenQuery.Get("useOriginalTitle"); got != "-1" {
		t.Fatalf("expected flat item pref query, got %q", got)
	}
	if got := seenQuery.Get("prefs[useOriginalTitle]"); got != "" {
		t.Fatalf("expected no nested prefs query, got %q", got)
	}
}

func TestUpdateItemsDynamicIncludesSectionMediaType(t *testing.T) {
	var seenPath string
	var seenQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.UpdateItemsDynamic(context.Background(), BulkUpdateInput{
		SectionID: "10",
		MediaType: 1,
		Filter:    "id=47648",
		Set:       map[string]string{"title": "Alien Nation"},
	})
	if err != nil {
		t.Fatalf("UpdateItemsDynamic() error = %v", err)
	}

	if seenPath != "/library/sections/10/all" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got := seenQuery.Get("type"); got != "1" {
		t.Fatalf("expected type query, got %q", got)
	}
	if got := seenQuery.Get("filters"); got != "id=47648" {
		t.Fatalf("expected filters query, got %q", got)
	}
	if got := seenQuery.Get("title.value"); got != "Alien Nation" {
		t.Fatalf("expected title.value query, got %q", got)
	}
}

func TestSetItemArtworkByURLUsesPut(t *testing.T) {
	var seenMethod string
	var seenPath string
	var seenQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.SetItemArtworkByURL(context.Background(), "47648", "poster", "https://example.com/poster.jpg")
	if err != nil {
		t.Fatalf("SetItemArtworkByURL() error = %v", err)
	}

	if seenMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %s", seenMethod)
	}
	if seenPath != "/library/metadata/47648/poster" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got := seenQuery.Get("url"); got != "https://example.com/poster.jpg" {
		t.Fatalf("expected url query, got %q", got)
	}
}

func TestCreateMarkerDynamicUsesStringType(t *testing.T) {
	var seenMethod string
	var seenPath string
	var seenQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.CreateMarkerDynamic(context.Background(), "47648", "bookmark", 1000, nil, nil)
	if err != nil {
		t.Fatalf("CreateMarkerDynamic() error = %v", err)
	}

	if seenMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", seenMethod)
	}
	if seenPath != "/library/metadata/47648/marker" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got := seenQuery.Get("type"); got != "bookmark" {
		t.Fatalf("expected string marker type, got %q", got)
	}
}

func TestEditSectionFillsAgentAndScannerFromExistingSection(t *testing.T) {
	var seenPutQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/library/sections":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"MediaContainer":{"Directory":[{"key":"16","agent":"tv.plex.agents.movie","scanner":"Plex Movie"}]}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/library/sections/16":
			seenPutQuery = r.URL.Query()
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.EditSection(context.Background(), "16", SectionMutationInput{Name: "Renamed"})
	if err != nil {
		t.Fatalf("EditSection() error = %v", err)
	}

	if got := seenPutQuery.Get("name"); got != "Renamed" {
		t.Fatalf("expected name query, got %q", got)
	}
	if got := seenPutQuery.Get("agent"); got != "tv.plex.agents.movie" {
		t.Fatalf("expected agent query, got %q", got)
	}
	if got := seenPutQuery.Get("scanner"); got != "Plex Movie" {
		t.Fatalf("expected scanner query, got %q", got)
	}
}

func TestDeleteStreamFallsBackToPathWithoutExtension(t *testing.T) {
	requests := make([]string, 0, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		switch r.URL.Path {
		case "/library/streams/99.srt":
			http.Error(w, "bad request", http.StatusBadRequest)
		case "/library/streams/99":
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "token")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if err := client.DeleteStreamByID(context.Background(), 99, "srt"); err != nil {
		t.Fatalf("DeleteStreamByID() error = %v", err)
	}

	want := []string{"/library/streams/99.srt", "/library/streams/99"}
	if len(requests) != len(want) {
		t.Fatalf("unexpected request count: got %d want %d", len(requests), len(want))
	}
	for i := range want {
		if requests[i] != want[i] {
			t.Fatalf("unexpected request at %d: got %q want %q", i, requests[i], want[i])
		}
	}
}
