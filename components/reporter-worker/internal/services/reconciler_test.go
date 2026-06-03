// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	extractionRepo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/extraction"
	reportData "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/report"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/redis"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

// newTestReconciler creates a Reconciler with all mocks wired up for testing.
func newTestReconciler(
	t *testing.T,
	ctrl *gomock.Controller,
) (*Reconciler, *extractionRepo.MockRepository, *redis.MockRedisRepository, *MockExtractionJobStatusChecker, *reportData.MockRepository) {
	t.Helper()

	mockExtRepo := extractionRepo.NewMockRepository(ctrl)
	mockRedis := redis.NewMockRedisRepository(ctrl)
	mockFetcher := NewMockExtractionJobStatusChecker(ctrl)
	mockReportRepo := reportData.NewMockRepository(ctrl)
	tracer := noop.NewTracerProvider().Tracer("test")

	// Default: FindStaleProcessing returns empty (tests focus on pending path unless overridden)
	mockExtRepo.EXPECT().FindStaleProcessing(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	mockTemplateRepo := &mockTemplateSeaweedFS{
		getFunc: func(_ context.Context, _ string) ([]byte, error) {
			return []byte("Report: {{ data }}"), nil
		},
	}
	mockReportFileRepo := &mockReportSeaweedFS{
		putFunc: func(_ context.Context, _ string, _ string, _ []byte, _ string) error {
			return nil
		},
	}

	uc := &UseCase{
		Logger:                log.NewNop(),
		Tracer:                tracer,
		ExtractionMappingRepo: mockExtRepo,
		ReportDataRepo:        mockReportRepo,
		TemplateSeaweedFS:     mockTemplateRepo,
		ReportSeaweedFS:       mockReportFileRepo,
	}

	r := NewReconciler(
		uc,
		mockExtRepo,
		mockFetcher,
		mockRedis,
		log.NewNop(),
		tracer,
		WithInterval(100*time.Millisecond),
		WithStaleThreshold(15*time.Minute),
	)

	return r, mockExtRepo, mockRedis, mockFetcher, mockReportRepo
}

func TestNewReconciler_DefaultOptions(t *testing.T) {
	t.Parallel()

	tracer := noop.NewTracerProvider().Tracer("test")

	r := NewReconciler(
		&UseCase{},
		nil,
		nil,
		nil,
		log.NewNop(),
		tracer,
	)

	assert.Equal(t, defaultReconcileInterval, r.interval)
	assert.Equal(t, defaultStaleThreshold, r.staleThreshold)
}

func TestNewReconciler_CustomOptions(t *testing.T) {
	t.Parallel()

	tracer := noop.NewTracerProvider().Tracer("test")
	customInterval := 10 * time.Second
	customThreshold := 30 * time.Minute

	r := NewReconciler(
		&UseCase{},
		nil,
		nil,
		nil,
		log.NewNop(),
		tracer,
		WithInterval(customInterval),
		WithStaleThreshold(customThreshold),
	)

	assert.Equal(t, customInterval, r.interval)
	assert.Equal(t, customThreshold, r.staleThreshold)
}

func TestReconciler_ReconcileSingleIteration_AcquiresLock(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, mockExtRepo, mockRedis, _, _ := newTestReconciler(t, ctrl)

	// Lock acquired successfully
	mockRedis.EXPECT().
		SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), 2*r.interval).
		Return(true, nil)
	mockRedis.EXPECT().
		Del(gomock.Any(), reconcilerLockKey).
		Return(nil)

	// No stale mappings found
	mockExtRepo.EXPECT().
		FindStalePending(gomock.Any(), r.staleThreshold).
		Return(nil, nil)

	r.reconcile(context.Background())
}

func TestReconciler_ReconcileSingleIteration_LockNotAcquired(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, _, mockRedis, _, _ := newTestReconciler(t, ctrl)

	// Lock NOT acquired — another instance holds it
	mockRedis.EXPECT().
		SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), 2*r.interval).
		Return(false, nil)

	// No FindStalePending or any other calls expected
	r.reconcile(context.Background())
}

func TestReconciler_ReconcileSingleIteration_LockError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, _, mockRedis, _, _ := newTestReconciler(t, ctrl)

	// Lock acquire fails
	mockRedis.EXPECT().
		SetNX(gomock.Any(), reconcilerLockKey, gomock.Any(), 2*r.interval).
		Return(false, errors.New("redis connection refused"))

	// No further calls expected
	r.reconcile(context.Background())
}

func TestReconciler_GracefulShutdown(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	r, _, _, _, _ := newTestReconciler(t, ctrl)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()

	// Cancel immediately — the goroutine should exit gracefully
	cancel()

	select {
	case <-done:
		// Success — Start returned after cancellation
	case <-time.After(2 * time.Second):
		t.Fatal("Reconciler.Start did not exit within 2 seconds after context cancellation")
	}
}
