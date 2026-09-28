// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package in

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	redistransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// captureFailingBatchRepository fails the next captures, standing in for a
// request that dies after the engine applied it and before it froze its
// response.
type captureFailingBatchRepository struct {
	command.AtomicTransactionBatchIdempotencyRepository

	failures atomic.Int32
}

func (repository *captureFailingBatchRepository) CaptureAtomicTransactionBatchInitialResponse(
	ctx context.Context,
	organizationID, ledgerID, executionID uuid.UUID,
	ownerToken string,
	transactionID uuid.UUID,
	response json.RawMessage,
) (*redistransaction.AtomicTransactionBatchInitialResponseCaptureResult, error) {
	if repository.failures.Add(-1) >= 0 {
		return nil, errors.New("request interrupted before capture")
	}

	return repository.AtomicTransactionBatchIdempotencyRepository.CaptureAtomicTransactionBatchInitialResponse(
		ctx, organizationID, ledgerID, executionID, ownerToken, transactionID, response,
	)
}

// The fixture runs without a tenant in context, so the engine writes its
// receipts with an empty tenant: the single-tenant shape recovery must accept.
func TestIntegration_AtomicTransactionBatchV2_SingleTenantRecoverySealsTheBatch(t *testing.T) {
	fixture := setupAtomicBatchHTTPIntegrationFixture(t)

	t.Run("batch create", func(t *testing.T) {
		ledgerID := fixture.newLedger(t)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerID, "@st-batch-source", "@st-batch-destination", 100)
		transactions := []CreateTransactionV2Request{
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "st batch first", "@st-batch-source", "@st-batch-destination", 40),
			atomicBatchTransfer(fixture.infra.orgID, ledgerID, "st batch second", "@st-batch-source", "@st-batch-destination", 60),
		}
		body := marshalAtomicBatchHTTPBody(t, transactions)

		counted := interruptNextAtomicBatchCapture(t, fixture, func() *http.Response {
			return postTransaction(t, fixture.app, v2CreateURL("batch"), body, "st-recovered-batch")
		})

		executionID := pendingRecoveryExecutionID(t, fixture, []uuid.UUID{ledgerID})
		recoverAtomicBatchExecution(t, fixture, executionID)

		record := atomicBatchRecordOfExecution(t, fixture, ledgerID, executionID)
		require.Equal(t, redistransaction.AtomicTransactionBatchStateComplete, record.State)
		sealed := decodeSealedBatchMembers(t, record.Response)
		require.Len(t, sealed, 2)
		require.Equal(t, "st batch first", sealed[0].Description)
		require.Equal(t, "st batch second", sealed[1].Description)
		for _, member := range sealed {
			require.Equal(t, constant.CREATED, member.Status.Code)
		}

		replay := postTransaction(t, fixture.app, v2CreateURL("batch"), body, "st-recovered-batch")
		require.Equal(t, "true", replay.Header.Get("X-Idempotency-Replayed"))
		replayed := decodeAtomicBatchResponse(t, replay, http.StatusCreated)
		require.Len(t, replayed.Transactions, 2)
		for index, member := range replayed.Transactions {
			require.Equal(t, sealed[index].ID, member.ID)
			require.Equal(t, constant.CREATED, member.Status.Code)
		}
		require.Equal(t, int64(1), counted.calls.Load(), "neither recovery nor replay may execute the batch again")
	})

	t.Run("cross-ledger direct", func(t *testing.T) {
		// The batch record lives in the coordination scope, the smallest ledger,
		// while the engine receipt lives in the debit ledger. Making the debit
		// ledger the larger one keeps the two scopes apart on every run.
		ledgerA, ledgerB := fixture.newLedger(t), fixture.newLedger(t)
		if ledgerA.String() < ledgerB.String() {
			ledgerA, ledgerB = ledgerB, ledgerA
		}
		fixture.setCrossLedgerEnabled(t, ledgerA, true)
		fixture.setCrossLedgerEnabled(t, ledgerB, true)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerA, "@st-direct-source", "@external/USD", 100)
		seedTransfer(t, fixture.infra.pgContainer.DB, fixture.infra.orgID, ledgerB, "@external/USD", "@st-direct-destination", 100)
		request := atomicBatchTransfer(fixture.infra.orgID, ledgerA, "st recovered direct", "@st-direct-source", "@st-direct-destination", 100)
		request.Credits[0].LedgerID = ledgerB.String()
		raw, err := json.Marshal(request)
		require.NoError(t, err)

		counted := interruptNextAtomicBatchCapture(t, fixture, func() *http.Response {
			return postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), "st-recovered-direct")
		})

		executionID := pendingRecoveryExecutionID(t, fixture, []uuid.UUID{ledgerA, ledgerB})
		recoverAtomicBatchExecution(t, fixture, executionID)

		record := atomicBatchRecordOfExecution(t, fixture, ledgerB, executionID)
		require.Equal(t, redistransaction.AtomicTransactionBatchStateComplete, record.State)
		require.NotNil(t, record.ReceiptLedgerID, "a cross-ledger record points at the receipt scope")
		require.Equal(t, ledgerA, *record.ReceiptLedgerID)
		sealed := decodeSealedBatchMembers(t, record.Response)
		require.Len(t, sealed, 2)
		require.ElementsMatch(t, []string{ledgerA.String(), ledgerB.String()},
			[]string{sealed[0].LedgerID, sealed[1].LedgerID})
		for _, member := range sealed {
			require.Equal(t, constant.CREATED, member.Status.Code)
		}

		replay := postTransaction(t, fixture.app, v2CreateURL("direct"), string(raw), "st-recovered-direct")
		replayBody := drainBody(t, replay)
		require.Equal(t, http.StatusCreated, replay.StatusCode, "body: %s", string(replayBody))
		require.Equal(t, "true", replay.Header.Get("X-Idempotency-Replayed"))
		var replayed CreateTransactionV2Response
		require.NoError(t, json.Unmarshal(replayBody, &replayed))
		require.Len(t, replayed.Transactions, 2)
		for index, member := range replayed.Transactions {
			require.Equal(t, sealed[index].ID, member.ID)
			require.Equal(t, constant.CREATED, member.Status.Code)
		}
		require.Equal(t, int64(1), counted.calls.Load(), "neither recovery nor replay may execute the group again")
	})
}

