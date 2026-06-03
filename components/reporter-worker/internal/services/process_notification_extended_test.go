// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/datasource"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/fetcher"
	extractionRepo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/extraction"
	reportData "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/report"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func TestProcessFetcherNotification_NilMapping(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	mockExtractionRepo.EXPECT().
		FindByJobID(gomock.Any(), "job-nil-mapping").
		Return(nil, nil)

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                noop.NewTracerProvider().Tracer("test"),
		ExtractionMappingRepo: mockExtractionRepo,
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-nil-mapping",
		Status: constant.FetcherStatusCompleted,
		Result: &fetcher.FetcherResultData{Path: "data/test.json"},
	}
	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.Error(t, err)

	var entityNotFound pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFound))
}

func TestProcessFetcherNotification_AtomicClaimError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	mapping := &datasource.ExtractionMapping{
		JobID:      "job-claim-err",
		ReportID:   uuid.New().String(),
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-time.Minute),
	}

	mockExtractionRepo.EXPECT().FindByJobID(gomock.Any(), "job-claim-err").Return(mapping, nil)
	mockExtractionRepo.EXPECT().AtomicClaimPending(gomock.Any(), "job-claim-err").
		Return(false, errors.New("mongo CAS failure"))

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                noop.NewTracerProvider().Tracer("test"),
		ExtractionMappingRepo: mockExtractionRepo,
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-claim-err",
		Status: constant.FetcherStatusCompleted,
		Result: &fetcher.FetcherResultData{Path: "data/test.json"},
	}
	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "atomic claim")
}

func TestProcessFetcherNotification_NoDocumentsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	mockExtractionRepo.EXPECT().FindByJobID(gomock.Any(), "job-no-docs").
		Return(nil, errors.New("mongo: no documents in result"))

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                noop.NewTracerProvider().Tracer("test"),
		ExtractionMappingRepo: mockExtractionRepo,
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-no-docs",
		Status: constant.FetcherStatusCompleted,
		Result: &fetcher.FetcherResultData{Path: "data/test.json"},
	}
	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.Error(t, err)

	var entityNotFound pkg.EntityNotFoundError
	assert.True(t, errors.As(err, &entityNotFound))
}

func TestHandleCompletedNotification_TenantMismatch(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)
	mockReportDataRepo := reportData.NewMockRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	mapping := &datasource.ExtractionMapping{
		JobID:      "job-mismatch",
		ReportID:   reportID.String(),
		TemplateID: templateID.String(),
		TenantID:   "tenant-a",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-time.Minute),
	}

	mockExtractionRepo.EXPECT().FindByJobID(gomock.Any(), "job-mismatch").Return(mapping, nil)
	mockExtractionRepo.EXPECT().AtomicClaimPending(gomock.Any(), "job-mismatch").Return(true, nil)
	mockExtractionRepo.EXPECT().UpdateStatus(gomock.Any(), "job-mismatch", constant.ExtractionStatusCompleted, gomock.Any()).Return(nil)

	// Report status updated to Error due to tenant mismatch
	mockReportDataRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), constant.ErrorStatus, reportID, gomock.Any(), gomock.Any()).
		Return(nil)

	mockFetcherStorage := &mockFetcherDataDownloader{
		downloadFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte(`{}`), nil
		},
	}

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                noop.NewTracerProvider().Tracer("test"),
		ExtractionMappingRepo: mockExtractionRepo,
		ReportDataRepo:        mockReportDataRepo,
		FetcherDataStorage:    mockFetcherStorage,
		TemplateSeaweedFS: &mockTemplateSeaweedFS{
			getFunc: func(_ context.Context, _ string) ([]byte, error) {
				return []byte("template"), nil
			},
		},
	}

	// Notification with data path that doesn't match tenant
	notification := fetcher.FetcherNotification{
		JobID:  "job-mismatch",
		Status: constant.FetcherStatusCompleted,
		Result: &fetcher.FetcherResultData{Path: "tenant-b/data/job-mismatch.json"},
	}
	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant mismatch")
}

