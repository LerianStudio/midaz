// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fetcher

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Extraction error tests ---

func TestFetcherClient_CreateExtractionJob_ServerError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":"INTERNAL","message":"extraction failed"}`))
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	req := CreateExtractionJobRequest{
		DataRequest: ExtractionDataRequest{
			MappedFields: map[string]map[string][]string{
				"ds-1": {"users": {"id"}},
			},
		},
		Metadata: map[string]any{"source": "reporter"},
	}

	_, err := client.CreateExtractionJob(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestFetcherClient_CreateExtractionJob_NetworkError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := NewFetcherClient(server.URL)

	req := CreateExtractionJobRequest{
		DataRequest: ExtractionDataRequest{
			MappedFields: map[string]map[string][]string{
				"ds-1": {"users": {"id"}},
			},
		},
		Metadata: map[string]any{"source": "reporter"},
	}

	_, err := client.CreateExtractionJob(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute request")
}

// --- GetExtractionJobStatus error tests ---

func TestFetcherClient_GetExtractionJobStatus_NotFound(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(ErrorResponse{Code: "NOT_FOUND", Message: "job not found"})
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	_, err := client.GetExtractionJobStatus(context.Background(), "nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestFetcherClient_GetExtractionJobStatus_NetworkError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := NewFetcherClient(server.URL)

	_, err := client.GetExtractionJobStatus(context.Background(), "job-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute request")
}

// --- Timeout tests for extraction ---

func TestFetcherClient_CreateExtractionJob_ContextTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := CreateExtractionJobRequest{
		DataRequest: ExtractionDataRequest{
			MappedFields: map[string]map[string][]string{
				"ds-1": {"users": {"id"}},
			},
		},
		Metadata: map[string]any{"source": "reporter"},
	}

	_, err := client.CreateExtractionJob(ctx, req)
	require.Error(t, err)
}

func TestFetcherClient_GetExtractionJobStatus_ContextTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.GetExtractionJobStatus(ctx, "job-001")
	require.Error(t, err)
}

// --- Invalid JSON response tests for extraction ---

func TestFetcherClient_CreateExtractionJob_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{invalid`))
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	req := CreateExtractionJobRequest{
		DataRequest: ExtractionDataRequest{
			MappedFields: map[string]map[string][]string{
				"ds-1": {"users": {"id"}},
			},
		},
		Metadata: map[string]any{"source": "reporter"},
	}

	_, err := client.CreateExtractionJob(context.Background(), req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode response")
}

func TestFetcherClient_GetExtractionJobStatus_InvalidJSON(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	_, err := client.GetExtractionJobStatus(context.Background(), "job-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode response")
}
