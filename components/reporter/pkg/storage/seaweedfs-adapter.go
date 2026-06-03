// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/LerianStudio/reporter/pkg/seaweedfs"
)

// SeaweedFSAdapter wraps the existing SeaweedFS client to implement ObjectStorage interface.
// This provides compatibility between the HTTP-based SeaweedFS client and the storage abstraction.
type SeaweedFSAdapter struct {
	client *seaweedfs.SeaweedFSClient
	bucket string
}

// NewSeaweedFSAdapter creates a new SeaweedFS adapter.
func NewSeaweedFSAdapter(client *seaweedfs.SeaweedFSClient, bucket string) *SeaweedFSAdapter {
	return &SeaweedFSAdapter{
		client: client,
		bucket: bucket,
	}
}

// Upload stores content from a reader at the given key.
func (a *SeaweedFSAdapter) Upload(ctx context.Context, key string, reader io.Reader, contentType string) (string, error) {
	return a.UploadWithTTL(ctx, key, reader, contentType, "")
}

// UploadWithTTL stores content with a time-to-live.
// TTL format: 3m (3 minutes), 4h (4 hours), 5d (5 days), 6w (6 weeks), 7M (7 months), 8y (8 years)
// If ttl is empty string, no TTL is applied and the file will be stored permanently
func (a *SeaweedFSAdapter) UploadWithTTL(ctx context.Context, key string, reader io.Reader, contentType string, ttl string) (string, error) {
	// Read data from reader
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("reading data: %w", err)
	}

	// Build the full path: /bucket/key
	path := fmt.Sprintf("/%s/%s", a.bucket, key)

	// Upload to SeaweedFS with TTL
	if err := a.client.UploadFileWithTTL(ctx, path, data, ttl); err != nil {
		return "", err
	}

	return key, nil
}

// Download retrieves content from the given key.
func (a *SeaweedFSAdapter) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	// Build the full path: /bucket/key
	path := fmt.Sprintf("/%s/%s", a.bucket, key)

	// Download from SeaweedFS
	data, err := a.client.DownloadFile(ctx, path)
	if err != nil {
		return nil, err
	}

	// Return as ReadCloser
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Delete removes an object by key.
func (a *SeaweedFSAdapter) Delete(ctx context.Context, key string) error {
	// Build the full path: /bucket/key
	path := fmt.Sprintf("/%s/%s", a.bucket, key)

	// Delete from SeaweedFS
	return a.client.DeleteFile(ctx, path)
}

// Exists checks if an object exists at the given key.
func (a *SeaweedFSAdapter) Exists(ctx context.Context, key string) (bool, error) {
	// Try to download the file to check existence
	_, err := a.Download(ctx, key)
	if err != nil {
		// SeaweedFS returns 404 status in error message when file doesn't exist
		if strings.Contains(err.Error(), "status 404") {
			return false, nil
		}
		// For other errors, propagate them
		return false, err
	}

	return true, nil
}

// GeneratePresignedURL creates a time-limited download URL.
// Note: SeaweedFS HTTP mode does not support presigned URLs.
// This returns the direct URL which requires no authentication.
func (a *SeaweedFSAdapter) GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	// SeaweedFS HTTP mode doesn't support presigned URLs
	// Return direct URL (note: this URL doesn't expire!)
	path := fmt.Sprintf("/%s/%s", a.bucket, key)
	baseURL := a.client.GetBaseURL()
	url := fmt.Sprintf("%s%s", baseURL, path)

	return url, nil
}

// Compile-time interface check.
var _ ObjectStorage = (*SeaweedFSAdapter)(nil)
