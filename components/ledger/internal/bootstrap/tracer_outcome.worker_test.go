// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

type outcomeApplierStub struct {
	err      error
	requests []tracerclient.ApplyOutcomeRequest
	tenants  []string
}

type outcomeTenantLoaderStub struct {
	loaded []string
	err    error
}

func (s *outcomeTenantLoaderStub) LoadTenant(_ context.Context, tenantID string) (*tmcore.TenantConfig, error) {
	s.loaded = append(s.loaded, tenantID)
	if s.err != nil {
		return nil, s.err
	}

	return &tmcore.TenantConfig{}, nil
}

func (s *outcomeApplierStub) ApplyOutcome(ctx context.Context, req tracerclient.ApplyOutcomeRequest) (*tracerclient.ApplyOutcomeResult, error) {
	s.requests = append(s.requests, req)
	s.tenants = append(s.tenants, tmcore.GetTenantIDContext(ctx))
	if s.err != nil {
		return nil, s.err
	}
	return &tracerclient.ApplyOutcomeResult{
		TransactionID: req.TransactionID, OutcomeID: req.OutcomeID, Outcome: req.Outcome,
	}, nil
}

func TestTracerOutcomeWorkerMultiTenantDiscoversDurableBacklogAfterRestart(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := redisTransaction.NewMockRedisRepository(ctrl)
	applier := &outcomeApplierStub{}
	loader := &outcomeTenantLoaderStub{}
	worker := NewTracerOutcomeWorkerMT(newTestLogger(), repo, applier,
		TracerOutcomeWorkerConfig{DeliveredTTL: time.Hour}, loader)
	now := time.Unix(1700000000, 0).UTC()
	worker.now = func() time.Time { return now }
	record := completeWorkerOutcome(mmodel.TracerOutcomeCommitted)
	key := utils.TransactionTracerOutcomeKey(record.OrganizationID, record.LedgerID, record.TransactionID)
	repo.EXPECT().ListTracerOutcomeTenants(gomock.Any()).Return([]redisTransaction.TracerOutcomeTenantRegistration{
		{TenantID: "tenant-a", Generation: 4},
		{TenantID: "tenant-b", Generation: 9},
	}, nil)
	repo.EXPECT().AcquireOwnedKey(gomock.Any(), utils.TracerOutcomeDispatcherLock, worker.owner, gomock.Any()).Return(true, nil).Times(2)
	repo.EXPECT().ListDueTracerOutcomes(gomock.Any(), now, int64(100)).Return([]string{key}, nil).Times(2)
	repo.EXPECT().ReadTracerOutcomeByKey(gomock.Any(), key).Return(record, nil).Times(2)
	repo.EXPECT().MarkTracerOutcomeDelivered(gomock.Any(), key, record.OutcomeID, record.State, now, time.Hour).Return(true, nil).Times(2)
	repo.EXPECT().ReleaseOwnedKey(gomock.Any(), utils.TracerOutcomeDispatcherLock, worker.owner).Return(true, nil).Times(2)
	repo.EXPECT().RetireTracerOutcomeTenant(gomock.Any(), "tenant-a", int64(4)).Return(true, nil)
	repo.EXPECT().RetireTracerOutcomeTenant(gomock.Any(), "tenant-b", int64(9)).Return(true, nil)

	worker.runCycle(context.Background())
	assert.ElementsMatch(t, []string{"tenant-a", "tenant-b"}, loader.loaded)
	assert.ElementsMatch(t, []string{"tenant-a", "tenant-b"}, applier.tenants)
}

