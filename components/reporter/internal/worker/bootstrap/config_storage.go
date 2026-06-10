// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/storage"
)

// initStorageClient creates the S3-compatible storage client from configuration.
func initStorageClient(ctx context.Context, cfg *Config) (storage.ObjectStorage, error) {
	storageConfig := storage.Config{
		Bucket:            cfg.ObjectStorageBucket,
		S3Endpoint:        cfg.ObjectStorageEndpoint,
		S3Region:          cfg.ObjectStorageRegion,
		S3AccessKeyID:     cfg.ObjectStorageAccessKeyID,
		S3SecretAccessKey: cfg.ObjectStorageSecretKey,
		S3UsePathStyle:    cfg.ObjectStorageUsePathStyle,
		S3DisableSSL:      cfg.ObjectStorageDisableSSL,
	}

	storageClient, err := storage.NewStorageClient(ctx, storageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage client: %w", err)
	}

	return storageClient, nil
}
