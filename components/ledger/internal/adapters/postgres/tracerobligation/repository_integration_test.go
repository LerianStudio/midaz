// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package tracerobligation

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/tracerreservation"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

func TestObligationDurabilityAndFencing(t *testing.T) {
	infra := pgtestutil.SetupMigratedContainer(t, "transaction")
	ctx := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), dbresolver.New(dbresolver.WithPrimaryDBs(infra.DB)), constant.ModuleTransaction)
	intent, cfg := obligationFixture(t)
	repo, err := NewRepository(nil, cfg, true, 10)
	require.NoError(t, err)
	record, err := repo.Prepare(ctx, intent)
	require.NoError(t, err)
	require.Equal(t, tracerreservation.Prepared, record.State)
	require.Equal(t, intent.Payload, record.Intent.Payload)
	again, err := repo.Prepare(ctx, intent)
	require.NoError(t, err)
	require.Equal(t, record, again)
	// A reconstructed repository resumes the immutable intent after restart.
	restarted, err := NewRepository(nil, cfg, true, 10)
	require.NoError(t, err)
	altered := intent
	altered.ExecutionID = uuid.MustParse("fed05aab-99d7-44e2-9e13-38f81c77a663")
	_, err = restarted.Prepare(ctx, altered)
	require.ErrorIs(t, err, constant.ErrReserveDecisionConflict)
	require.NoError(t, restarted.BeginExecution(ctx, intent.Key, intent.CreatedAt.Add(time.Millisecond)))
	require.ErrorIs(t, restarted.BeginExecution(ctx, intent.Key, intent.CreatedAt.Add(2*time.Millisecond)), constant.ErrReserveOperationConflict)
	expired, err := restarted.ExpirePrepared(ctx, intent.Key, intent.PrepareDeadline.Add(time.Second))
	require.NoError(t, err)
	require.False(t, expired, "time never proves an executing operation aborted")
	require.NoError(t, restarted.SetOutcome(ctx, intent.Key, tracerreservation.Confirmed, intent.PrepareDeadline))
	require.NoError(t, restarted.SetOutcome(ctx, intent.Key, tracerreservation.Confirmed, intent.PrepareDeadline))
	require.ErrorIs(t, restarted.SetOutcome(ctx, intent.Key, tracerreservation.Released, intent.PrepareDeadline), constant.ErrReserveOperationConflict)
	due, err := restarted.ClaimDue(ctx, intent.PrepareDeadline, intent.PrepareDeadline.Add(time.Second), 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, tracerreservation.Confirmed, due[0].State)
	require.Equal(t, intent.Key, due[0].Key)
	require.Equal(t, intent.CreatedAt, due[0].CreatedAt.UTC())
	require.NoError(t, restarted.MarkDelivered(ctx, intent.Key, tracerreservation.Confirmed, intent.PrepareDeadline))
	require.NoError(t, restarted.MarkDelivered(ctx, intent.Key, tracerreservation.Confirmed, intent.PrepareDeadline))
	due, err = restarted.ClaimDue(ctx, intent.PrepareDeadline.Add(time.Hour), intent.PrepareDeadline.Add(2*time.Hour), 10)
	require.NoError(t, err)
	require.Empty(t, due)
	_, err = infra.DB.ExecContext(ctx, `UPDATE tracer_reservation_obligation SET payload='tampered'::bytea`)
	require.Error(t, err)
	_, err = infra.DB.ExecContext(ctx, `DELETE FROM tracer_reservation_obligation`)
	require.Error(t, err)
	down, err := os.ReadFile("../../../../migrations/transaction/000036_create_tracer_reservation_obligation.down.sql")
	require.NoError(t, err)
	_, err = infra.DB.ExecContext(ctx, string(down))
	require.Error(t, err, "rollback must preserve coordination history")
}

