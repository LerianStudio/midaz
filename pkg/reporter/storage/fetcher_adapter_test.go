// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package storage

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestFetcherStorageAdapter_DownloadFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		key         string
		mockData    string
		mockErr     error
		wantData    []byte
		wantErr     bool
		errContains string
	}{
		{
			name:     "successful download with tenant prefix",
			key:      "tenant-123/external-data/job-456.json",
			mockData: `{"data": "test"}`,
			wantData: []byte(`{"data": "test"}`),
		},
		{
			name:     "successful download without tenant prefix",
			key:      "external-data/job-456.json",
			mockData: `{"data": "test"}`,
			wantData: []byte(`{"data": "test"}`),
		},
		{
			name:        "empty key returns error",
			key:         "",
			wantErr:     true,
			errContains: "empty download key",
		},
		{
			name:        "storage download error propagates",
			key:         "external-data/job-456.json",
			mockErr:     fmt.Errorf("connection refused"),
			wantErr:     true,
			errContains: "download from fetcher storage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockStorage := NewMockObjectStorage(ctrl)

			if tt.key != "" {
				if tt.mockErr != nil {
					mockStorage.EXPECT().
						Download(gomock.Any(), tt.key).
						Return(nil, tt.mockErr)
				} else {
					reader := io.NopCloser(strings.NewReader(tt.mockData))
					mockStorage.EXPECT().
						Download(gomock.Any(), tt.key).
						Return(reader, nil)
				}
			}

			adapter := NewFetcherStorageAdapter(mockStorage)
			data, err := adapter.DownloadFile(context.Background(), tt.key)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantData, data)
		})
	}
}
