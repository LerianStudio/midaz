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

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// fixedReaperTime is the deterministic "now" used across reaper tests. Per the
// tracer test rules, tests never call time.Now() — the clock is injected.
func fixedReaperTime() time.Time {
	return time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
}

// reaperOperation is the operation that owns the reservation seeded with id.
func reaperOperation(id uuid.UUID) model.ReserveOperationIdentity {
	return model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: id}
}

// ownedExpired returns one expired reservation per id, each owned by its own
// operation.
func ownedExpired(ids ...uuid.UUID) []model.ExpiredReservation {
	expired := make([]model.ExpiredReservation, 0, len(ids))
	for _, id := range ids {
		expired = append(expired, model.ExpiredReservation{ID: id, Operation: reaperOperation(id)})
	}

	return expired
}

// unusedExpirer satisfies the required expirer for sweeps that find nothing; it
// fails any call.
type unusedExpirer struct{}

func (unusedExpirer) Execute(context.Context, model.ReserveOperationIdentity, time.Time) (int, error) {
	return 0, errors.New("empty sweep expired an operation")
}

func TestNewReservationReaperWorker(t *testing.T) {
	tests := []struct {
		name        string
		config      ReservationReaperWorkerConfig
		nilRepo     bool
		nilExpirer  bool
		nilLogger   bool
		expectError error
	}{
		{
			name:        "creates worker with valid config",
			config:      ReservationReaperWorkerConfig{ReapInterval: 30 * time.Second, BatchSize: 10},
			expectError: nil,
		},
		{
			name:        "returns error when expirer is nil",
			config:      ReservationReaperWorkerConfig{ReapInterval: 30 * time.Second, BatchSize: 10},
			nilExpirer:  true,
			expectError: ErrNilOperationExpirer,
		},
		{
			name:        "returns error when batch size is zero",
			config:      ReservationReaperWorkerConfig{ReapInterval: 30 * time.Second},
			expectError: ErrInvalidReaperBatchSize,
		},
		{
			name:        "returns error when repository is nil",
			config:      ReservationReaperWorkerConfig{ReapInterval: 30 * time.Second},
			nilRepo:     true,
			expectError: ErrNilRepository,
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

			var expirer ReserveOperationExpirer = unusedExpirer{}
			if tt.nilExpirer {
				expirer = nil
			}

			var logger libLog.Logger = testutil.NewMockLogger()
			if tt.nilLogger {
				logger = nil
			}

			worker, err := NewReservationReaperWorkerWithPoolResolver(repo, expirer, tt.config, logger, nil, "", nil)

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

// TestReservationReaperWorker_RunOnce_ReleasesExpired asserts that the owning
// operation of every expired reservation returned by the repo is expired once
// and the sweep reports what the expirer moved.
func TestReservationReaperWorker_RunOnce_ReleasesExpired(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockExpirer := mocks.NewMockReserveOperationExpirer(ctrl)
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
		FindExpiredReservations(gomock.Any(), now.UTC(), nil, DefaultReservationReaperBatchSize).
		Return(ownedExpired(expired...), nil).
		Times(1)

	for _, id := range expired {
		mockExpirer.EXPECT().
			Execute(gomock.Any(), reaperOperation(id), now.UTC()).
			Return(1, nil).
			Times(1)
	}

	worker, err := NewReservationReaperWorkerWithPoolResolver(mockRepo, mockExpirer, DefaultReservationReaperWorkerConfig(), logger, testClock, "", nil)
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())

	require.NoError(t, err)
	assert.Equal(t, len(expired), released)
}

// TestReservationReaperWorker_RunOnce_FreshUntouched asserts that when nothing is
// expired, the reaper expires nothing.
func TestReservationReaperWorker_RunOnce_FreshUntouched(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	logger := testutil.NewMockLogger()

	now := fixedReaperTime()
	testClock := mockClock{fixedTime: now}

	mockRepo.EXPECT().
		FindExpiredReservations(gomock.Any(), now.UTC(), nil, DefaultReservationReaperBatchSize).
		Return(nil, nil).
		Times(1)

	// unusedExpirer fails the sweep if any operation is expired.

	worker, err := NewReservationReaperWorkerWithPoolResolver(mockRepo, unusedExpirer{}, DefaultReservationReaperWorkerConfig(), logger, testClock, "", nil)
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())

	require.NoError(t, err)
	assert.Equal(t, 0, released)
}

// ttlPredicateRepo is a fake ReservationReaperRepository that honors the
// FindExpiredReservations contract (return only rows whose reservation_expires_at
// is strictly before now) against an in-memory set of seeded reservations, and a
// ReserveOperationExpirer that expires them. Unlike the gomock mock, it does NOT
// let the test script which rows come back — the TTL predicate decides, so the
// test proves the reaper respects expiry rather than asserting a hand-fed slice.
type ttlPredicateRepo struct {
	expiresAt map[uuid.UUID]time.Time
	released  []uuid.UUID
}

func (r *ttlPredicateRepo) FindExpiredReservations(_ context.Context, now time.Time, _ *model.ReservationExpiryPosition, _ int) ([]model.ExpiredReservation, error) {
	var ids []uuid.UUID

	for id, exp := range r.expiresAt {
		if exp.Before(now) {
			ids = append(ids, id)
		}
	}

	return ownedExpired(ids...), nil
}

func (r *ttlPredicateRepo) Execute(_ context.Context, key model.ReserveOperationIdentity, _ time.Time) (int, error) {
	r.released = append(r.released, key.TransactionID)
	delete(r.expiresAt, key.TransactionID)

	return 1, nil
}

// TestReservationReaperWorker_RunOnce_LongLivedNotSweptBeforeTTL proves the R18
// guarantee at the reaper seam: a long-lived (PENDING) reservation whose
// reservation_expires_at is far in the future is NOT reaped while the clock is
// before its TTL, and IS reaped once the clock advances past it. A short-TTL
// (direct) reservation alongside it expires immediately, so the test also proves
// the two TTL classes are swept independently.
func TestReservationReaperWorker_RunOnce_LongLivedNotSweptBeforeTTL(t *testing.T) {
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	now := fixedReaperTime()

	directID := testutil.MustDeterministicUUID(1)
	longLivedID := testutil.MustDeterministicUUID(2)

	repo := &ttlPredicateRepo{
		expiresAt: map[uuid.UUID]time.Time{
			// Direct reservation: short TTL, already expired relative to now.
			directID: now.Add(-1 * time.Minute),
			// Long-lived PENDING reservation: 30-day TTL, far past now.
			longLivedID: now.Add(720 * time.Hour),
		},
	}

	logger := testutil.NewMockLogger()

	// First sweep at now: only the direct row is past its TTL; the long-lived row
	// must survive.
	worker, err := NewReservationReaperWorkerWithPoolResolver(repo, repo, DefaultReservationReaperWorkerConfig(), logger, mockClock{fixedTime: now}, "", nil)
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, released, "only the expired direct reservation is reaped")
	require.Len(t, repo.released, 1)
	assert.Equal(t, directID, repo.released[0])
	assert.Contains(t, repo.expiresAt, longLivedID, "the long-lived reservation must NOT be swept before its TTL")

	// Advance the clock past the long-lived TTL: now the reaper sweeps it too.
	afterTTL := now.Add(721 * time.Hour)
	workerAfter, err := NewReservationReaperWorkerWithPoolResolver(repo, repo, DefaultReservationReaperWorkerConfig(), logger, mockClock{fixedTime: afterTTL}, "", nil)
	require.NoError(t, err)

	released, err = workerAfter.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, released, "once the clock passes the long-lived TTL the row is reaped")
	require.Len(t, repo.released, 2)
	assert.Equal(t, longLivedID, repo.released[1])
}