func TestObligationBatchFenceIsAtomicAndOrdered(t *testing.T) {
	infra := pgtestutil.SetupMigratedContainer(t, "transaction")
	ctx := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), dbresolver.New(dbresolver.WithPrimaryDBs(infra.DB)), constant.ModuleTransaction)
	first, cfg := obligationFixture(t)
	repo, err := NewRepository(nil, cfg, true, 10)
	require.NoError(t, err)
	_, err = repo.Prepare(ctx, first)
	require.NoError(t, err)
	request, err := first.Request(ctx, cfg)
	require.NoError(t, err)
	request.TransactionID = uuid.MustParse("ffffffff-ffff-4fff-8fff-ffffffffffff")
	request.RequestID = uuid.MustParse("88888888-8888-4888-8888-888888888888")
	secondKey := first.Key
	secondKey.TransactionID = request.TransactionID
	second, err := tracerreservation.NewIntent(ctx, secondKey, first.ExecutionID, first.Scope, request, first.CreatedAt, first.PrepareDeadline, cfg)
	require.NoError(t, err)
	keys := []tracerreservation.Key{first.Key, second.Key}
	at := first.CreatedAt.Add(time.Millisecond)
	require.ErrorIs(t, repo.BeginExecutions(ctx, keys, at), constant.ErrReserveOperationConflict)
	stored, err := repo.Find(ctx, first.Key)
	require.NoError(t, err)
	require.Equal(t, tracerreservation.Prepared, stored.State, "missing second member rolls back first acquisition")
	_, err = repo.Prepare(ctx, second)
	require.NoError(t, err)
	require.ErrorIs(t, repo.BeginExecutions(ctx, []tracerreservation.Key{first.Key, first.Key}, at), constant.ErrInvalidRequestBody)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, order := range [][]tracerreservation.Key{keys, {second.Key, first.Key}} {
		go func() { <-start; results <- repo.BeginExecutions(ctx, order, at) }()
	}
	close(start)
	left, right := <-results, <-results
	if left == nil {
		require.ErrorIs(t, right, constant.ErrReserveOperationConflict)
	} else {
		require.ErrorIs(t, left, constant.ErrReserveOperationConflict)
		require.NoError(t, right)
	}
	for _, key := range keys {
		stored, err := repo.Find(ctx, key)
		require.NoError(t, err)
		require.Equal(t, tracerreservation.Executing, stored.State)
		foreign, err := repo.Find(tmcore.ContextWithTenantID(ctx, "tenant-b"), key)
		require.NoError(t, err)
		require.Nil(t, foreign)
	}
}

func TestObligationExpiryRacesExecution(t *testing.T) {
	infra := pgtestutil.SetupMigratedContainer(t, "transaction")
	ctx := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), dbresolver.New(dbresolver.WithPrimaryDBs(infra.DB)), constant.ModuleTransaction)
	intent, cfg := obligationFixture(t)
	repo, err := NewRepository(nil, cfg, true, 10)
	require.NoError(t, err)
	_, err = repo.Prepare(ctx, intent)
	require.NoError(t, err)
	start := make(chan struct{})
	var wg sync.WaitGroup
	var beginErr, expiryErr error
	var expired bool
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		beginErr = repo.BeginExecution(ctx, intent.Key, intent.CreatedAt.Add(time.Millisecond))
	}()
	go func() {
		defer wg.Done()
		<-start
		expired, expiryErr = repo.ExpirePrepared(ctx, intent.Key, intent.PrepareDeadline)
	}()
	close(start)
	wg.Wait()
	require.NoError(t, expiryErr)
	if expired {
		require.ErrorIs(t, beginErr, constant.ErrReserveOperationConflict)
	} else {
		require.NoError(t, beginErr)
	}
	var state string
	require.NoError(t, infra.DB.QueryRowContext(ctx, `SELECT state FROM tracer_reservation_obligation`).Scan(&state))
	require.Contains(t, []string{"EXECUTING", "RELEASED"}, state)
}

