// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package seaweedfs

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSeaweedFSClient(t *testing.T) {
	t.Parallel()

	client := NewSeaweedFSClient("http://localhost:8888")

	assert.NotNil(t, client)
	assert.Equal(t, "http://localhost:8888", client.baseURL)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, 30*time.Second, client.httpClient.Timeout)
}

func TestSeaweedFSClient_GetBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
	}{
		{
			name:    "Standard URL",
			baseURL: "http://localhost:8888",
		},
		{
			name:    "URL with port",
			baseURL: "http://seaweedfs.example.com:9333",
		},
		{
			name:    "HTTPS URL",
			baseURL: "https://secure-storage.example.com",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := NewSeaweedFSClient(tt.baseURL)
			assert.Equal(t, tt.baseURL, client.GetBaseURL())
		})
	}
}

func TestSeaweedFSClient_UploadFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Equal(t, "/bucket/test-file.txt", r.URL.Path)
		assert.Equal(t, "application/octet-stream", r.Header.Get("Content-Type"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Equal(t, "test content", string(body))

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)
	err := client.UploadFile(context.Background(), "/bucket/test-file.txt", []byte("test content"))
	require.NoError(t, err)
}

func TestSeaweedFSClient_UploadFileWithTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		ttl         string
		expectQuery string
	}{
		{
			name:        "5 minutes TTL",
			ttl:         "5m",
			expectQuery: "ttl=5m",
		},
		{
			name:        "1 hour TTL",
			ttl:         "1h",
			expectQuery: "ttl=1h",
		},
		{
			name:        "7 days TTL",
			ttl:         "7d",
			expectQuery: "ttl=7d",
		},
		{
			name:        "Empty TTL",
			ttl:         "",
			expectQuery: "",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPut, r.Method)
				if tt.ttl != "" {
					assert.Equal(t, tt.ttl, r.URL.Query().Get("ttl"))
				} else {
					assert.Empty(t, r.URL.Query().Get("ttl"))
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := NewSeaweedFSClient(server.URL)
			err := client.UploadFileWithTTL(context.Background(), "/test-file", []byte("content"), tt.ttl)
			require.NoError(t, err)
		})
	}
}

func TestSeaweedFSClient_UploadFile_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		response   string
	}{
		{
			name:       "Internal server error",
			statusCode: http.StatusInternalServerError,
			response:   "internal error",
		},
		{
			name:       "Bad request",
			statusCode: http.StatusBadRequest,
			response:   "bad request",
		},
		{
			name:       "Service unavailable",
			statusCode: http.StatusServiceUnavailable,
			response:   "service unavailable",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			client := NewSeaweedFSClient(server.URL)
			err := client.UploadFile(context.Background(), "/test-file", []byte("content"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "upload failed")
		})
	}
}

func TestSeaweedFSClient_DownloadFile(t *testing.T) {
	t.Parallel()

	expectedContent := "downloaded file content"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/bucket/download-file.txt", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedContent))
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)
	data, err := client.DownloadFile(context.Background(), "/bucket/download-file.txt")
	require.NoError(t, err)
	assert.Equal(t, expectedContent, string(data))
}

func TestSeaweedFSClient_DownloadFile_Error(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("file not found"))
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)
	_, err := client.DownloadFile(context.Background(), "/non-existent-file")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download failed")
}

func TestSeaweedFSClient_DeleteFile(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/bucket/delete-file.txt", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)
	err := client.DeleteFile(context.Background(), "/bucket/delete-file.txt")
	require.NoError(t, err)
}

func TestSeaweedFSClient_DeleteFile_NoContent(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)
	err := client.DeleteFile(context.Background(), "/bucket/delete-file.txt")
	require.NoError(t, err)
}

func TestSeaweedFSClient_DeleteFile_Error(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("delete failed"))
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)
	err := client.DeleteFile(context.Background(), "/bucket/delete-file.txt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

func TestSeaweedFSClient_HealthCheck(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/status", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)
	err := client.HealthCheck(context.Background())
	require.NoError(t, err)
}

func TestSeaweedFSClient_HealthCheck_Error(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)
	err := client.HealthCheck(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "health check failed")
}

func TestSeaweedFSClient_ContextCancellation(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := client.UploadFile(ctx, "/test-file", []byte("content"))
	require.Error(t, err)
}

func TestSeaweedFSClient_UploadFile_Created(t *testing.T) {
	t.Parallel()

	// Test that HTTP 201 Created is also accepted
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)
	err := client.UploadFile(context.Background(), "/new-file", []byte("content"))
	require.NoError(t, err)
}

func TestSeaweedFSClient_LargeFile(t *testing.T) {
	t.Parallel()

	// Create a 1MB file
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}

	// Use a channel to safely communicate the received size from the handler goroutine
	receivedSizeChan := make(chan int, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedSizeChan <- len(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewSeaweedFSClient(server.URL)
	err := client.UploadFile(context.Background(), "/large-file", largeContent)
	require.NoError(t, err)

	receivedSize := <-receivedSizeChan
	assert.Equal(t, len(largeContent), receivedSize)
}

// ---------------------------------------------------------------------------
// Tests covering invalid URL paths (NewRequestWithContext failure)
// ---------------------------------------------------------------------------

func TestSeaweedFSClient_DownloadFile_InvalidURL(t *testing.T) {
	t.Parallel()

	client := NewSeaweedFSClient("http://\x00invalid")
	_, err := client.DownloadFile(context.Background(), "/some-file")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestSeaweedFSClient_DeleteFile_InvalidURL(t *testing.T) {
	t.Parallel()

	client := NewSeaweedFSClient("http://\x00invalid")
	err := client.DeleteFile(context.Background(), "/some-file")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create request")
}

func TestSeaweedFSClient_HealthCheck_InvalidURL(t *testing.T) {
	t.Parallel()

	client := NewSeaweedFSClient("http://\x00invalid")
	err := client.HealthCheck(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create health check request")
}

// ---------------------------------------------------------------------------
// Tests covering network error paths (connection refused)
// ---------------------------------------------------------------------------

func TestSeaweedFSClient_DownloadFile_NetworkError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close() // Close immediately so connections are refused

	client := NewSeaweedFSClient(server.URL)
	_, err := client.DownloadFile(context.Background(), "/some-file")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to download file")
}

func TestSeaweedFSClient_DeleteFile_NetworkError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := NewSeaweedFSClient(server.URL)
	err := client.DeleteFile(context.Background(), "/some-file")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete file")
}

func TestSeaweedFSClient_HealthCheck_NetworkError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	client := NewSeaweedFSClient(server.URL)
	err := client.HealthCheck(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "health check failed")
}