// interruptNextAtomicBatchCapture sends one request whose engine execution is
// applied but whose first response capture fails, then restores the real
// repository. The engine stays wrapped until the test ends so callers can prove
// no later step executes it again.
func interruptNextAtomicBatchCapture(
	t *testing.T,
	fixture *atomicBatchHTTPIntegrationFixture,
	send func() *http.Response,
) *countingAtomicBatchEngine {
	t.Helper()

	original := fixture.infra.handler.Command.AtomicTransactionBatchIdempotencyRepo
	originalEngine := fixture.infra.handler.Command.Engine
	// The fixture is shared by sibling subtests, so a failure below must not
	// leave them the interrupting wrapper.
	t.Cleanup(func() {
		fixture.infra.handler.Command.AtomicTransactionBatchIdempotencyRepo = original
		fixture.infra.handler.Command.Engine = originalEngine
	})

	interrupted := &captureFailingBatchRepository{AtomicTransactionBatchIdempotencyRepository: original}
	interrupted.failures.Store(1)
	counted := &countingAtomicBatchEngine{delegate: fixture.engine}
	fixture.infra.handler.Command.Engine = counted
	fixture.infra.handler.Command.AtomicTransactionBatchIdempotencyRepo = interrupted

	response := send()
	body := drainBody(t, response)

	// The recovery finalizer type-asserts a repository method the embedding
	// wrapper does not expose, so the real repository must be back first.
	fixture.infra.handler.Command.AtomicTransactionBatchIdempotencyRepo = original

	require.GreaterOrEqual(t, response.StatusCode, http.StatusInternalServerError, "body: %s", string(body))
	require.Equal(t, int64(1), counted.calls.Load(), "the interrupted request applied its execution")

	return counted
}