func TestTracerOutcomeWorkerMultiTenantKeepsDeletedTenantWithBacklog(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := redisTransaction.NewMockRedisRepository(ctrl)
	loader := &outcomeTenantLoaderStub{err: tmcore.ErrTenantNotFound}
	worker := NewTracerOutcomeWorkerMT(newTestLogger(), repo, &outcomeApplierStub{},
		TracerOutcomeWorkerConfig{}, loader)
	repo.EXPECT().ListTracerOutcomeTenants(gomock.Any()).Return([]redisTransaction.TracerOutcomeTenantRegistration{
		{TenantID: "deleted-tenant", Generation: 3},
	}, nil)
	repo.EXPECT().RetireTracerOutcomeTenant(gomock.Any(), "deleted-tenant", int64(3)).Return(false, nil)

	worker.runCycle(context.Background())
	assert.Equal(t, []string{"deleted-tenant"}, loader.loaded)
}

func completeWorkerOutcome(state string) *mmodel.TracerOutcomeRecord {
	txID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	outcome := mmodel.TransactionOutcomeCommitted
	if state == mmodel.TracerOutcomeAborted {
		outcome = mmodel.TransactionOutcomeAborted
	}
	record := &mmodel.TracerOutcomeRecord{
		Version: mmodel.TracerOutcomeVersion, TransactionID: txID,
		OutcomeID: utils.TransactionTracerOutcomeID(txID), OrganizationID: uuid.New(), LedgerID: uuid.New(),
		State: state, Owner: "owner", EconomicPlanVersion: "1", EconomicPlanDigest: "digest",
		PreparedAtUnixMS: 1, UpdatedAtUnixMS: 2,
	}
	if state != mmodel.TracerOutcomePrepared {
		record.EconomicOutcome = &mmodel.BalanceExecutionOutcome{
			Identity: txID, Outcome: outcome, Owner: "owner",
			EconomicPlanVersion: "1", EconomicPlanDigest: "digest",
		}
	}
	return record
}

func TestTracerOutcomeWorkerLeadershipFencesFollowers(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := redisTransaction.NewMockRedisRepository(ctrl)
	worker := NewTracerOutcomeWorker(newTestLogger(), repo, &outcomeApplierStub{}, TracerOutcomeWorkerConfig{})
	repo.EXPECT().AcquireOwnedKey(gomock.Any(), utils.TracerOutcomeDispatcherLock, worker.owner, gomock.Any()).Return(false, nil)

	worker.runTenantCycle(context.Background())
}

func TestTracerOutcomeWorkerLifecycleStopsOnCancellation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := redisTransaction.NewMockRedisRepository(ctrl)
	worker := NewTracerOutcomeWorker(newTestLogger(), repo, &outcomeApplierStub{}, TracerOutcomeWorkerConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo.EXPECT().AcquireOwnedKey(gomock.Any(), utils.TracerOutcomeDispatcherLock, worker.owner, gomock.Any()).Return(false, nil)

	require.NoError(t, worker.RunContext(ctx))
	require.False(t, NewTracerOutcomeWorker(newTestLogger(), nil, &outcomeApplierStub{}, TracerOutcomeWorkerConfig{}).Ready())
}

func TestTracerOutcomeWorkerLostAckReschedulesWithBackoff(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := redisTransaction.NewMockRedisRepository(ctrl)
	applier := &outcomeApplierStub{err: errors.New("ack lost")}
	worker := NewTracerOutcomeWorker(newTestLogger(), repo, applier, TracerOutcomeWorkerConfig{})
	now := time.Unix(1700000000, 0).UTC()
	worker.backoff = func(int) time.Duration { return 2 * time.Second }
	record := completeWorkerOutcome(mmodel.TracerOutcomeCommitted)
	key := utils.TransactionTracerOutcomeKey(record.OrganizationID, record.LedgerID, record.TransactionID)
	repo.EXPECT().ReadTracerOutcomeByKey(gomock.Any(), key).Return(record, nil)
	repo.EXPECT().RescheduleTracerOutcome(gomock.Any(), key, record.OutcomeID, record.State,
		"ack lost", now, now.Add(2*time.Second)).Return(nil)

	worker.dispatchOne(context.Background(), key, now)
	require.Len(t, applier.requests, 1)
	assert.Equal(t, tracerclient.ReservationOutcomeCommitted, applier.requests[0].Outcome)
}

