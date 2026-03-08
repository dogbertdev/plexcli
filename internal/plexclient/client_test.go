package plexclient

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSDKBackedLibraryCallsUseSharedTransportAndRetries(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:1", "token", WithTimeout(50*time.Millisecond), WithMaxRetries(1))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	attempts := 0
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		statusCode := http.StatusOK
		body := ""
		if attempts == 1 {
			statusCode = http.StatusServiceUnavailable
		} else {
			body = `{}`
		}

		return &http.Response{
			StatusCode: statusCode,
			Status:     http.StatusText(statusCode),
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Request:    req,
		}, nil
	})

	if err := client.StopAllRefreshes(context.Background()); err != nil {
		t.Fatalf("StopAllRefreshes() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("expected shared transport with one retry, got %d attempts", attempts)
	}
}
