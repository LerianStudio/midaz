// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/services/workers/mocks"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
)

// fixedReaperTime is the deterministic "now" used across reaper tests. Per the
// tracer test rules, tests never call time.Now() — the clock is injected.
func fixedReaperTime() time.Time {
	return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
}

func TestNewReservationReaperWorker(t *testing.T) {
	tests := []struct {
		name        string
		config      ReservationReaperWorkerConfig
		nilRepo     bool
		nilAuditor  bool
		nilLogger   bool
		expectError error
	}{
		{
			name:        "creates worker with valid config",
			config:      ReservationReaperWorkerConfig{ReapInterval: 30 * time.Second},
			expectError: nil,
		},
		{
			name:        "returns error when repository is nil",
			config:      ReservationReaperWorkerConfig{ReapInterval: 30 * time.Second},
			nilRepo:     true,
			expectError: ErrNilRepository,
		},
		{
			name:        "returns error when auditor is nil",
			config:      ReservationReaperWorkerConfig{ReapInterval: 30 * time.Second},
			nilAuditor:  true,
			expectError: ErrNilReservationAuditor,
		},
		{
			name:        "returns error when logger is nil",
			config:      ReservationReaperWorkerConfig{ReapInterval: 30 * time.Second},
			nilLogger:   true,
			expectError: ErrNilLogger,
		},
		{
			name:        "returns error when interval is zero",
			config:      ReservationReaperWorkerConfig{ReapInterval: 0},
			expectError: ErrInvalidReaperInterval,
		},
		{
			name:        "returns error when interval is negative",
			config:      ReservationReaperWorkerConfig{ReapInterval: -1 * time.Second},
			expectError: ErrInvalidReaperInterval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			_, cleanup := setupTestTracer(t)
			defer cleanup()

			var repo ReservationReaperRepository
			if !tt.nilRepo {
				repo = mocks.NewMockReservationReaperRepository(ctrl)
			}

			var auditor ReservationExpiryAuditor
			if !tt.nilAuditor {
				auditor = mocks.NewMockReservationExpiryAuditor(ctrl)
			}

			var logger libLog.Logger = testutil.NewMockLogger()
			if tt.nilLogger {
				logger = nil
			}

			worker, err := NewReservationReaperWorker(repo, auditor, tt.config, logger, nil, "")

			if tt.expectError != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.expectError)
				assert.Nil(t, worker)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, worker)
			}
		})
	}
}

func TestReservationReaperWorker_DefaultConfig(t *testing.T) {
	config := DefaultReservationReaperWorkerConfig()

	assert.Equal(t, 30*time.Second, config.ReapInterval)
	assert.Equal(t, DefaultReservationReaperInterval, config.ReapInterval)
}

// TestReservationReaperWorker_RunOnce_ReleasesExpired asserts that every expired
// reservation returned by the repo is released as EXPIRED and exactly ONE
// batch-summary audit row is written for the sweep.
func TestReservationReaperWorker_RunOnce_ReleasesExpired(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockAuditor := mocks.NewMockReservationExpiryAuditor(ctrl)
	logger := testutil.NewMockLogger()

	now := fixedReaperTime()
	testClock := mockClock{fixedTime: now}

	expired := []uuid.UUID{
		testutil.MustDeterministicUUID(1),
		testutil.MustDeterministicUUID(2),
		testutil.MustDeterministicUUID(3),
	}

	// The sweep reads with the injected clock's "now".
	mockRepo.EXPECT().
		FindExpiredReservations(gomock.Any(), now.UTC()).
		Return(expired, nil).
		Times(1)

	// Each expired reservation is released as EXPIRED exactly once.
	for _, id := range expired {
		mockRepo.EXPECT().
			ReleaseExpired(gomock.Any(), id).
			Return(nil).
			Times(1)
	}

	// Exactly ONE batch audit row for the whole sweep, carrying the count.
	mockAuditor.EXPECT().
		RecordReservationExpiryBatch(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, summary command.ReservationExpiryBatchSummary) error {
			assert.Equal(t, len(expired), summary.ExpiredCount)
			assert.Equal(t, now.UTC(), summary.SweptAt)
			return nil
		}).
		Times(1)

	worker, err := NewReservationReaperWorker(mockRepo, mockAuditor, DefaultReservationReaperWorkerConfig(), logger, testClock, "")
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())

	require.NoError(t, err)
	assert.Equal(t, len(expired), released)
}

// TestReservationReaperWorker_RunOnce_FreshUntouched asserts that when nothing is
// expired, the reaper releases nothing AND writes no batch audit row (empty sweep
// produces no audit noise).
func TestReservationReaperWorker_RunOnce_FreshUntouched(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockAuditor := mocks.NewMockReservationExpiryAuditor(ctrl)
	logger := testutil.NewMockLogger()

	now := fixedReaperTime()
	testClock := mockClock{fixedTime: now}

	mockRepo.EXPECT().
		FindExpiredReservations(gomock.Any(), now.UTC()).
		Return(nil, nil).
		Times(1)

	// No ReleaseExpired and no RecordReservationExpiryBatch expected — gomock
	// fails the test if either is called.

	worker, err := NewReservationReaperWorker(mockRepo, mockAuditor, DefaultReservationReaperWorkerConfig(), logger, testClock, "")
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 0, released)
}

