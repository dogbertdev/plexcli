package plexclient

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type retryableTransportError struct{}

func (retryableTransportError) Error() string   { return "temporary transport failure" }
func (retryableTransportError) Temporary() bool { return true }
func (retryableTransportError) Timeout() bool   { return true }

func TestSDKBackedLibraryCallsUseSharedTransportAndRetries(t *testing.T) {
	client, err := NewClient("http://127.0.0.1:1", "token", WithTimeout(50*time.Millisecond), WithMaxRetries(1))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	attempts := 0
	client.httpClient.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, &url.Error{
				Op:  req.Method,
				URL: req.URL.String(),
				Err: retryableTransportError{},
			}
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     http.StatusText(http.StatusOK),
			Body:       io.NopCloser(strings.NewReader(`{}`)),
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
