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

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestReconciler_ReconcileMapping_FetcherCompleted(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "stale-job-001",
		ReportID:   reportID.String(),
		TemplateID: templateID.String(),
		TenantID:   "tenant-a",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	// Lock acquired
	mockRedis.EXPECT().
		SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), 2*r.interval).
		Return(true, nil)
	mockRedis.EXPECT().
		Del(gomock.Any(), reconcilerLockKey).
		Return(nil)

	// Stale mapping found
	mockExtRepo.EXPECT().
		FindStalePending(gomock.Any(), r.staleThreshold).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)

	// Fetcher says completed but has no result data path.
	// The reconciler should update the extraction mapping status to completed,
	// then stop early because there is no data to download (no result.path).
	// This tests the graceful degradation when the Fetcher completed a job
	// without producing downloadable data.
	mockFetcher.EXPECT().
		GetExtractionJobStatus(gomock.Any(), "stale-job-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "stale-job-001",
			Status: constant.FetcherStatusCompleted,
		}, nil)

	// Completion flow: update extraction mapping status
	mockExtRepo.EXPECT().
		UpdateStatus(gomock.Any(), "stale-job-001", constant.ExtractionStatusCompleted, gomock.Any()).
		Return(nil)

	// Note: UpdateReportStatusById is NOT expected because the reconciler
	// returns early when Result.Path is empty (no data to process).
	// The report remains in its current state for manual review.

	r.reconcile(context.Background())
}

func TestReconciler_ReconcileMapping_FetcherFailed(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, mockReportRepo := newTestReconciler(t, ctrl)

	reportID := uuid.New()
	templateID := uuid.New()

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "stale-job-002",
		ReportID:   reportID.String(),
		TemplateID: templateID.String(),
		TenantID:   "tenant-b",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	// Lock acquired
	mockRedis.EXPECT().
		SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), 2*r.interval).
		Return(true, nil)
	mockRedis.EXPECT().
		Del(gomock.Any(), reconcilerLockKey).
		Return(nil)

	// Stale mapping found
	mockExtRepo.EXPECT().
		FindStalePending(gomock.Any(), r.staleThreshold).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)

	// Fetcher says failed
	mockFetcher.EXPECT().
		GetExtractionJobStatus(gomock.Any(), "stale-job-002").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "stale-job-002",
			Status: constant.FetcherStatusFailed,
			Error:  "datasource timeout",
		}, nil)

	// Mark extraction mapping as failed
	mockExtRepo.EXPECT().
		UpdateStatus(gomock.Any(), "stale-job-002", constant.ExtractionStatusFailed, gomock.Any()).
		Return(nil)

	// Mark report as Error
	mockReportRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), constant.ErrorStatus, reportID, gomock.Any(), gomock.Any()).
		Return(nil)

	r.reconcile(context.Background())
}

func TestReconciler_ReconcileMapping_FetcherStillPending(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "stale-job-003",
		ReportID:   uuid.New().String(),
		TemplateID: uuid.New().String(),
		TenantID:   "tenant-c",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	// Lock acquired
	mockRedis.EXPECT().
		SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), 2*r.interval).
		Return(true, nil)
	mockRedis.EXPECT().
		Del(gomock.Any(), reconcilerLockKey).
		Return(nil)

	// Stale mapping found
	mockExtRepo.EXPECT().
		FindStalePending(gomock.Any(), r.staleThreshold).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)

	// Fetcher says still pending — skip
	mockFetcher.EXPECT().
		GetExtractionJobStatus(gomock.Any(), "stale-job-003").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "stale-job-003",
			Status: constant.ExtractionStatusPending,
		}, nil)

	// No UpdateStatus or report calls expected

	r.reconcile(context.Background())
}

