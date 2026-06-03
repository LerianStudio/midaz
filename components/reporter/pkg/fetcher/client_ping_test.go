// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetcherClient_Ping_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "200 OK is success", statusCode: http.StatusOK},
		{name: "204 No Content is success", statusCode: http.StatusNoContent},
		{name: "299 boundary is success", statusCode: 299},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodGet, r.Method)
				assert.Equal(t, "/readyz", r.URL.Path)
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			c := NewFetcherClient(srv.URL)
			err := c.Ping(context.Background())
			assert.NoError(t, err)
		})
	}
}

func TestFetcherClient_Ping_NonSuccessStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "300 redirect is error", statusCode: http.StatusMultipleChoices},
		{name: "404 not found is error", statusCode: http.StatusNotFound},
		{name: "500 internal server error is error", statusCode: http.StatusInternalServerError},
		{name: "503 service unavailable is error", statusCode: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			c := NewFetcherClient(srv.URL)
			err := c.Ping(context.Background())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "fetcher /readyz returned")
		})
	}
}

func TestFetcherClient_Ping_NetworkError(t *testing.T) {
	t.Parallel()

	// Use an immediately-closed server so the dial will fail.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	srv.Close()

	c := NewFetcherClient(srv.URL)
	err := c.Ping(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execute ping request")
}

// TestFetcherClient_Ping_RedactsCredentialsInError verifies that when the
// configured baseURL contains userinfo (user:password@host) and the dial
// fails, the resulting error message does NOT leak the credentials. The
// net/http.Client error format embeds the URL in quotes, so we must redact
// before returning.
func TestFetcherClient_Ping_RedactsCredentialsInError(t *testing.T) {
	t.Parallel()

	// Pick an address that immediately refuses connection; doesn't matter
	// which port — we're only checking the error string.
	c := NewFetcherClient("http://alice:hunter2@127.0.0.1:1")

	err := c.Ping(context.Background())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "alice",
		"username must not appear in the returned error")
	assert.NotContains(t, err.Error(), "hunter2",
		"password must not appear in the returned error")
}

func TestFetcherClient_Ping_ContextCancelled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Hold the server long enough that the context cancellation triggers.
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewFetcherClient(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := c.Ping(ctx)
	require.Error(t, err)
}

func TestFetcherClient_Ping_MalformedBaseURL(t *testing.T) {
	t.Parallel()

	c := NewFetcherClient("://not-a-url")

	err := c.Ping(context.Background())
	require.Error(t, err)
}