func TestHandleCompletedNotification_InvalidReportID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	mapping := &datasource.ExtractionMapping{
		JobID:      "job-bad-report",
		ReportID:   "not-a-uuid",
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-time.Minute),
	}

	mockExtractionRepo.EXPECT().FindByJobID(gomock.Any(), "job-bad-report").Return(mapping, nil)
	mockExtractionRepo.EXPECT().AtomicClaimPending(gomock.Any(), "job-bad-report").Return(true, nil)
	mockExtractionRepo.EXPECT().UpdateStatus(gomock.Any(), "job-bad-report", constant.ExtractionStatusCompleted, gomock.Any()).Return(nil)

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                noop.NewTracerProvider().Tracer("test"),
		ExtractionMappingRepo: mockExtractionRepo,
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-bad-report",
		Status: constant.FetcherStatusCompleted,
		Result: &fetcher.FetcherResultData{Path: "data/test.json"},
	}
	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid report ID")
}

func TestHandleCompletedNotification_InvalidTemplateID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	mapping := &datasource.ExtractionMapping{
		JobID:      "job-bad-tpl",
		ReportID:   uuid.New().String(),
		TemplateID: "not-a-uuid",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-time.Minute),
	}

	mockExtractionRepo.EXPECT().FindByJobID(gomock.Any(), "job-bad-tpl").Return(mapping, nil)
	mockExtractionRepo.EXPECT().AtomicClaimPending(gomock.Any(), "job-bad-tpl").Return(true, nil)
	mockExtractionRepo.EXPECT().UpdateStatus(gomock.Any(), "job-bad-tpl", constant.ExtractionStatusCompleted, gomock.Any()).Return(nil)

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                noop.NewTracerProvider().Tracer("test"),
		ExtractionMappingRepo: mockExtractionRepo,
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-bad-tpl",
		Status: constant.FetcherStatusCompleted,
		Result: &fetcher.FetcherResultData{Path: "data/test.json"},
	}
	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid template ID")
}

func TestHandleCompletedNotification_DownloadError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)
	mockReportDataRepo := reportData.NewMockRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	mapping := &datasource.ExtractionMapping{
		JobID:      "job-dl-err",
		ReportID:   reportID.String(),
		TemplateID: templateID.String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-time.Minute),
	}

	mockExtractionRepo.EXPECT().FindByJobID(gomock.Any(), "job-dl-err").Return(mapping, nil)
	mockExtractionRepo.EXPECT().AtomicClaimPending(gomock.Any(), "job-dl-err").Return(true, nil)
	mockExtractionRepo.EXPECT().UpdateStatus(gomock.Any(), "job-dl-err", constant.ExtractionStatusCompleted, gomock.Any()).Return(nil)
	mockReportDataRepo.EXPECT().UpdateReportStatusById(gomock.Any(), constant.ErrorStatus, reportID, gomock.Any(), gomock.Any()).Return(nil)

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                noop.NewTracerProvider().Tracer("test"),
		ExtractionMappingRepo: mockExtractionRepo,
		ReportDataRepo:        mockReportDataRepo,
		TemplateSeaweedFS: &mockTemplateSeaweedFS{
			getFunc: func(_ context.Context, _ string) ([]byte, error) {
				return []byte("template"), nil
			},
		},
		FetcherDataStorage: &mockFetcherDataDownloader{
			downloadFunc: func(_ context.Context, _ string) ([]byte, error) {
				return nil, errors.New("S3 unavailable")
			},
		},
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-dl-err",
		Status: constant.FetcherStatusCompleted,
		Result: &fetcher.FetcherResultData{Path: "data/test.json"},
	}
	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.Error(t, err)
}

