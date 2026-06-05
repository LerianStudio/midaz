// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/fetcher"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

func TestReconciler_HandleReconcileCompleted_FullFlow(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, mockReportRepo := newTestReconciler(t, ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	staleMapping := &datasource.ExtractionMapping{
		JobID:        "completed-job-001",
		ReportID:     reportID.String(),
		TemplateID:   templateID.String(),
		OutputFormat: "html",
		Status:       constant.ExtractionStatusPending,
		CreatedAt:    time.Now().Add(-20 * time.Minute),
	}

	// Lock acquired
	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)

	// Stale mapping found
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)

	// Fetcher says completed WITH data path
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "completed-job-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "completed-job-001",
			Status: constant.FetcherStatusCompleted,
			Result: &fetcher.FetcherResultData{
				Path: "extractions/completed-job-001/data.json",
				HMAC: "some-hmac-value",
			},
		}, nil)

	// Update extraction mapping status
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "completed-job-001", constant.ExtractionStatusCompleted, gomock.Any()).Return(nil)

	// UseCase downloads extracted data
	r.useCase.FetcherDataStorage = &mockFetcherDataDownloader{
		downloadFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte(`{"db":{"table":[{"id":"1"}]}}`), nil
		},
	}

	// Report saved and marked as finished
	mockReportRepo.EXPECT().UpdateReportStatusById(gomock.Any(), constant.FinishedStatus, reportID, gomock.Any(), nil).Return(nil)

	r.reconcile(context.Background())
}

func TestReconciler_HandleReconcileCompleted_UpdateStatusError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "err-status-001",
		ReportID:   reportID.String(),
		TemplateID: templateID.String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "err-status-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "err-status-001",
			Status: constant.FetcherStatusCompleted,
			Result: &fetcher.FetcherResultData{Path: "data/test.json"},
		}, nil)

	// UpdateStatus fails — handleReconcileCompleted should return early
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "err-status-001", constant.ExtractionStatusCompleted, gomock.Any()).
		Return(errors.New("mongo write failure"))

	// No further calls expected

	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_HandleReconcileCompleted_InvalidReportID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "invalid-report-001",
		ReportID:   "not-a-uuid",
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "invalid-report-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "invalid-report-001",
			Status: constant.FetcherStatusCompleted,
			Result: &fetcher.FetcherResultData{Path: "data/test.json"},
		}, nil)
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "invalid-report-001", constant.ExtractionStatusCompleted, gomock.Any()).Return(nil)

	// Should return early due to invalid UUID
	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_HandleReconcileCompleted_InvalidTemplateID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "invalid-tpl-001",
		ReportID:   uuid.New().String(),
		TemplateID: "not-a-uuid",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "invalid-tpl-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "invalid-tpl-001",
			Status: constant.FetcherStatusCompleted,
			Result: &fetcher.FetcherResultData{Path: "data/test.json"},
		}, nil)
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "invalid-tpl-001", constant.ExtractionStatusCompleted, gomock.Any()).Return(nil)

	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_HandleReconcileCompleted_DownloadError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "download-err-001",
		ReportID:   reportID.String(),
		TemplateID: templateID.String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "download-err-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "download-err-001",
			Status: constant.FetcherStatusCompleted,
			Result: &fetcher.FetcherResultData{Path: "data/test.json"},
		}, nil)
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "download-err-001", constant.ExtractionStatusCompleted, gomock.Any()).Return(nil)

	r.useCase.FetcherDataStorage = &mockFetcherDataDownloader{
		downloadFunc: func(_ context.Context, _ string) ([]byte, error) {
			return nil, errors.New("storage unavailable")
		},
	}

	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_HandleReconcileFailed_FullFlow(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, mockReportRepo := newTestReconciler(t, ctrl)

	reportID := uuid.New()

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "failed-job-001",
		ReportID:   reportID.String(),
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "failed-job-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "failed-job-001",
			Status: constant.FetcherStatusFailed,
			Error:  "datasource connection timeout",
		}, nil)
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "failed-job-001", constant.ExtractionStatusFailed, gomock.Any()).Return(nil)
	mockReportRepo.EXPECT().UpdateReportStatusById(gomock.Any(), constant.ErrorStatus, reportID, gomock.Any(), gomock.Any()).Return(nil)

	r.reconcile(context.Background())
}