// TestReservationReaperWorker_RunOnce_FindError asserts a find failure is returned
// and no operation is expired.
func TestReservationReaperWorker_RunOnce_FindError(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	logger := testutil.NewMockLogger()

	now := fixedReaperTime()
	testClock := mockClock{fixedTime: now}

	mockRepo.EXPECT().
		FindExpiredReservations(gomock.Any(), now.UTC(), nil, DefaultReservationReaperBatchSize).
		Return(nil, errors.New("db down")).
		Times(1)

	worker, err := NewReservationReaperWorkerWithPoolResolver(mockRepo, unusedExpirer{}, DefaultReservationReaperWorkerConfig(), logger, testClock, "", nil)
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())

	require.Error(t, err)
	assert.Equal(t, 0, released)
	assert.Contains(t, err.Error(), "failed to find expired reservations")
}

// TestReservationReaperWorker_Cadence asserts the worker honors the ticker: each
// tick drives a sweep. A controllable ticker channel lets the test pump exactly
// the number of cycles it wants without sleeping on wall-clock.
func TestReservationReaperWorker_Cadence(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	logger := testutil.NewMockLogger()

	now := fixedReaperTime()
	tickerChan := make(chan time.Time)
	testClock := mockClock{fixedTime: now, tickerChan: tickerChan}

	// Count sweeps via the find call (one per cycle). The initial cycle on start
	// plus N ticks => N+1 sweeps. Each sweep finds nothing, so nothing expires.
	sweeps := make(chan struct{}, 8)

	mockRepo.EXPECT().
		FindExpiredReservations(gomock.Any(), now.UTC(), nil, DefaultReservationReaperBatchSize).
		DoAndReturn(func(_ context.Context, _ time.Time, _ *model.ReservationExpiryPosition, _ int) ([]model.ExpiredReservation, error) {
			sweeps <- struct{}{}
			return nil, nil
		}).
		MinTimes(3)

	worker, err := NewReservationReaperWorkerWithPoolResolver(mockRepo, unusedExpirer{}, ReservationReaperWorkerConfig{ReapInterval: time.Hour, BatchSize: DefaultReservationReaperBatchSize}, logger, testClock, "", nil)
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
	logger := testutil.NewMockLogger()

	// No repo calls expected: gomock fails if FindExpiredReservations runs.

	worker, err := NewReservationReaperWorkerWithPoolResolver(mockRepo, unusedExpirer{}, DefaultReservationReaperWorkerConfig(), logger, mockClock{fixedTime: fixedReaperTime()}, "", nil)
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
	logger := testutil.NewMockLogger()

	// No repo calls expected when the pool fails to resolve.

	worker, err := NewReservationReaperWorkerWithPoolResolver(
		mockRepo,
		unusedExpirer{},
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

func TestReservationReaperWorker_RunOnce_ExpiresEachOperationOnce(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockExpirer := mocks.NewMockReserveOperationExpirer(ctrl)

	now := fixedReaperTime()
	opA := model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(12)}
	opB := model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(13)}
	// The same transaction under another producer is a different operation.
	opC := model.ReserveOperationIdentity{IntegrationID: "other-producer", TransactionID: opA.TransactionID}

	mockRepo.EXPECT().FindExpiredReservations(gomock.Any(), now.UTC(), nil, DefaultReservationReaperBatchSize).Return([]model.ExpiredReservation{
		{ID: testutil.MustDeterministicUUID(21), Operation: opA},
		{ID: testutil.MustDeterministicUUID(22), Operation: opB},
		{ID: testutil.MustDeterministicUUID(23), Operation: opA},
		{ID: testutil.MustDeterministicUUID(24), Operation: opC},
	}, nil)
	mockExpirer.EXPECT().Execute(gomock.Any(), opA, now.UTC()).Return(2, nil).Times(1)
	mockExpirer.EXPECT().Execute(gomock.Any(), opB, now.UTC()).Return(0, nil).Times(1)
	mockExpirer.EXPECT().Execute(gomock.Any(), opC, now.UTC()).Return(1, nil).Times(1)

	worker, err := NewReservationReaperWorkerWithPoolResolver(mockRepo, mockExpirer, DefaultReservationReaperWorkerConfig(), testutil.NewMockLogger(), mockClock{fixedTime: now}, "", nil)
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, released, "two rows of A and one of C; B was already terminal")
}