func TestHandleCompletedNotification_DefaultOutputFormat(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)
	mockReportDataRepo := reportData.NewMockRepository(ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	// Mapping with empty OutputFormat — should default to "html"
	mapping := &datasource.ExtractionMapping{
		JobID:        "job-default-fmt",
		ReportID:     reportID.String(),
		TemplateID:   templateID.String(),
		OutputFormat: "",
		Status:       constant.ExtractionStatusPending,
		CreatedAt:    time.Now().Add(-time.Minute),
	}

	mockExtractionRepo.EXPECT().FindByJobID(gomock.Any(), "job-default-fmt").Return(mapping, nil)
	mockExtractionRepo.EXPECT().AtomicClaimPending(gomock.Any(), "job-default-fmt").Return(true, nil)
	mockExtractionRepo.EXPECT().UpdateStatus(gomock.Any(), "job-default-fmt", constant.ExtractionStatusCompleted, gomock.Any()).Return(nil)
	mockReportDataRepo.EXPECT().UpdateReportStatusById(gomock.Any(), constant.FinishedStatus, reportID, gomock.Any(), nil).Return(nil)

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                noop.NewTracerProvider().Tracer("test"),
		ExtractionMappingRepo: mockExtractionRepo,
		ReportDataRepo:        mockReportDataRepo,
		TemplateSeaweedFS: &mockTemplateSeaweedFS{
			getFunc: func(_ context.Context, _ string) ([]byte, error) {
				return []byte("Report: {{ data }}"), nil
			},
		},
		ReportSeaweedFS: &mockReportSeaweedFS{
			putFunc: func(_ context.Context, _ string, _ string, _ []byte, _ string) error {
				return nil
			},
		},
		FetcherDataStorage: &mockFetcherDataDownloader{
			downloadFunc: func(_ context.Context, _ string) ([]byte, error) {
				return []byte(`{"db":{"table":[{"id":"1"}]}}`), nil
			},
		},
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-default-fmt",
		Status: constant.FetcherStatusCompleted,
		Result: &fetcher.FetcherResultData{Path: "data/test.json"},
	}
	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.NoError(t, err)
}

func TestHandleFailedNotification_InvalidReportID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockExtractionRepo := extractionRepo.NewMockRepository(ctrl)

	mapping := &datasource.ExtractionMapping{
		JobID:      "job-fail-bad-id",
		ReportID:   "not-a-uuid",
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-time.Minute),
	}

	mockExtractionRepo.EXPECT().FindByJobID(gomock.Any(), "job-fail-bad-id").Return(mapping, nil)
	mockExtractionRepo.EXPECT().AtomicClaimPending(gomock.Any(), "job-fail-bad-id").Return(true, nil)
	mockExtractionRepo.EXPECT().UpdateStatus(gomock.Any(), "job-fail-bad-id", constant.ExtractionStatusFailed, gomock.Any()).Return(nil)

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                noop.NewTracerProvider().Tracer("test"),
		ExtractionMappingRepo: mockExtractionRepo,
	}

	notification := fetcher.FetcherNotification{
		JobID:  "job-fail-bad-id",
		Status: constant.FetcherStatusFailed,
		Metadata: map[string]any{
			"error": map[string]any{"message": "timeout"},
		},
	}
	body := mustMarshal(t, notification)

	err := uc.ProcessFetcherNotification(context.Background(), body)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid report ID")
}

func TestReportErrorMetadata_CanceledContext(t *testing.T) {
	t.Parallel()

	metadata := reportErrorMetadata(context.Canceled)
	assert.Equal(t, "Report generation was canceled", metadata["error"])
	assert.Equal(t, "report_generation_canceled", metadata["error_code"])
}

func TestReportErrorMetadata_NilError(t *testing.T) {
	t.Parallel()

	metadata := reportErrorMetadata(nil)
	assert.Equal(t, "Report generation failed", metadata["error"])
	assert.Equal(t, "report_generation_failed", metadata["error_code"])
	_, hasDetail := metadata["error_detail"]
	assert.False(t, hasDetail)
}
