// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/reporter/pkg/seaweedfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSeaweedFSAdapter(t *testing.T) {
	t.Parallel()

	client := seaweedfs.NewSeaweedFSClient("http://localhost:8888")
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	assert.NotNil(t, adapter)
	assert.Equal(t, "test-bucket", adapter.bucket)
	assert.Equal(t, client, adapter.client)
}

func TestSeaweedFSAdapter_Upload(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Contains(t, r.URL.Path, "/test-bucket/test-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := seaweedfs.NewSeaweedFSClient(server.URL)
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	data := []byte("test content")
	reader := bytes.NewReader(data)

	key, err := adapter.Upload(context.Background(), "test-key", reader, "text/plain")
	require.NoError(t, err)
	assert.Equal(t, "test-key", key)
}

func TestSeaweedFSAdapter_UploadWithTTL(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		assert.Contains(t, r.URL.Path, "/test-bucket/ttl-key")
		assert.Equal(t, "5m", r.URL.Query().Get("ttl"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := seaweedfs.NewSeaweedFSClient(server.URL)
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	data := []byte("test content with ttl")
	reader := bytes.NewReader(data)

	key, err := adapter.UploadWithTTL(context.Background(), "ttl-key", reader, "text/plain", "5m")
	require.NoError(t, err)
	assert.Equal(t, "ttl-key", key)
}

func TestSeaweedFSAdapter_Download(t *testing.T) {
	t.Parallel()

	expectedContent := "downloaded content"

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Contains(t, r.URL.Path, "/test-bucket/download-key")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedContent))
	}))
	defer server.Close()

	client := seaweedfs.NewSeaweedFSClient(server.URL)
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	reader, err := adapter.Download(context.Background(), "download-key")
	require.NoError(t, err)
	assert.NotNil(t, reader)
	defer reader.Close()

	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, string(content))
}

func TestSeaweedFSAdapter_Delete(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Contains(t, r.URL.Path, "/test-bucket/delete-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := seaweedfs.NewSeaweedFSClient(server.URL)
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	err := adapter.Delete(context.Background(), "delete-key")
	require.NoError(t, err)
}

func TestSeaweedFSAdapter_Exists_True(t *testing.T) {
	t.Parallel()

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("file content"))
	}))
	defer server.Close()

	client := seaweedfs.NewSeaweedFSClient(server.URL)
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	exists, err := adapter.Exists(context.Background(), "existing-key")
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestSeaweedFSAdapter_Exists_False(t *testing.T) {
	t.Parallel()

	// Create a test server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("status 404"))
	}))
	defer server.Close()

	client := seaweedfs.NewSeaweedFSClient(server.URL)
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	exists, err := adapter.Exists(context.Background(), "non-existing-key")
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestSeaweedFSAdapter_GeneratePresignedURL(t *testing.T) {
	t.Parallel()

	baseURL := "http://localhost:8888"
	client := seaweedfs.NewSeaweedFSClient(baseURL)
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	url, err := adapter.GeneratePresignedURL(context.Background(), "my-file.pdf", 1*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, "http://localhost:8888/test-bucket/my-file.pdf", url)
}

func TestSeaweedFSAdapter_Upload_Error(t *testing.T) {
	t.Parallel()

	// Create a test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("upload failed"))
	}))
	defer server.Close()

	client := seaweedfs.NewSeaweedFSClient(server.URL)
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	data := []byte("test content")
	reader := bytes.NewReader(data)

	_, err := adapter.Upload(context.Background(), "fail-key", reader, "text/plain")
	require.Error(t, err)
}

func TestSeaweedFSAdapter_Download_Error(t *testing.T) {
	t.Parallel()

	// Create a test server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("download failed"))
	}))
	defer server.Close()

	client := seaweedfs.NewSeaweedFSClient(server.URL)
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	_, err := adapter.Download(context.Background(), "fail-key")
	require.Error(t, err)
}

func TestSeaweedFSAdapter_ImplementsInterface(t *testing.T) {
	t.Parallel()

	client := seaweedfs.NewSeaweedFSClient("http://localhost:8888")
	adapter := NewSeaweedFSAdapter(client, "test-bucket")

	// Verify adapter implements ObjectStorage interface
	var _ ObjectStorage = adapter
}

func TestSeaweedFSAdapter_PathBuilding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		bucket     string
		key        string
		expectPath string
	}{
		{
			name:       "Simple key",
			bucket:     "bucket",
			key:        "file.txt",
			expectPath: "/bucket/file.txt",
		},
		{
			name:       "Key with subdirectory",
			bucket:     "reports",
			key:        "2024/01/report.pdf",
			expectPath: "/reports/2024/01/report.pdf",
		},
		{
			name:       "Key with special characters",
			bucket:     "templates",
			key:        "report_v2.0_final.tpl",
			expectPath: "/templates/report_v2.0_final.tpl",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var capturedPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := seaweedfs.NewSeaweedFSClient(server.URL)
			adapter := NewSeaweedFSAdapter(client, tt.bucket)

			_, _ = adapter.Upload(context.Background(), tt.key, bytes.NewReader([]byte("test")), "text/plain")

			assert.Equal(t, tt.expectPath, capturedPath)
		})
	}
}
