// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// schemeWiringCases is the shared table for the scheme wiring proofs: a declared
// scheme must reach the tracer verbatim and an undeclared one must reach it as the
// empty string, never a default.
var schemeWiringCases = []struct {
	name   string
	scheme string
}{
	{name: "declared scheme is forwarded verbatim", scheme: "PIX"},
	{name: "no scheme is forwarded as empty", scheme: ""},
}

// TestCreateTransactionV2_SchemeWiring_ReachesReserveAndRow drives the real v2
// create through its engine harness and proves the scheme flows to both sinks:
// the tracer reserve request (as transactionType) and the persisted row, read
// through the completion plan the completer receives and the write set built
// from it.
func TestCreateTransactionV2_SchemeWiring_ReachesReserveAndRow(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	for _, tc := range schemeWiringCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			redisRepo := txRedis.NewMockRedisRepository(ctrl)
			idempotencySet := make(chan struct{})
			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil).Times(1)
			redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
				func(context.Context, string, string, time.Duration) error {
					close(idempotencySet)
					return nil
				},
			).Times(1)

			organizationID := uuid.MustParse("0199a600-0000-7000-8000-000000000001")
			ledgerID := uuid.MustParse("0199a600-0000-7000-8000-000000000002")
			settings := mmodel.LedgerSettings{}
			settings.Tracer = mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed}
			reader := &createEngineReader{settings: settings, balances: []*mmodel.Balance{
				translationBalance(organizationID, ledgerID, "0199a600-0000-7000-8000-000000000003", "@source", constant.DefaultBalanceKey),
				translationBalance(organizationID, ledgerID, "0199a600-0000-7000-8000-000000000004", "@target", constant.DefaultBalanceKey),
			}}
			executor := &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}}
			finalizer := &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}}
			reservationID := uuid.MustParse("0199a600-0000-7000-8000-000000000005")
			reserver := &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}}
			uc := &UseCase{
				TransactionRedisRepo: redisRepo, TransactionReader: reader,
				Engine: executor, AppliedTransactionCompleter: finalizer, TracerReserver: reserver,
				EngineRecoveryAcknowledger: &recordingEngineRecoveryAcknowledger{},
			}

			input := createEngineTransaction(time.Date(2026, time.October, 5, 10, 0, 0, 0, time.UTC))
			input.Scheme = tc.scheme

			got, replayed, err := uc.CreateTransactionV2(
				tmcore.ContextWithTenantID(context.Background(), "tenant-scheme"),
				CreateTransactionV2Input{
					OrganizationID: organizationID, LedgerID: ledgerID,
					Transaction: input, TransactionStatus: constant.CREATED, IdempotencyTTL: time.Minute,
				},
			)
			require.NoError(t, err)
			assert.False(t, replayed)
			require.NotNil(t, got)

			requests := reserver.reserveRequests()
			require.Len(t, requests, 1, "an enforcing v2 create reserves exactly once")
			assert.Equal(t, tc.scheme, requests[0].TransactionType, "the reserve carries the scheme as transactionType")
			assert.False(t, requests[0].Revert)

			assert.Equal(t, tc.scheme, got.Scheme, "the returned transaction carries the scheme")

			require.Len(t, finalizer.envelopes, 1)
			payload := mustCreateEnginePayload(t, finalizer.envelopes[0])
			assert.Equal(t, tc.scheme, payload.TransactionInput.Scheme, "the frozen completion plan carries the scheme")

			writeSet, err := BuildTransactionWriteSet(*payload, finalizer.envelopes[0].Result)
			require.NoError(t, err)
			assert.Equal(t, tc.scheme, writeSet.Transaction.Scheme, "the persisted row carries the scheme")

			select {
			case <-idempotencySet:
			case <-time.After(time.Second):
				t.Fatal("durable v2 create did not populate the idempotency value")
			}
		})
	}
}

// TestRevertTransactionV2_SchemeWiring_InheritsOriginOnReserve drives the real v2
// revert through its engine harness with an origin row that carries a scheme and
// proves the reversal reserves under the ORIGIN's scheme, marked as a revert, and
// persists it on the reversal row.
func TestRevertTransactionV2_SchemeWiring_InheritsOriginOnReserve(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	for _, tc := range schemeWiringCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			redisRepo := txRedis.NewMockRedisRepository(ctrl)
			idempotencySet := make(chan struct{})
			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Duration(300)).Return(true, nil).Times(1)
			redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(300)).DoAndReturn(
				func(context.Context, string, string, time.Duration) error {
					close(idempotencySet)
					return nil
				},
			).Times(1)

			organizationID := uuid.MustParse("0199a600-0000-7000-8000-000000000011")
			ledgerID := uuid.MustParse("0199a600-0000-7000-8000-000000000012")
			originID := uuid.MustParse("0199a600-0000-7000-8000-000000000013")
			origin := revertEngineOrigin(organizationID, ledgerID, originID)
			origin.Scheme = tc.scheme
			settings := mmodel.LedgerSettings{}
			settings.Tracer = mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureClosed}
			reader := &revertEngineReader{
				revertReader: &revertReader{origin: origin, versionReader: versionReader{settings: settings}},
				balances: []*mmodel.Balance{
					revertEngineBalance(organizationID, ledgerID, "0199a600-0000-7000-8000-000000000014", "@payee", 50, 7),
					revertEngineBalance(organizationID, ledgerID, "0199a600-0000-7000-8000-000000000015", "@payer", 20, 3),
				},
			}
			finalizer := &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}}
			reservationID := uuid.MustParse("0199a600-0000-7000-8000-000000000016")
			reserver := &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}}
			uc := &UseCase{
				TransactionRedisRepo: redisRepo, TransactionReader: reader,
				Engine: &revertLiteralEngine{t: t}, AppliedTransactionCompleter: finalizer, TracerReserver: reserver,
				EngineRecoveryAcknowledger: &recordingEngineRecoveryAcknowledger{},
			}

			got, replayed, err := uc.RevertTransactionV2(
				tmcore.ContextWithTenantID(context.Background(), "tenant-scheme-revert"),
				RevertTransactionInput{OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: originID},
			)
			require.NoError(t, err)
			assert.False(t, replayed)
			require.NotNil(t, got)

			requests := reserver.reserveRequests()
			require.Len(t, requests, 1, "an enforcing v2 revert reserves capacity of its own")
			assert.Equal(t, tc.scheme, requests[0].TransactionType, "the reversal reserves under the origin's scheme")
			assert.True(t, requests[0].Revert, "the reversal's reserve is marked as a revert")

			assert.Equal(t, tc.scheme, got.Scheme, "the reversal row inherits the origin's scheme")

			require.Len(t, finalizer.envelopes, 1)
			payload := mustCreateEnginePayload(t, finalizer.envelopes[0])
			assert.Equal(t, constant.ActionRevert, payload.Action)
			assert.Equal(t, tc.scheme, payload.TransactionInput.Scheme, "the reversal's completion plan carries the origin's scheme")

			select {
			case <-idempotencySet:
			case <-time.After(time.Second):
				t.Fatal("durable v2 revert did not populate the idempotency value")
			}
		})
	}
}

