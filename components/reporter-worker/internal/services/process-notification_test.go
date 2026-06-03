// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/fetcher"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUseCase_ProcessFetcherNotification_ParseMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        []byte
		wantErr     bool
		errContains string
	}{
		{
			name: "valid completed notification",
			body: mustMarshal(t, fetcher.FetcherNotification{
				JobID:  "job-001",
				Status: constant.FetcherStatusCompleted,
				Result: &fetcher.FetcherResultData{
					Path:      "extractions/job-001/result.json.enc",
					SizeBytes: 4096,
					RowCount:  150,
					Format:    "json",
					HMAC:      "abc123",
				},
			}),
			wantErr: false,
		},
		{
			name: "valid failed notification",
			body: mustMarshal(t, fetcher.FetcherNotification{
				JobID:  "job-002",
				Status: constant.FetcherStatusFailed,
				Metadata: map[string]any{
					"source": "reporter",
					"error":  map[string]any{"message": "datasource connection timeout"},
				},
			}),
			wantErr: false,
		},
		{
			name:        "invalid JSON body",
			body:        []byte(`{invalid`),
			wantErr:     true,
			errContains: "unmarshal",
		},
		{
			name:        "empty body",
			body:        []byte(``),
			wantErr:     true,
			errContains: "unmarshal",
		},
		{
			name: "missing jobId",
			body: mustMarshal(t, fetcher.FetcherNotification{
				Status: constant.FetcherStatusCompleted,
				Result: &fetcher.FetcherResultData{Path: "data/x.json"},
			}),
			wantErr:     true,
			errContains: "jobId is required",
		},
		{
			name: "missing status",
			body: mustMarshal(t, fetcher.FetcherNotification{
				JobID:  "job-003",
				Result: &fetcher.FetcherResultData{Path: "data/x.json"},
			}),
			wantErr:     true,
			errContains: "status is required",
		},
		{
			name: "invalid status value",
			body: mustMarshal(t, fetcher.FetcherNotification{
				JobID:  "job-004",
				Status: "unknown",
			}),
			wantErr:     true,
			errContains: "invalid notification status",
		},
		{
			name: "completed without result.path",
			body: mustMarshal(t, fetcher.FetcherNotification{
				JobID:  "job-005",
				Status: constant.FetcherStatusCompleted,
			}),
			wantErr:     true,
			errContains: "result.path is required",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			notification, err := parseNotificationMessage(tt.body)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)

				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, notification.JobID)
			assert.NotEmpty(t, notification.Status)
		})
	}
}

// mustMarshal is a test helper that marshals v to JSON or fails the test.
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()

	data, err := json.Marshal(v)
	require.NoError(t, err)

	return data
}

// mockTemplateSeaweedFS implements template.Repository for tests.
type mockTemplateSeaweedFS struct {
	getFunc func(ctx context.Context, id string) ([]byte, error)
}

func (m *mockTemplateSeaweedFS) Get(ctx context.Context, id string) ([]byte, error) {
	return m.getFunc(ctx, id)
}

func (m *mockTemplateSeaweedFS) Put(_ context.Context, _ string, _ string, _ []byte) error {
	return nil
}

// mockReportSeaweedFS implements report.Repository for tests.
type mockReportSeaweedFS struct {
	putFunc func(ctx context.Context, name, contentType string, data []byte, ttl string) error
}

func (m *mockReportSeaweedFS) Put(ctx context.Context, name, contentType string, data []byte, ttl string) error {
	return m.putFunc(ctx, name, contentType, data, ttl)
}

func (m *mockReportSeaweedFS) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

// mockFetcherDataDownloader implements FetcherDataDownloader for tests.
type mockFetcherDataDownloader struct {
	downloadFunc func(ctx context.Context, path string) ([]byte, error)
}

func (m *mockFetcherDataDownloader) DownloadFile(ctx context.Context, path string) ([]byte, error) {
	return m.downloadFunc(ctx, path)
}
