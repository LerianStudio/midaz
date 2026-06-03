// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package storage

import (
	"context"
	"fmt"
	"io"
)

// FetcherStorageAdapter wraps an ObjectStorage client (configured for the Fetcher bucket)
// to download extraction result files. The key received from Fetcher notifications is used
// as-is (e.g., "{tenantId}/external-data/{jobId}.json" or "external-data/{jobId}.json").
type FetcherStorageAdapter struct {
	storage ObjectStorage
}

// NewFetcherStorageAdapter creates an adapter that bridges ObjectStorage (io.ReadCloser)
// to the FetcherDataDownloader interface ([]byte) expected by the Worker UseCase.
func NewFetcherStorageAdapter(storage ObjectStorage) *FetcherStorageAdapter {
	return &FetcherStorageAdapter{storage: storage}
}

// DownloadFile downloads data from S3 using the key from the Fetcher notification result.path.
func (a *FetcherStorageAdapter) DownloadFile(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("empty download key")
	}

	reader, err := a.storage.Download(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("download from fetcher storage (key=%s): %w", key, err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read fetcher storage response (key=%s): %w", key, err)
	}

	return data, nil
}