func TestTracerOutcomeWorkerExactReplayMarksDelivered(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	repo := redisTransaction.NewMockRedisRepository(ctrl)
	applier := &outcomeApplierStub{}
	worker := NewTracerOutcomeWorker(newTestLogger(), repo, applier, TracerOutcomeWorkerConfig{DeliveredTTL: time.Hour})
	now := time.Unix(1700000000, 0).UTC()
	record := completeWorkerOutcome(mmodel.TracerOutcomeAborted)
	key := utils.TransactionTracerOutcomeKey(record.OrganizationID, record.LedgerID, record.TransactionID)
	repo.EXPECT().ReadTracerOutcomeByKey(gomock.Any(), key).Return(record, nil)
	repo.EXPECT().MarkTracerOutcomeDelivered(gomock.Any(), key, record.OutcomeID, record.State, now, time.Hour).Return(true, nil)

	worker.dispatchOne(context.Background(), key, now)
	require.Len(t, applier.requests, 1)
	assert.Equal(t, tracerclient.ReservationOutcomeAborted, applier.requests[0].Outcome)
}

func TestTracerOutcomeWorkerRecoversOnlyPreparedNeverPendingHeld(t *testing.T) {
	t.Parallel()

	t.Run("stale prepared becomes aborted and is delivered", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := redisTransaction.NewMockRedisRepository(ctrl)
		applier := &outcomeApplierStub{}
		worker := NewTracerOutcomeWorker(newTestLogger(), repo, applier,
			TracerOutcomeWorkerConfig{PreparedTimeout: time.Second, DeliveredTTL: time.Hour})
		now := time.Unix(1700000000, 0).UTC()
		prepared := completeWorkerOutcome(mmodel.TracerOutcomePrepared)
		prepared.PreparedAtUnixMS = now.Add(-time.Minute).UnixMilli()
		aborted := completeWorkerOutcome(mmodel.TracerOutcomeAborted)
		aborted.OrganizationID, aborted.LedgerID, aborted.OutcomeID = prepared.OrganizationID, prepared.LedgerID, prepared.OutcomeID
		key := utils.TransactionTracerOutcomeKey(prepared.OrganizationID, prepared.LedgerID, prepared.TransactionID)
		repo.EXPECT().ReadTracerOutcomeByKey(gomock.Any(), key).Return(prepared, nil)
		repo.EXPECT().AbortPreparedTracerOutcome(gomock.Any(), prepared.OrganizationID, prepared.LedgerID,
			prepared.TransactionID, prepared.Owner, prepared.OutcomeID, now).Return(aborted, nil)
		repo.EXPECT().MarkTracerOutcomeDelivered(gomock.Any(), key, aborted.OutcomeID, aborted.State, now, time.Hour).Return(true, nil)

		worker.dispatchOne(context.Background(), key, now)
		require.Len(t, applier.requests, 1)
		assert.Equal(t, tracerclient.ReservationOutcomeAborted, applier.requests[0].Outcome)
	})

	t.Run("pending held is unscheduled without outcome", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		repo := redisTransaction.NewMockRedisRepository(ctrl)
		applier := &outcomeApplierStub{}
		worker := NewTracerOutcomeWorker(newTestLogger(), repo, applier, TracerOutcomeWorkerConfig{})
		now := time.Unix(1700000000, 0).UTC()
		record := completeWorkerOutcome(mmodel.TracerOutcomePendingHeld)
		key := utils.TransactionTracerOutcomeKey(record.OrganizationID, record.LedgerID, record.TransactionID)
		repo.EXPECT().ReadTracerOutcomeByKey(gomock.Any(), key).Return(record, nil)
		repo.EXPECT().RemoveTracerOutcomeSchedule(gomock.Any(), key).Return(nil)

		worker.dispatchOne(context.Background(), key, now)
		assert.Empty(t, applier.requests)
	})
}