// TestReservationReaperWorker_RunOnce_CountsWhatTheExpirerMoved asserts the sweep
// reports the expirer's released count, not the number of rows it read.
func TestReservationReaperWorker_RunOnce_CountsWhatTheExpirerMoved(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockExpirer := mocks.NewMockReserveOperationExpirer(ctrl)

	now := fixedReaperTime()
	op := model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(31)}

	mockRepo.EXPECT().FindExpiredReservations(gomock.Any(), now.UTC(), nil, DefaultReservationReaperBatchSize).Return([]model.ExpiredReservation{{ID: testutil.MustDeterministicUUID(32), Operation: op}}, nil)
	mockExpirer.EXPECT().Execute(gomock.Any(), op, now.UTC()).Return(3, nil)

	worker, err := NewReservationReaperWorkerWithPoolResolver(mockRepo, mockExpirer, DefaultReservationReaperWorkerConfig(), testutil.NewMockLogger(), mockClock{fixedTime: now}, "", nil)
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, released, "the count is what the expirer moved, including rows the sweep did not read")
}

func TestReservationReaperWorker_RunOnce_OperationFailureDoesNotStopSweep(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockExpirer := mocks.NewMockReserveOperationExpirer(ctrl)

	now := fixedReaperTime()
	failure := errors.New("expire failed")
	opA := model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(42)}
	opB := model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(43)}

	mockRepo.EXPECT().FindExpiredReservations(gomock.Any(), now.UTC(), nil, DefaultReservationReaperBatchSize).Return([]model.ExpiredReservation{
		{ID: testutil.MustDeterministicUUID(44), Operation: opA},
		{ID: testutil.MustDeterministicUUID(45), Operation: opB},
	}, nil)
	mockExpirer.EXPECT().Execute(gomock.Any(), opA, now.UTC()).Return(0, failure)
	mockExpirer.EXPECT().Execute(gomock.Any(), opB, now.UTC()).Return(1, nil)

	worker, err := NewReservationReaperWorkerWithPoolResolver(mockRepo, mockExpirer, DefaultReservationReaperWorkerConfig(), testutil.NewMockLogger(), mockClock{fixedTime: now}, "", nil)
	require.NoError(t, err)

	released, err := worker.RunOnce(context.Background())
	require.ErrorIs(t, err, failure)
	assert.Equal(t, 1, released, "B expired although A failed")
}

