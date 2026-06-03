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

// --- CreateExtractionJob tests ---

func TestFetcherClient_CreateExtractionJob_Success(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/v1/fetcher", r.URL.Path)

		var reqBody CreateExtractionJobRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&reqBody))
		assert.Contains(t, reqBody.DataRequest.MappedFields, "ds-1")
		assert.Contains(t, reqBody.DataRequest.MappedFields["ds-1"]["users"], "id")
		assert.Contains(t, reqBody.DataRequest.MappedFields["ds-1"]["users"], "name")

		resp := ExtractionJobResponse{
			JobID:     "job-001",
			Status:    "pending",
			CreatedAt: now,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	req := CreateExtractionJobRequest{
		DataRequest: ExtractionDataRequest{
			MappedFields: map[string]map[string][]string{
				"ds-1": {"users": {"id", "name"}},
			},
		},
		Metadata: map[string]any{"source": "reporter"},
	}

	job, err := client.CreateExtractionJob(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "job-001", job.JobID)
	assert.Equal(t, "pending", job.Status)
}

func TestFetcherClient_CreateExtractionJob_WithAuth(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer m2m-token", r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-Organization-Id"))
		assert.Empty(t, r.Header.Get("X-Tenant-ID"))

		resp := ExtractionJobResponse{
			JobID:     "job-002",
			Status:    "pending",
			CreatedAt: time.Now().UTC(),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFetcherClient(
		server.URL,
		WithM2MTokenProvider(&stubM2MProvider{token: "m2m-token"}),
	)

	req := CreateExtractionJobRequest{
		DataRequest: ExtractionDataRequest{
			MappedFields: map[string]map[string][]string{
				"ds-1": {"users": {"id"}},
			},
		},
		Metadata: map[string]any{"source": "reporter"},
	}

	job, err := client.CreateExtractionJob(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "job-002", job.JobID)
}

// --- GetExtractionJobStatus tests ---

func TestFetcherClient_GetExtractionJobStatus_Success(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	completed := now.Add(5 * time.Minute)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/v1/fetcher/job-001", r.URL.Path)

		resp := ExtractionJobResponse{
			JobID:       "job-001",
			Status:      "completed",
			CreatedAt:   now,
			CompletedAt: &completed,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	job, err := client.GetExtractionJobStatus(context.Background(), "job-001")
	require.NoError(t, err)
	assert.Equal(t, "job-001", job.JobID)
	assert.Equal(t, "completed", job.Status)
	assert.NotNil(t, job.CompletedAt)
}

func TestFetcherClient_GetExtractionJobStatus_Pending(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ExtractionJobResponse{
			JobID:     "job-003",
			Status:    "pending",
			CreatedAt: time.Now().UTC(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewFetcherClient(server.URL)

	job, err := client.GetExtractionJobStatus(context.Background(), "job-003")
	require.NoError(t, err)
	assert.Equal(t, "pending", job.Status)
	assert.Nil(t, job.CompletedAt)
}
