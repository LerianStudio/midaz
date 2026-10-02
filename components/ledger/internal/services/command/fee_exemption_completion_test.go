// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestFeeExemptionMetadataCompletes completes a plan carrying the feeExemption JSON string the
// fee engine writes (proven in pkg/fee: services/fees imports this package, so it cannot run
// here). The value is flat, so the transaction is persisted with its metadata, on replay too.
func TestFeeExemptionMetadataCompletes(t *testing.T) {
	raw := `{"exempt":true,"reason":"all_source_accounts_exempt","message":"All source accounts are exempt from fees."}`

	var err error

	payload, result := recoveryContractFixture(t)
	payload.TransactionInput.Metadata = map[string]any{"feeExemption": raw}
	payload.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)

	envelope := recoveryContractEnvelope(t, payload, result)
	ctx, _ := finalizationFixture(t)
	finalizer, store, metadata, _ := finalizationDependencies()

	require.NoError(t, completionError(finalizer.Complete(ctx, &envelope)))
	require.NoError(t, completionError(finalizer.Complete(ctx, &envelope)), "a recovery replay completes too")

	require.Len(t, store.records, 2)
	stored := metadata.data[constant.EntityTransaction+":"+payload.TransactionID.String()]
	require.NotNil(t, stored)
	assert.Equal(t, raw, stored.Data["feeExemption"])
}

// TestNestedTransactionMetadataStopsBeforeEngine keeps a ledger-written nested
// metadata value from reaching the engine: the pre-execution plan check refuses
// it, so no balance moves without a record the completer can persist.
func TestNestedTransactionMetadataStopsBeforeEngine(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil).Times(1)
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	organizationID := uuid.MustParse("b1111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("b2222222-2222-4222-8222-222222222222")
	reader := &createEngineReader{balances: []*mmodel.Balance{
		translationBalance(organizationID, ledgerID, "b4444444-4444-4444-8444-444444444444", "@source", constant.DefaultBalanceKey),
		translationBalance(organizationID, ledgerID, "b5555555-5555-4555-8555-555555555555", "@target", constant.DefaultBalanceKey),
	}}
	executor := &createEngineErrorExecutor{err: errors.New("must not execute")}
	feeApplier := &fakeFeeApplier{mutate: func(cf *model.FeeCalculate) {
		cf.Transaction.Metadata = map[string]any{"feeExemption": map[string]any{"exempt": true}}
	}}
	uc := &UseCase{
		TransactionRedisRepo: redisRepo, TransactionReader: reader, FeeApplier: feeApplier,
		Engine: executor, AppliedTransactionCompleter: &createAppliedTransactionCompleter{},
	}

	_, _, err := uc.CreateTransactionV2(context.Background(), CreateTransactionV2Input{
		OrganizationID: organizationID, LedgerID: ledgerID,
		Transaction:       createEngineTransaction(time.Date(2026, time.September, 28, 13, 0, 0, 0, time.UTC)),
		TransactionStatus: constant.CREATED, IdempotencyTTL: time.Minute,
	})
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)
	assert.Equal(t, 1, feeApplier.calls)
	assert.Empty(t, executor.requests, "the engine is never called")
}

// TestFrozenNestedMetadataPlanDecodes keeps a plan frozen with nested transaction
// metadata readable by evidence, recovery and account closing, while encode refuses it.
func TestFrozenNestedMetadataPlanDecodes(t *testing.T) {
	payload, _ := recoveryContractFixture(t)
	payload.TransactionInput.Metadata = map[string]any{"feeExemption": map[string]any{"exempt": true}}

	_, err := EncodeTransactionCompletionPlan(payload)
	require.ErrorIs(t, err, ErrInvalidTransactionCompletionRecord)

	frozen, err := json.Marshal(payload)
	require.NoError(t, err)
	decoded, err := DecodeTransactionCompletionPlan(frozen)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"exempt": true}, decoded.TransactionInput.Metadata["feeExemption"])
}