func TestReconciler_HandleReconcileFailed_UpdateStatusError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "fail-err-001",
		ReportID:   uuid.New().String(),
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "fail-err-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "fail-err-001",
			Status: constant.FetcherStatusFailed,
			Error:  "timeout",
		}, nil)
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "fail-err-001", constant.ExtractionStatusFailed, gomock.Any()).
		Return(errors.New("mongo failure"))

	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_HandleReconcileFailed_InvalidReportID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "fail-invalid-001",
		ReportID:   "not-a-uuid",
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "fail-invalid-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "fail-invalid-001",
			Status: constant.FetcherStatusFailed,
			Error:  "timeout",
		}, nil)
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "fail-invalid-001", constant.ExtractionStatusFailed, gomock.Any()).Return(nil)

	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_ReconcileMapping_NilResponse(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "nil-resp-001",
		ReportID:   uuid.New().String(),
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)

	// Fetcher returns nil response
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "nil-resp-001").
		Return(nil, nil)

	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_ReconcileMapping_Timeout(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, mockReportRepo := newTestReconciler(t, ctrl)

	reportID := uuid.New()

	// Mapping older than 60 minutes
	staleMapping := &datasource.ExtractionMapping{
		JobID:      "timeout-001",
		ReportID:   reportID.String(),
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-61 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)

	// Fetcher says still processing
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "timeout-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "timeout-001",
			Status: "processing",
		}, nil)

	// Should timeout and mark as failed
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "timeout-001", constant.ExtractionStatusFailed, gomock.Any()).Return(nil)
	mockReportRepo.EXPECT().UpdateReportStatusById(gomock.Any(), constant.ErrorStatus, reportID, gomock.Any(), gomock.Any()).Return(nil)

	r.reconcile(context.Background())
}

func TestReconciler_ReconcileMapping_Timeout_UpdateStatusError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "timeout-err-001",
		ReportID:   uuid.New().String(),
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-61 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "timeout-err-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "timeout-err-001",
			Status: "processing",
		}, nil)

	// UpdateStatus fails
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "timeout-err-001", constant.ExtractionStatusFailed, gomock.Any()).
		Return(errors.New("mongo write failure"))

	// No further calls expected
	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_ReconcileMapping_Timeout_InvalidReportID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "timeout-inv-001",
		ReportID:   "not-a-uuid",
		TemplateID: uuid.New().String(),
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-61 * time.Minute),
	}

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)
	mockFetcher.EXPECT().GetExtractionJobStatus(gomock.Any(), "timeout-inv-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "timeout-inv-001",
			Status: "processing",
		}, nil)
	mockExtRepo.EXPECT().UpdateStatus(gomock.Any(), "timeout-inv-001", constant.ExtractionStatusFailed, gomock.Any()).Return(nil)

	// Should return early due to invalid UUID
	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

// simpleTenantLister is a minimal TenantLister for non-build-tagged tests.
type simpleTenantLister struct{}

func (s *simpleTenantLister) GetActiveTenantsByService(_ context.Context, _ string) ([]*tmclient.TenantSummary, error) {
	return nil, nil
}

func TestWithMultiTenant_Option(t *testing.T) {
	t.Parallel()

	lister := &simpleTenantLister{}
	r := NewReconciler(
		&UseCase{},
		nil, nil, nil,
		nil,
		noop.NewTracerProvider().Tracer("test"),
		WithMultiTenant(lister, nil, "reporter"),
	)

	assert.True(t, r.multiTenantEnabled)
	assert.Equal(t, "reporter", r.serviceName)
	assert.Equal(t, lister, r.tenantLister)
}

func TestReconciler_ReconcileForContext_FindStaleProcessingError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, _, _ := newTestReconciler(t, ctrl)

	// Override the default FindStaleProcessing expectation
	mockExtRepo.EXPECT().FindStaleProcessing(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("mongo read failure")).AnyTimes()

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(nil)

	// FindStalePending returns empty
	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).Return(nil, nil)

	// Should continue even with FindStaleProcessing error
	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

func TestReconciler_LockReleaseFails_DoesNotPanic(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, _, _ := newTestReconciler(t, ctrl)

	mockRedis.EXPECT().SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), gomock.Any()).Return(true, nil)
	mockRedis.EXPECT().Del(gomock.Any(), reconcilerLockKey).Return(errors.New("redis unavailable"))

	mockExtRepo.EXPECT().FindStalePending(gomock.Any(), gomock.Any()).Return(nil, nil)

	require.NotPanics(t, func() {
		r.reconcile(context.Background())
	})
}

// noop import to silence the linter
var _ = assert.Equal