// pendingRecoveryExecutionID returns the one execution the interrupted request
// left in the engine recovery queue for the given ledgers.
func pendingRecoveryExecutionID(
	t *testing.T,
	fixture *atomicBatchHTTPIntegrationFixture,
	ledgerIDs []uuid.UUID,
) uuid.UUID {
	t.Helper()

	all, err := fixture.infra.redisContainer.Client.HGetAll(context.Background(), cachepolicy.EngineRecoverQueue).Result()
	require.NoError(t, err)

	executions := make(map[uuid.UUID]struct{})
	for _, raw := range all {
		envelope, err := command.DecodeTransactionWriteBehindEnvelope([]byte(raw))
		require.NoError(t, err)
		if slices.Contains(ledgerIDs, envelope.Record.LedgerID) {
			executions[envelope.Record.ExecutionID] = struct{}{}
		}
	}

	require.Len(t, executions, 1, "the interrupted request must leave exactly its execution for the consumer")

	for executionID := range executions {
		return executionID
	}

	return uuid.Nil
}

// atomicBatchRecordOfExecution reads the batch record of one execution from
// the ledger scope expected to hold it.
func atomicBatchRecordOfExecution(
	t *testing.T,
	fixture *atomicBatchHTTPIntegrationFixture,
	ledgerID, executionID uuid.UUID,
) redistransaction.AtomicTransactionBatchIdempotencyRecord {
	t.Helper()

	repository := fixture.infra.redisRepo.(*redistransaction.RedisConsumerRepository)
	lookup, err := repository.GetAtomicTransactionBatchByExecutionID(
		context.Background(), fixture.infra.orgID, ledgerID, executionID,
	)
	require.NoError(t, err)
	require.NotNil(t, lookup, "the execution has no batch record in ledger %s", ledgerID)

	return lookup.Record
}

type sealedBatchMember struct {
	ID          string `json:"id"`
	LedgerID    string `json:"ledgerId"`
	Description string `json:"description"`
	Status      struct {
		Code string `json:"code"`
	} `json:"status"`
}

func decodeSealedBatchMembers(t *testing.T, response json.RawMessage) []sealedBatchMember {
	t.Helper()

	var sealed struct {
		Transactions []sealedBatchMember `json:"transactions"`
	}
	require.NoError(t, json.Unmarshal(response, &sealed), "sealed response: %s", string(response))

	return sealed.Transactions
}

// recoverAtomicBatchExecution runs the engine recovery consumer's completion,
// batch finalization, and protected acknowledgment for every pending member
// of one execution, retrying members whose finalization waits on a sibling.
// The runner lives in bootstrap, which imports this package, so the sequence
// is replicated here.
func recoverAtomicBatchExecution(t *testing.T, fixture *atomicBatchHTTPIntegrationFixture, executionID uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	repository := fixture.infra.redisRepo.(*redistransaction.RedisConsumerRepository)
	completedAt := time.Date(2026, time.September, 16, 13, 0, 0, 0, time.UTC)

	pendingFields := func() map[string]string {
		all, err := fixture.infra.redisContainer.Client.HGetAll(ctx, cachepolicy.EngineRecoverQueue).Result()
		require.NoError(t, err)

		pending := make(map[string]string)
		for field, raw := range all {
			if strings.HasSuffix(field, ":"+executionID.String()) {
				pending[field] = raw
			}
		}

		return pending
	}

	pending := pendingFields()
	require.NotEmpty(t, pending)

	for pass := 0; pass <= len(pending) && len(pending) > 0; pass++ {
		for field, raw := range pending {
			envelope, err := command.DecodeTransactionWriteBehindEnvelope([]byte(raw))
			require.NoError(t, err)
			completion, err := fixture.infra.handler.Command.AppliedTransactionCompleter.Complete(ctx, &envelope.Record)
			require.NoError(t, err)
			prepared, err := fixture.infra.handler.Command.PrepareAtomicTransactionBatchRecoveryFinalization(ctx, &envelope.Record, completion)
			require.NoError(t, err)
			require.NotNil(t, prepared, "an atomic batch member recovers through the batch finalizer")

			status, err := repository.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
				ctx,
				redistransaction.RecoveryQueueSourceEngineRecover,
				envelope.Record.OrganizationID,
				envelope.Record.LedgerID,
				field,
				raw,
				true,
				completedAt,
				prepared.ReceiptToken,
				prepared.Transactions,
			)
			require.NoError(t, err)
			require.Contains(t, []int64{redistransaction.RecoveryAckDeleted, redistransaction.RecoveryAckFinalizationRequired}, status)
		}

		pending = pendingFields()
	}

	require.Empty(t, pending, "recovery must acknowledge every member of the execution")
}
