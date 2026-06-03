// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package storage

import (
	"context"
	"fmt"
)

// Config contains configuration for creating a storage client
type Config struct {
	// Bucket name for the storage
	Bucket string

	// S3-compatible storage config (AWS S3, MinIO, SeaweedFS S3, etc)
	S3Endpoint        string
	S3Region          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3UsePathStyle    bool
	S3DisableSSL      bool
}

// NewStorageClient creates a storage client based on the provided configuration
func NewStorageClient(ctx context.Context, cfg Config) (ObjectStorage, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	s3Config := S3Config{
		Endpoint:        cfg.S3Endpoint,
		Region:          cfg.S3Region,
		Bucket:          cfg.Bucket,
		AccessKeyID:     cfg.S3AccessKeyID,
		SecretAccessKey: cfg.S3SecretAccessKey,
		UsePathStyle:    cfg.S3UsePathStyle,
		DisableSSL:      cfg.S3DisableSSL,
	}

	return NewS3Client(ctx, s3Config)
}
