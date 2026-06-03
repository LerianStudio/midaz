// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/datasource"
	"github.com/LerianStudio/midaz/v3/pkg/reporter/fetcher"
	extractionRepo "github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb/extraction"
	reportData "github.com/LerianStudio/midaz/v3/pkg/reporter/mongodb/report"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func TestUseCase_ProcessFetcherNotification_CompletedStatus(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)
	mockReportDataRepo := reportData.NewMockRepository(ctrl)
	reportID := uuid.New()
	templateID := uuid.New()

	mapping := &datasource.ExtractionMapping{
		JobID:      "job-100",
		ReportID:   reportID.String(),
		TemplateID: templateID.String(),
		TenantID:   "tenant-abc",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-time.Minute),
	}

	mockExtractionRepo.EXPECT().
		FindByJobID(gomock.Any(), "job-100").
		Return(mapping, nil)

	// Gap 14: AtomicClaimPending returns true (this worker claims the job)
	mockExtractionRepo.EXPECT().
		AtomicClaimPending(gomock.Any(), "job-100").
		Return(true, nil)

	mockExtractionRepo.EXPECT().
		UpdateStatus(gomock.Any(), "job-100", constant.ExtractionStatusCompleted, gomock.Any()).
		Return(nil)

	// Mock SeaweedFS download of extracted data (valid JSON result map)
	extractedJSON := `{"db1":{"public__users":[{"id":"1","name":"Alice"}]}}`
	mockFetcherStorage := &mockFetcherDataDownloader{
		downloadFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte(extractedJSON), nil
		},
	}

	// Report status should be updated to Finished after successful resume
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), constant.FinishedStatus, reportID, gomock.Any(), nil).
		Return(nil)

	tracer := noop.NewTracerProvider().Tracer("test")

	mockTemplateRepo := &mockTemplateSeaweedFS{
		getFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte("Report: {{ data }}"), nil
		},
	}
	mockReportRepo := &mockReportSeaweedFS{
		putFunc: func(_ context.Context, _ string, _ string, _ []byte, _ string) error {
			return nil
		},
	}

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                tracer,
		ExtractionMappingRepo: mockExtractionRepo,
		ReportDataRepo:        mockReportDataRepo,
		TemplateSeaweedFS:     mockTemplateRepo,
		ReportSeaweedFS:       mockReportRepo,
		FetcherDataStorage:    mockFetcherStorage,
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-100",
		Status: constant.FetcherStatusCompleted,
		Result: &fetcher.FetcherResultData{
			Path: "tenant-abc/data/job-100.json",
		},
	}

	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.NoError(t, err)
}

func TestUseCase_ProcessFetcherNotification_FailedStatus(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)
	mockReportDataRepo := reportData.NewMockRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	mapping := &datasource.ExtractionMapping{
		JobID:      "job-200",
		ReportID:   reportID.String(),
		TemplateID: templateID.String(),
		TenantID:   "tenant-xyz",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-time.Minute),
	}

	mockExtractionRepo.EXPECT().
		FindByJobID(gomock.Any(), "job-200").
		Return(mapping, nil)

	// Gap 14: AtomicClaimPending returns true (this worker claims the job)
	mockExtractionRepo.EXPECT().
		AtomicClaimPending(gomock.Any(), "job-200").
		Return(true, nil)

	mockExtractionRepo.EXPECT().
		UpdateStatus(gomock.Any(), "job-200", constant.ExtractionStatusFailed, gomock.Any()).
		Return(nil)

	// Report status should be updated to Error
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), constant.ErrorStatus, reportID, gomock.Any(), gomock.Any()).
		Return(nil)

	tracer := noop.NewTracerProvider().Tracer("test")

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                tracer,
		ExtractionMappingRepo: mockExtractionRepo,
		ReportDataRepo:        mockReportDataRepo,
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-200",
		Status: constant.FetcherStatusFailed,
		Metadata: map[string]any{
			"source": "reporter",
			"error":  map[string]any{"message": "datasource connection refused"},
		},
	}

	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.NoError(t, err)
}

func TestUseCase_ProcessFetcherNotification_Idempotency(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{
			name: "already claimed mapping is skipped",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

			// FindByJobID still returns the mapping (for span attributes etc.)
			mapping := &datasource.ExtractionMapping{
				JobID:      "job-300",
				ReportID:   uuid.New().String(),
				TemplateID: uuid.New().String(),
				Status:     constant.ExtractionStatusCompleted,
				CreatedAt:  time.Now().Add(-time.Minute),
			}

			mockExtractionRepo.EXPECT().
				FindByJobID(gomock.Any(), "job-300").
				Return(mapping, nil)

			// Gap 14: AtomicClaimPending returns false — another worker already claimed it
			mockExtractionRepo.EXPECT().
				AtomicClaimPending(gomock.Any(), "job-300").
				Return(false, nil)

			// No UpdateStatus or report status calls expected — idempotent skip
			tracer := noop.NewTracerProvider().Tracer("test")

			uc := &UseCase{
				Logger:                log.NewNop(),
				Tracer:                tracer,
				ExtractionMappingRepo: mockExtractionRepo,
			}

			notification := fetcher.FetcherNotification{
				JobID:  "job-300",
				Status: constant.FetcherStatusCompleted,
				Result: &fetcher.FetcherResultData{Path: "data/job-300.json"},
			}

			body := mustMarshal(t, notification)

			err := uc.ProcessFetcherNotification(context.Background(), body)
			require.NoError(t, err)
		})
	}
}

func TestUseCase_ProcessFetcherNotification_MappingNotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	mockExtractionRepo.EXPECT().
		FindByJobID(gomock.Any(), "job-404").
		Return(nil, errors.New("no extraction mapping found"))

	tracer := noop.NewTracerProvider().Tracer("test")

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                tracer,
		ExtractionMappingRepo: mockExtractionRepo,
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-404",
		Status: constant.FetcherStatusCompleted,
		Result: &fetcher.FetcherResultData{Path: "data/job-404.json"},
	}

	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lookup extraction mapping")
}

func TestUseCase_ProcessFetcherNotification_UpdateStatusError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	mapping := &datasource.ExtractionMapping{
		JobID:      "job-500",
		ReportID:   uuid.New().String(),
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-time.Minute),
	}

	mockExtractionRepo.EXPECT().
		FindByJobID(gomock.Any(), "job-500").
		Return(mapping, nil)

	// Gap 14: AtomicClaimPending returns true
	mockExtractionRepo.EXPECT().
		AtomicClaimPending(gomock.Any(), "job-500").
		Return(true, nil)

	mockExtractionRepo.EXPECT().
		UpdateStatus(gomock.Any(), "job-500", constant.ExtractionStatusFailed, gomock.Any()).
		Return(errors.New("mongo write failure"))

	tracer := noop.NewTracerProvider().Tracer("test")

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                tracer,
		ExtractionMappingRepo: mockExtractionRepo,
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-500",
		Status: constant.FetcherStatusFailed,
		Metadata: map[string]any{
			"error": map[string]any{"message": "some error"},
		},
	}

	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update extraction mapping status")
}
