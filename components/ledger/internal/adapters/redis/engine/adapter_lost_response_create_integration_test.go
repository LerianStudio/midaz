//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestIntegration_CreateTransactionV2LostResponseRetainsRecoverableExecution(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-create-lost-response")
	ctx = libObservability.ContextWithHeaderID(ctx, "request-create-lost-response")
	inspector, address, password := newAdapterValkey(t)
	require.NoError(t, inspector.ScriptFlush(ctx).Err())
	proxy := newAccountingProxy(t, address, true)
	client := redis.NewClient(&redis.Options{
		Addr: proxy.listener.Addr().String(), Password: password, DB: 2, Protocol: 2,
		TLSConfig: proxy.clientTLS, MaxRetries: 3,
	})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	organizationID := uuid.MustParse("b1111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("b2222222-2222-4222-8222-222222222222")
	reader := &pendingLifecycleReader{
		settings: mmodel.LedgerSettings{Tracer: mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce}},
		balances: []*mmodel.Balance{
			adapterCreateBalance(organizationID, ledgerID, "b3333333-3333-4333-8333-333333333333", "b4444444-4444-4444-8444-444444444444", "@source", 100, 7),
			adapterCreateBalance(organizationID, ledgerID, "b5555555-5555-4555-8555-555555555555", "b6666666-6666-4666-8666-666666666666", "@target", 20, 3),
		},
	}
	ctrl := gomock.NewController(t)
	idempotency := txredis.NewMockRedisRepository(ctrl)
	var claimed atomic.Bool
	idempotency.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).DoAndReturn(
		func(context.Context, string, string, time.Duration) (bool, error) {
			claimed.Store(true)
			return true, nil
		},
	)

	realAdapter, err := newAdapterWithLimits(pendingLifecycleClientProvider{client: client}, guardBootstrapLimits())
	require.NoError(t, err)
	executor := &recordingCreateAdapter{delegate: realAdapter}
	finalizer := &adapterCreateFinalizer{}
	tracerControl := &pendingLifecycleTracer{reservationID: uuid.MustParse("b7777777-7777-4777-8777-777777777777")}
	uc := &command.UseCase{
		TransactionRedisRepo:        idempotency,
		TransactionReader:           reader,
		Engine:                      executor,
		AppliedTransactionCompleter: finalizer,
		TracerReserver:              tracerControl,
	}

	date := time.Date(2026, time.September, 8, 18, 0, 0, 0, time.UTC)
	amount := decimal.NewFromInt(30)
	created, replayed, err := uc.CreateTransactionV2(ctx, command.CreateTransactionV2Input{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Transaction: mtransaction.Transaction{
			Description:     "lost response create",
			TransactionDate: (*mtransaction.TransactionDate)(&date),
			Send: mtransaction.Send{
				Asset: "USD",
				Value: amount,
				Source: mtransaction.Source{From: []mtransaction.FromTo{{
					AccountAlias: "@source",
					Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
				}}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
					AccountAlias: "@target",
					Amount:       &mtransaction.Amount{Asset: "USD", Value: amount},
				}}},
			},
		},
		TransactionStatus: constant.CREATED,
		IdempotencyTTL:    time.Minute,
	})
	require.Nil(t, created)
	require.False(t, replayed)
	require.Error(t, err)
	var technical interface {
		EngineFailureCode() string
		OutcomeIndeterminate() bool
	}
	require.True(t, errors.As(err, &technical))
	require.Equal(t, "transport", technical.EngineFailureCode())
	require.True(t, technical.OutcomeIndeterminate())
	require.True(t, claimed.Load())
	require.Len(t, executor.inputs, 1)
	require.Nil(t, finalizer.envelope)
	require.Len(t, tracerControl.reserveRequests, 1)
	require.Empty(t, tracerControl.confirmedIDs)
	require.Empty(t, tracerControl.releasedIDs)
	require.Empty(t, tracerControl.confirmedTxns)
	require.Empty(t, tracerControl.releasedTxns)
	require.Equal(t, 1, proxy.count("EVALSHA"))
	require.Equal(t, 1, proxy.count("EVAL"))

	execution := executor.inputs[0]
	require.Equal(t, tracerControl.reserveRequests[0].TransactionID, execution.Execution.Transactions[0].ID)
	keys, err := resolveAdapterKeys(ctx, execution.Execution)
	require.NoError(t, err)
	t.Cleanup(func() { deleteMultiTransactionAcceptanceState(t, inspector, keys) })
	require.Equal(t, int64(1), inspector.HLen(ctx, keys.Guards).Val())
	require.Equal(t, execution.Guards[0].NextToken, inspector.HGet(ctx, keys.Guards, execution.Execution.Transactions[0].ID.String()).Val())
	require.Equal(t, int64(1), inspector.HLen(ctx, keys.Recovery).Val())
	require.Equal(t, int64(1), inspector.HLen(ctx, keys.Receipts).Val())
	assertPendingLifecycleBalances(t, ctx, inspector, keys, []pendingLifecycleBalanceExpectation{
		{ref: "@source#default", available: "70", onHold: "0", version: 8},
		{ref: "@target#default", available: "50", onHold: "0", version: 4},
	})

	recoveryRaw, err := inspector.HGet(ctx, keys.Recovery, execution.Execution.Transactions[0].ID.String()+":"+execution.Execution.ExecutionID.String()).Bytes()
	require.NoError(t, err)
	recovery, err := command.DecodeTransactionCompletionRecord(recoveryRaw)
	require.NoError(t, err)
	require.Equal(t, execution.Execution.ExecutionID, recovery.ExecutionID)
	require.Equal(t, execution.IntentFingerprint, recovery.IntentFingerprint)
	payload, err := command.DecodeTransactionCompletionPlan([]byte(recovery.Payload))
	require.NoError(t, err)
	projected, err := command.BuildOperationRecordsFromMovements(*payload, recovery.Result)
	require.NoError(t, err)
	require.Len(t, projected, 2)

	beforeRecoveryFinalization := captureAdapterState(t, inspector, keys)
	outcome, err := finalizer.Complete(ctx, recovery)
	require.NoError(t, err)
	require.Equal(t, constant.APPROVED, outcome.Outcome.TransactionStatus)
	require.Same(t, recovery, finalizer.envelope)
	require.Equal(t, beforeRecoveryFinalization, captureAdapterState(t, inspector, keys))
	require.Equal(t, 1, proxy.count("EVALSHA"))
	require.Equal(t, 1, proxy.count("EVAL"))
}