// TestRevertFlattensLegacyNestedFeeExemption reverts an origin whose feeExemption Mongo
// returns as a document: the reversal plan carries it as the JSON string.
func TestRevertFlattensLegacyNestedFeeExemption(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	idempotencySet := make(chan struct{})
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Duration(300)).Return(true, nil).Times(1)
	redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(300)).DoAndReturn(
		func(context.Context, string, string, time.Duration) error { close(idempotencySet); return nil },
	).Times(1)

	organizationID := uuid.MustParse("c1111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("c2222222-2222-4222-8222-222222222222")
	originID := uuid.MustParse("c3333333-3333-4333-8333-333333333333")
	origin := revertEngineOrigin(organizationID, ledgerID, originID)
	origin.Metadata = map[string]any{"feeExemption": bson.D{{Key: "exempt", Value: true}, {Key: "reason", Value: "all_source_accounts_exempt"}}}
	reader := &revertEngineReader{revertReader: &revertReader{origin: origin}, balances: []*mmodel.Balance{
		revertEngineBalance(organizationID, ledgerID, "c4444444-4444-4444-8444-444444444444", "@payee", 50, 7),
		revertEngineBalance(organizationID, ledgerID, "c5555555-5555-4555-8555-555555555555", "@payer", 20, 3),
	}}
	finalizer := &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}}
	uc := &UseCase{
		TransactionRedisRepo: redisRepo, TransactionReader: reader, Engine: &revertLiteralEngine{t: t},
		AppliedTransactionCompleter: finalizer, EngineRecoveryAcknowledger: &recordingEngineRecoveryAcknowledger{},
	}

	_, _, err := uc.RevertTransactionV1(context.Background(), RevertTransactionInput{OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: originID})
	require.NoError(t, err)
	require.Len(t, finalizer.envelopes, 1)
	payload := mustCreateEnginePayload(t, finalizer.envelopes[0])
	assert.Equal(t, `{"exempt":true,"reason":"all_source_accounts_exempt"}`, payload.TransactionInput.Metadata["feeExemption"])
	assert.IsType(t, bson.D{}, origin.Metadata["feeExemption"], "the origin keeps its own value")
	select {
	case <-idempotencySet:
	case <-time.After(time.Second):
		t.Fatal("the revert did not populate the idempotency value")
	}
}

// TestPendingCommitFlattensLegacyNestedFeeExemption commits a PENDING whose stored body
// holds feeExemption as an object: the commit plan carries it as the JSON string.
func TestPendingCommitFlattensLegacyNestedFeeExemption(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	uc, reader, _, finalizer, in := newTransitionEngineUseCase(t, constant.APPROVED)
	reader.persisted.Body.Metadata = map[string]any{"feeExemption": map[string]any{"exempt": true, "reason": "all_source_accounts_exempt"}}

	_, err := uc.CommitTransactionV2(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, finalizer.envelopes, 1)
	payload := mustCreateEnginePayload(t, finalizer.envelopes[0])
	assert.Equal(t, `{"exempt":true,"reason":"all_source_accounts_exempt"}`, payload.TransactionInput.Metadata["feeExemption"])
}

// TestLegacyFeeExemptionDocumentCompletesPendingCommit completes a plan carrying the
// JSON string over a stored document Mongo returns with the legacy object: the stored
// document is kept and the record completes.
func TestLegacyFeeExemptionDocumentCompletesPendingCommit(t *testing.T) {
	payload, result := recoveryContractFixture(t)
	payload.TransactionInput.Metadata = map[string]any{"feeExemption": `{"exempt":true,"reason":"all_source_accounts_exempt"}`}
	var err error
	payload.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)

	envelope := recoveryContractEnvelope(t, payload, result)
	ctx, _ := finalizationFixture(t)
	finalizer, store, metadata, _ := finalizationDependencies()
	key := constant.EntityTransaction + ":" + payload.TransactionID.String()
	legacy := mongodb.JSON{"feeExemption": bson.D{{Key: "exempt", Value: true}, {Key: "reason", Value: "all_source_accounts_exempt"}}}
	metadata.data[key] = &mongodb.Metadata{EntityID: payload.TransactionID.String(), EntityName: constant.EntityTransaction, Data: legacy}

	require.NoError(t, completionError(finalizer.Complete(ctx, &envelope)))
	require.Len(t, store.records, 1)
	assert.Equal(t, legacy, metadata.data[key].Data)
}

// TestNestedFeeExemptionPlanCompletes completes a plan frozen before the string
// contract, whose feeExemption is an object: the Mongo record holds the string form.
func TestNestedFeeExemptionPlanCompletes(t *testing.T) {
	payload, result := recoveryContractFixture(t)
	payload.TransactionInput.Metadata = map[string]any{"feeExemption": map[string]any{"exempt": true, "reason": "all_source_accounts_exempt"}}
	var err error
	payload.IntentFingerprint, err = ComputeEngineIntentFingerprint(recoveryContractIntent(payload))
	require.NoError(t, err)
	frozen, err := json.Marshal(payload)
	require.NoError(t, err)

	envelope := TransactionCompletionRecord{
		FormatVersion: TransactionCompletionFormatVersion, TenantID: payload.TenantID, OrganizationID: payload.OrganizationID,
		LedgerID: payload.LedgerID, TransactionID: payload.TransactionID, ExecutionID: payload.ExecutionID, IntentFingerprint: payload.IntentFingerprint,
		Payload: string(frozen), Result: result,
	}
	ctx, _ := finalizationFixture(t)
	finalizer, store, metadata, _ := finalizationDependencies()

	require.NoError(t, completionError(finalizer.Complete(ctx, &envelope)))
	require.Len(t, store.records, 1)
	stored := metadata.data[constant.EntityTransaction+":"+payload.TransactionID.String()]
	require.NotNil(t, stored)
	assert.Equal(t, mongodb.JSON{"feeExemption": `{"exempt":true,"reason":"all_source_accounts_exempt"}`}, stored.Data)
}