// TestCreateAtomicTransactionBatchV2_SchemeWiring_ForwardsPerItemInOrder drives
// the real atomic batch v2 with one item declaring a scheme and one declaring
// none, and proves each reserve carries its own item's scheme in request order,
// as does each item's result row and frozen completion plan.
func TestCreateAtomicTransactionBatchV2_SchemeWiring_ForwardsPerItemInOrder(t *testing.T) {
	t.Parallel()

	engine := &applyingAtomicTransactionBatchEngine{t: t}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, transactionIDs, _ := atomicTransactionBatchExecutionFixture(t, &atomicTransactionBatchClaimRepositoryFake{}, engine, reserver)

	wantSchemes := []string{"PIX", ""}
	for index := range input.Transactions {
		input.Transactions[index].Transaction.Scheme = wantSchemes[index]
	}

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Transactions, 2)

	requests, _, _ := reserver.snapshot()
	require.Len(t, requests, 2, "each item reserves once")
	assert.Equal(t, transactionIDs, atomicTransactionBatchTracerRequestIDs(requests), "reserves follow request order")
	assert.Equal(t, wantSchemes, []string{requests[0].TransactionType, requests[1].TransactionType},
		"each reserve carries its own item's scheme, never a neighbour's")

	assert.Equal(t, wantSchemes, []string{result.Transactions[0].Scheme, result.Transactions[1].Scheme},
		"each result row carries its own item's scheme")

	require.Len(t, engine.executions, 1)
	plans := engine.executions[0].CompletionPlans
	require.Len(t, plans, 2)

	gotPlanSchemes := make([]string, 0, len(plans))

	for _, plan := range plans {
		payload, decodeErr := DecodeTransactionCompletionPlan(plan.Payload)
		require.NoError(t, decodeErr)

		gotPlanSchemes = append(gotPlanSchemes, payload.TransactionInput.Scheme)
	}

	assert.Equal(t, wantSchemes, gotPlanSchemes, "each frozen completion plan carries its own item's scheme")
}

// TestDecomposeCrossLedgerTransaction_SchemeWiring_CopiesOntoEveryPart proves a
// cross-ledger decomposition carries the request's scheme onto every ledger part,
// so each part reserves and persists under the scheme the client declared once.
func TestDecomposeCrossLedgerTransaction_SchemeWiring_CopiesOntoEveryPart(t *testing.T) {
	t.Parallel()

	for _, tc := range schemeWiringCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			organizationID := uuid.MustParse("0199a600-0000-7000-8000-000000000021")
			ledgerA := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199a600-0000-7000-8000-000000000022")}
			ledgerB := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199a600-0000-7000-8000-000000000023")}
			ledgerC := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: uuid.MustParse("0199a600-0000-7000-8000-000000000024")}

			tx := crossLedgerTestTransaction("100",
				[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "100", true)},
				[]mtransaction.FromTo{crossLedgerAmountLeg("@credit-b", "40", false), crossLedgerAmountLeg("@credit-c", "60", false)})
			tx.Scheme = tc.scheme

			parts, err := decomposeCrossLedgerTransaction(tx, crossLedgerTransactionScopes{
				from: []atomicTransactionBatchLedgerRef{ledgerA},
				to:   []atomicTransactionBatchLedgerRef{ledgerB, ledgerC},
			})
			require.NoError(t, err)
			require.Len(t, parts, 3, "one debit ledger and two credit ledgers decompose into three parts")

			for i := range parts {
				assert.Equal(t, tc.scheme, parts[i].transaction.Scheme, "part %d must carry the request's scheme", i)
			}
		})
	}
}

// TestBuildTransactionEventSource_SchemeWiring_MapsEntityScheme proves the
// streaming source built from a persisted row carries the row's scheme, so the
// lifecycle payload publishes what the ledger stored.
func TestBuildTransactionEventSource_SchemeWiring_MapsEntityScheme(t *testing.T) {
	t.Parallel()

	for _, tc := range schemeWiringCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tran := transactionLifecycleFixture(nil, constant.APPROVED)
			tran.Scheme = tc.scheme

			source, err := buildTransactionEventSource(tran)
			require.NoError(t, err)

			assert.Equal(t, tran.ID, source.ID)
			assert.Equal(t, tc.scheme, source.Scheme, "the event source carries the row's scheme")
		})
	}
}