func TestObligationTenantAndRecoveryBounds(t *testing.T) {
	primary := pgtestutil.SetupMigratedContainer(t, "transaction")
	other := pgtestutil.SetupMigratedContainer(t, "transaction")
	ctx := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), dbresolver.New(dbresolver.WithPrimaryDBs(primary.DB), dbresolver.WithReplicaDBs(other.DB)), constant.ModuleTransaction)
	intent, cfg := obligationFixture(t)
	repo, err := NewRepository(nil, cfg, true, 10)
	require.NoError(t, err)
	_, err = repo.Prepare(ctx, intent)
	require.NoError(t, err)
	_, err = repo.Prepare(context.Background(), intent)
	require.Error(t, err)
	wrongTenant := tmcore.ContextWithTenantID(ctx, "tenant-b")
	_, err = repo.Prepare(wrongTenant, intent)
	require.Error(t, err)
	otherCtx := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-b"), dbresolver.New(dbresolver.WithPrimaryDBs(other.DB)), constant.ModuleTransaction)
	due, err := repo.ClaimDue(otherCtx, intent.PrepareDeadline, intent.PrepareDeadline.Add(time.Second), 10)
	require.NoError(t, err)
	require.Empty(t, due)
	// Delivery scans do not load or revalidate old request bodies against newly
	// reduced admission bounds. A settings/budget change must not strand holds.
	small := cfg
	small.MaxBodyBytes = 1
	drain, err := NewRepository(nil, small, true, 10)
	require.NoError(t, err)
	due, err = drain.ClaimDue(ctx, intent.PrepareDeadline, intent.PrepareDeadline.Add(time.Second), 10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	_, err = drain.ClaimDue(ctx, intent.PrepareDeadline, intent.PrepareDeadline.Add(time.Second), 11)
	require.Error(t, err)
	due, err = drain.ClaimDue(ctx, intent.PrepareDeadline, intent.PrepareDeadline.Add(time.Second), 10)
	require.NoError(t, err)
	require.Empty(t, due, "claimed work is rescheduled before network I/O")
}

func TestObligationAccountingEvidenceUsesTenantPrimary(t *testing.T) {
	primary := pgtestutil.SetupMigratedContainer(t, "transaction")
	replica := pgtestutil.SetupMigratedContainer(t, "transaction")
	intent, cfg := obligationFixture(t)
	key := intent.Key
	ctx := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), dbresolver.New(dbresolver.WithPrimaryDBs(primary.DB), dbresolver.WithReplicaDBs(replica.DB)), constant.ModuleTransaction)
	repo, err := NewRepository(nil, cfg, true, 10)
	require.NoError(t, err)
	status, err := repo.ReadAccountingStatus(ctx, key)
	require.NoError(t, err)
	require.Empty(t, status, "absence is not an abort")
	_, err = primary.DB.ExecContext(ctx, `INSERT INTO transaction (id,organization_id,ledger_id,description,status,amount,asset_code,chart_of_accounts_group_name,body,created_at,updated_at) VALUES ($1,$2,$3,'evidence','APPROVED',10.125,'BTC','','{}',$4,$4)`, key.TransactionID, key.OrganizationID, key.LedgerID, intent.CreatedAt)
	require.NoError(t, err)
	status, err = repo.ReadAccountingStatus(ctx, key)
	require.NoError(t, err)
	require.Equal(t, "APPROVED", status)
	foreign := key
	foreign.LedgerID = key.OrganizationID
	status, err = repo.ReadAccountingStatus(ctx, foreign)
	require.NoError(t, err)
	require.Empty(t, status)
	otherCtx := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-b"), dbresolver.New(dbresolver.WithPrimaryDBs(replica.DB)), constant.ModuleTransaction)
	status, err = repo.ReadAccountingStatus(otherCtx, key)
	require.NoError(t, err)
	require.Empty(t, status)
	// Empty coordination tables can roll back without touching accounting rows.
	down, err := os.ReadFile("../../../../migrations/transaction/000036_create_tracer_reservation_obligation.down.sql")
	require.NoError(t, err)
	_, err = primary.DB.ExecContext(ctx, string(down))
	require.NoError(t, err)
	status, err = repo.ReadAccountingStatus(ctx, key)
	require.NoError(t, err)
	require.Equal(t, "APPROVED", status)
}