// TestReservationReaperWorker_RunOnce_FindError asserts a find failure is returned
// and no release / audit is attempted.
func TestReservationReaperWorker_RunOnce_FindError(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockAuditor := mocks.NewMockReservationExpiryAuditor(ctrl)
	logger := testutil.NewMockLogger()

	now := fixedReaperTime()
	testClock := mockClock{fixedTime: now}

	mockRepo.EXPECT().
		FindExpiredReservations(gomock.Any(), now.UTC()).
		Return(nil, errors.New("db down")).
		Times(1)

	worker, err := NewReservationReaperWorker(mockRepo, mockAuditor, DefaultReservationReaperWorkerConfig(), logger, testClock, "")
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())

	require.Error(t, err)
	assert.Equal(t, 0, released)
	assert.Contains(t, err.Error(), "failed to find expired reservations")
}

// TestReservationReaperWorker_RunOnce_ReleaseErrorStopsSweep asserts a genuine
// release failure aborts the remaining releases, returns the partial count, and
// does NOT write a batch audit row for the aborted sweep.
func TestReservationReaperWorker_RunOnce_ReleaseErrorStopsSweep(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockAuditor := mocks.NewMockReservationExpiryAuditor(ctrl)
	logger := testutil.NewMockLogger()

	now := fixedReaperTime()
	testClock := mockClock{fixedTime: now}

	first := testutil.MustDeterministicUUID(1)
	second := testutil.MustDeterministicUUID(2)

	mockRepo.EXPECT().
		FindExpiredReservations(gomock.Any(), now.UTC()).
		Return([]uuid.UUID{first, second}, nil).
		Times(1)

	mockRepo.EXPECT().
		ReleaseExpired(gomock.Any(), first).
		Return(errors.New("release failed")).
		Times(1)

	// second is never reached; no batch audit is written.

	worker, err := NewReservationReaperWorker(mockRepo, mockAuditor, DefaultReservationReaperWorkerConfig(), logger, testClock, "")
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())

	require.Error(t, err)
	assert.Equal(t, 0, released)
	assert.Contains(t, err.Error(), "failed to release expired reservation")
}

// TestReservationReaperWorker_Cadence asserts the worker honors the ticker: each
// tick drives a sweep. A controllable ticker channel lets the test pump exactly
// the number of cycles it wants without sleeping on wall-clock.
func TestReservationReaperWorker_Cadence(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockAuditor := mocks.NewMockReservationExpiryAuditor(ctrl)
	logger := testutil.NewMockLogger()

	now := fixedReaperTime()
	tickerChan := make(chan time.Time)
	testClock := mockClock{fixedTime: now, tickerChan: tickerChan}

	// Count sweeps via the find call (one per cycle). The initial cycle on start
	// plus N ticks => N+1 sweeps. Each sweep finds nothing, so no release/audit.
	sweeps := make(chan struct{}, 8)

	mockRepo.EXPECT().
		FindExpiredReservations(gomock.Any(), now.UTC()).
		DoAndReturn(func(_ context.Context, _ time.Time) ([]uuid.UUID, error) {
			sweeps <- struct{}{}
			return nil, nil
		}).
		MinTimes(3)

	worker, err := NewReservationReaperWorker(mockRepo, mockAuditor, ReservationReaperWorkerConfig{ReapInterval: time.Hour}, logger, testClock, "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()
		_ = worker.RunWithContext(ctx)
	}()

	// Initial cycle on start.
	waitForSweep(t, sweeps)

	// Two driven ticks => two more sweeps.
	tickerChan <- now
	waitForSweep(t, sweeps)

	tickerChan <- now
	waitForSweep(t, sweeps)

	cancel()
	wg.Wait()
}

func waitForSweep(t *testing.T, sweeps <-chan struct{}) {
	t.Helper()

	select {
	case <-sweeps:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a reap sweep")
	}
}

// TestReservationReaperWorker_RunWithContext_StopsBeforeInitialCycle asserts a
// worker whose context is already cancelled does no work.
func TestReservationReaperWorker_RunWithContext_StopsBeforeInitialCycle(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockAuditor := mocks.NewMockReservationExpiryAuditor(ctrl)
	logger := testutil.NewMockLogger()

	// No repo/auditor calls expected: gomock fails if FindExpiredReservations runs.

	worker, err := NewReservationReaperWorker(mockRepo, mockAuditor, DefaultReservationReaperWorkerConfig(), logger, mockClock{fixedTime: fixedReaperTime()}, "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, worker.RunWithContext(ctx))
}

// stubFailingPoolResolver always fails to resolve a tenant pool.
type stubFailingPoolResolver struct{}

func (stubFailingPoolResolver) GetTenantDB(_ context.Context, _ string) (dbresolver.DB, error) {
	return nil, errors.New("tenant pool unavailable")
}

// TestReservationReaperWorker_SkipsCycleOnPoolResolveFailure asserts that in MT
// mode, when the tenant pool cannot be resolved, the cycle is skipped and the
// repo is NEVER touched — the reaper never falls back to the root pool.
func TestReservationReaperWorker_SkipsCycleOnPoolResolveFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockAuditor := mocks.NewMockReservationExpiryAuditor(ctrl)
	logger := testutil.NewMockLogger()

	// No repo/auditor calls expected when the pool fails to resolve.

	worker, err := NewReservationReaperWorkerWithPoolResolver(
		mockRepo,
		mockAuditor,
		DefaultReservationReaperWorkerConfig(),
		logger,
		mockClock{fixedTime: fixedReaperTime()},
		"tenant-a",
		stubFailingPoolResolver{},
	)
	require.NoError(t, err)

	// runReapCycle is unexported; drive it directly via a single cycle. The cycle
	// must short-circuit at pool resolution before any repo call.
	worker.runReapCycle(context.Background())
}