func TestReconciler_ReconcileMapping_FetcherStatusError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, _ := newTestReconciler(t, ctrl)

	staleMapping := &datasource.ExtractionMapping{
		JobID:      "stale-job-004",
		ReportID:   uuid.New().String(),
		TemplateID: uuid.New().String(),
		TenantID:   "tenant-d",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}

	// Lock acquired
	mockRedis.EXPECT().
		SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), 2*r.interval).
		Return(true, nil)
	mockRedis.EXPECT().
		Del(gomock.Any(), reconcilerLockKey).
		Return(nil)

	// Stale mapping found
	mockExtRepo.EXPECT().
		FindStalePending(gomock.Any(), r.staleThreshold).
		Return([]*datasource.ExtractionMapping{staleMapping}, nil)

	// Fetcher call fails
	mockFetcher.EXPECT().
		GetExtractionJobStatus(gomock.Any(), "stale-job-004").
		Return(nil, errors.New("fetcher unavailable"))

	// No UpdateStatus calls expected — error is logged, mapping skipped

	r.reconcile(context.Background())
}

func TestReconciler_FindStalePendingError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, _, _ := newTestReconciler(t, ctrl)

	// Lock acquired
	mockRedis.EXPECT().
		SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), 2*r.interval).
		Return(true, nil)
	mockRedis.EXPECT().
		Del(gomock.Any(), reconcilerLockKey).
		Return(nil)

	// FindStalePending fails
	mockExtRepo.EXPECT().
		FindStalePending(gomock.Any(), r.staleThreshold).
		Return(nil, errors.New("mongo read failure"))

	// No further calls expected

	r.reconcile(context.Background())
}

func TestReconciler_MultipleStaleMappings(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, mockFetcher, mockReportRepo := newTestReconciler(t, ctrl)

	reportID1 := uuid.New()
	reportID2 := uuid.New()

	mapping1 := &datasource.ExtractionMapping{
		JobID:      "multi-job-001",
		ReportID:   reportID1.String(),
		TemplateID: uuid.New().String(),
		TenantID:   "tenant-x",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-20 * time.Minute),
	}
	mapping2 := &datasource.ExtractionMapping{
		JobID:      "multi-job-002",
		ReportID:   reportID2.String(),
		TemplateID: uuid.New().String(),
		TenantID:   "tenant-y",
		Status:     constant.ExtractionStatusPending,
		CreatedAt:  time.Now().Add(-30 * time.Minute),
	}

	// Lock acquired
	mockRedis.EXPECT().
		SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), 2*r.interval).
		Return(true, nil)
	mockRedis.EXPECT().
		Del(gomock.Any(), reconcilerLockKey).
		Return(nil)

	// Two stale mappings found
	mockExtRepo.EXPECT().
		FindStalePending(gomock.Any(), r.staleThreshold).
		Return([]*datasource.ExtractionMapping{mapping1, mapping2}, nil)

	// First mapping: Fetcher completed (no result data — returns early after status update)
	mockFetcher.EXPECT().
		GetExtractionJobStatus(gomock.Any(), "multi-job-001").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "multi-job-001",
			Status: constant.FetcherStatusCompleted,
		}, nil)
	mockExtRepo.EXPECT().
		UpdateStatus(gomock.Any(), "multi-job-001", constant.ExtractionStatusCompleted, gomock.Any()).
		Return(nil)

	// Second mapping: Fetcher failed
	mockFetcher.EXPECT().
		GetExtractionJobStatus(gomock.Any(), "multi-job-002").
		Return(&fetcher.ExtractionJobResponse{
			JobID:  "multi-job-002",
			Status: constant.FetcherStatusFailed,
			Error:  "connection lost",
		}, nil)
	mockExtRepo.EXPECT().
		UpdateStatus(gomock.Any(), "multi-job-002", constant.ExtractionStatusFailed, gomock.Any()).
		Return(nil)
	mockReportRepo.EXPECT().
		UpdateReportStatusById(gomock.Any(), constant.ErrorStatus, reportID2, gomock.Any(), gomock.Any()).
		Return(nil)

	r.reconcile(context.Background())
}