// TestReservationReaperWorker_RunOnce_ResumesPastAFullPage pins the sweep walk:
// a full page moves the next sweep past its last row even when an operation on
// it failed, a short page returns the walk to the oldest expiry, an empty page
// past the resume position rereads from the oldest expiry in the same sweep, and
// a failed read keeps the resume position.
func TestReservationReaperWorker_RunOnce_ResumesPastAFullPage(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	mockRepo := mocks.NewMockReservationReaperRepository(ctrl)
	mockExpirer := mocks.NewMockReserveOperationExpirer(ctrl)

	now := fixedReaperTime()
	failing := model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(51)}
	head := []model.ExpiredReservation{
		{ID: testutil.MustDeterministicUUID(52), ExpiresAt: now.Add(-3 * time.Minute), Operation: failing},
		{ID: testutil.MustDeterministicUUID(53), ExpiresAt: now.Add(-3 * time.Minute), Operation: failing},
	}
	behind := model.ExpiredReservation{ID: testutil.MustDeterministicUUID(54), ExpiresAt: now.Add(-time.Minute), Operation: reaperOperation(testutil.MustDeterministicUUID(55))}
	pastHead := head[1].Position()
	failure := errors.New("expire failed")

	gomock.InOrder(
		mockRepo.EXPECT().FindExpiredReservations(gomock.Any(), now.UTC(), nil, 2).Return(head, nil),
		mockRepo.EXPECT().FindExpiredReservations(gomock.Any(), now.UTC(), &pastHead, 2).Return(nil, errors.New("read failed")),
		mockRepo.EXPECT().FindExpiredReservations(gomock.Any(), now.UTC(), &pastHead, 2).Return([]model.ExpiredReservation{behind}, nil),
		mockRepo.EXPECT().FindExpiredReservations(gomock.Any(), now.UTC(), nil, 2).Return(head, nil),
		mockRepo.EXPECT().FindExpiredReservations(gomock.Any(), now.UTC(), &pastHead, 2).Return(nil, nil),
		mockRepo.EXPECT().FindExpiredReservations(gomock.Any(), now.UTC(), nil, 2).Return(head, nil),
	)
	mockExpirer.EXPECT().Execute(gomock.Any(), failing, now.UTC()).Return(0, failure).Times(3)
	mockExpirer.EXPECT().Execute(gomock.Any(), behind.Operation, now.UTC()).Return(1, nil)

	worker, err := NewReservationReaperWorkerWithPoolResolver(mockRepo, mockExpirer,
		ReservationReaperWorkerConfig{ReapInterval: time.Second, BatchSize: 2}, testutil.NewMockLogger(), mockClock{fixedTime: now}, "", nil)
	require.NoError(t, err)

	_, err = worker.RunOnce(context.Background())
	require.ErrorIs(t, err, failure, "the failing operation fills the first page")

	_, err = worker.RunOnce(context.Background())
	require.Error(t, err, "the failed read keeps the resume position")

	released, err := worker.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, released, "the row behind the failing operation expires")

	_, err = worker.RunOnce(context.Background())
	require.ErrorIs(t, err, failure, "the short page returned the walk to the oldest expiry")

	_, err = worker.RunOnce(context.Background())
	require.ErrorIs(t, err, failure, "an empty page past the head rereads from the oldest expiry")
}
