// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package storage defines interfaces for object storage operations.
package storage

//go:generate mockgen --destination=ports.mock.go --package=storage --copyright_file=../../COPYRIGHT . ObjectStorage

import (
	"context"
	"io"
	"time"
)

// ObjectStorage provides object storage operations for templates and reports.
// This interface abstracts storage backends (SeaweedFS, S3, MinIO, etc.)
type ObjectStorage interface {
	// Upload stores content from a reader at the given key.
	// Returns the final key and any error.
	Upload(ctx context.Context, key string, reader io.Reader, contentType string) (string, error)

	// UploadWithTTL stores content with a time-to-live.
	// TTL format: 3m (3 minutes), 4h (4 hours), 5d (5 days), 6w (6 weeks), 7M (7 months), 8y (8 years)
	// If ttl is empty string, no TTL is applied and the file will be stored permanently
	UploadWithTTL(ctx context.Context, key string, reader io.Reader, contentType string, ttl string) (string, error)

	// Download retrieves content from the given key.
	// The caller must close the returned ReadCloser.
	Download(ctx context.Context, key string) (io.ReadCloser, error)

	// Delete removes an object by key.
	Delete(ctx context.Context, key string) error

	// Exists checks if an object exists at the given key.
	Exists(ctx context.Context, key string) (bool, error)

	// GeneratePresignedURL creates a time-limited download URL.
	// Note: Not all storage backends support presigned URLs (e.g., SeaweedFS HTTP mode)
	GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
}
