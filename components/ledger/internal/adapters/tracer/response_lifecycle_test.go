// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package tracer

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResponseContextLivesUntilBodyCloses(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, "complete body")
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(server.Close)
	client, err := NewTracerClient(server.URL, WithOperationTimeout(5*time.Second))
	require.NoError(t, err)
	response, err := client.post(t.Context(), "/v1/reservations", []byte(`{}`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = response.Body.Close() })
	require.NoError(t, response.Request.Context().Err(), "receiving headers must not cancel body reads")
	close(release)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, "complete body", string(body))
	require.NoError(t, response.Body.Close())
	require.ErrorIs(t, response.Request.Context().Err(), context.Canceled)
}

func TestResponseBodyRetainsOperationDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	client, err := NewTracerClient(server.URL, WithOperationTimeout(200*time.Millisecond))
	require.NoError(t, err)
	response, err := client.post(t.Context(), "/v1/reservations", nil)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	_, err = io.ReadAll(response.Body)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
