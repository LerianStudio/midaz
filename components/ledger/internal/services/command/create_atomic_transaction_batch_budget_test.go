// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	postgresTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

type atomicTransactionBatchClaimRepositoryFake struct {
	claims              int
	claimOrganizationID uuid.UUID
	claimLedgerID       uuid.UUID
	transitions         int
	handoffs            int
	finalizations       int
	captures            int
	capturedResponses   map[uuid.UUID]json.RawMessage
	captureOrder        *[]string
	aborts              int
	deletes             int
	effectiveKey        string
	claim               txRedis.AtomicTransactionBatchIdempotencyRecord
	transition          txRedis.AtomicTransactionBatchIdempotencyRecord
	handoff             txRedis.AtomicTransactionBatchIdempotencyRecord
	deleteOwner         string
	transitionErr       error
	handoffErr          error
	abortErr            error
	finalizationErr     error
	abortOutcome        txRedis.AtomicTransactionBatchRefusalAbortOutcome
	abortExecutionID    uuid.UUID
	abortTransactionIDs []uuid.UUID
}

func (repository *atomicTransactionBatchClaimRepositoryFake) CaptureAtomicTransactionBatchInitialResponse(
	_ context.Context,
	_, _, _ uuid.UUID,
	_ string,
	transactionID uuid.UUID,
	response json.RawMessage,
) (*txRedis.AtomicTransactionBatchInitialResponseCaptureResult, error) {
	repository.captures++
	if repository.capturedResponses == nil {
		repository.capturedResponses = make(map[uuid.UUID]json.RawMessage)
	}
	repository.capturedResponses[transactionID] = append(json.RawMessage(nil), response...)
	if repository.captureOrder != nil {
		*repository.captureOrder = append(*repository.captureOrder, "capture:"+transactionID.String())
	}

	return &txRedis.AtomicTransactionBatchInitialResponseCaptureResult{
		Outcome: txRedis.AtomicTransactionBatchInitialResponseCaptured,
	}, nil
}

func (repository *atomicTransactionBatchClaimRepositoryFake) FinalizeAtomicTransactionBatch(
	_ context.Context,
	_, _, _ uuid.UUID,
	_ string,
	_ map[uuid.UUID]json.RawMessage,
	_ time.Duration,
) (*txRedis.AtomicTransactionBatchFinalizationResult, error) {
	repository.finalizations++
	if repository.finalizationErr != nil {
		return nil, repository.finalizationErr
	}

	return &txRedis.AtomicTransactionBatchFinalizationResult{
		Outcome: txRedis.AtomicTransactionBatchFinalized,
	}, nil
}

func (repository *atomicTransactionBatchClaimRepositoryFake) ClaimAtomicTransactionBatch(
	_ context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey string,
	claim txRedis.AtomicTransactionBatchIdempotencyRecord,
) (*txRedis.AtomicTransactionBatchClaimResult, error) {
	repository.claims++
	repository.claimOrganizationID = organizationID
	repository.claimLedgerID = ledgerID
	repository.effectiveKey = effectiveKey
	repository.claim = claim

	return &txRedis.AtomicTransactionBatchClaimResult{
		Outcome: txRedis.AtomicTransactionBatchClaimed,
		Record:  claim,
	}, nil
}

func (repository *atomicTransactionBatchClaimRepositoryFake) TransitionAtomicTransactionBatch(
	_ context.Context,
	_, _ uuid.UUID,
	_, _ string,
	_ txRedis.AtomicTransactionBatchIdempotencyState,
	next txRedis.AtomicTransactionBatchIdempotencyRecord,
	_ time.Duration,
) (*txRedis.AtomicTransactionBatchTransitionResult, error) {
	repository.transitions++
	repository.transition = next
	if repository.transitionErr != nil {
		return nil, repository.transitionErr
	}

	return &txRedis.AtomicTransactionBatchTransitionResult{
		Outcome: txRedis.AtomicTransactionBatchTransitionUpdated,
		Record:  next,
	}, nil
}

func (repository *atomicTransactionBatchClaimRepositoryFake) HandoffAtomicTransactionBatchExecution(
	_ context.Context,
	_, _ uuid.UUID,
	_, _ string,
	next txRedis.AtomicTransactionBatchIdempotencyRecord,
) (*txRedis.AtomicTransactionBatchTransitionResult, error) {
	repository.handoffs++
	repository.handoff = next
	if repository.handoffErr != nil {
		return nil, repository.handoffErr
	}

	return &txRedis.AtomicTransactionBatchTransitionResult{
		Outcome: txRedis.AtomicTransactionBatchTransitionUpdated,
		Record:  next,
	}, nil
}

func (repository *atomicTransactionBatchClaimRepositoryFake) AbortAtomicTransactionBatchConfirmedRefusal(
	_ context.Context,
	_, _ uuid.UUID,
	_, _ string,
	executionID uuid.UUID,
	transactionIDs []uuid.UUID,
) (*txRedis.AtomicTransactionBatchRefusalAbortResult, error) {
	repository.aborts++
	repository.abortExecutionID = executionID
	repository.abortTransactionIDs = append([]uuid.UUID(nil), transactionIDs...)
	if repository.abortErr != nil {
		return nil, repository.abortErr
	}
	outcome := repository.abortOutcome
	if outcome == "" {
		outcome = txRedis.AtomicTransactionBatchRefusalDeleted
	}

	return &txRedis.AtomicTransactionBatchRefusalAbortResult{
		Outcome: outcome,
		Record:  repository.handoff,
	}, nil
}

func (repository *atomicTransactionBatchClaimRepositoryFake) DeleteAtomicTransactionBatchPrePublication(
	_ context.Context,
	_, _ uuid.UUID,
	_ string,
	ownerToken string,
) (*txRedis.AtomicTransactionBatchDeleteResult, error) {
	repository.deletes++
	repository.deleteOwner = ownerToken

	return &txRedis.AtomicTransactionBatchDeleteResult{
		Outcome: txRedis.AtomicTransactionBatchDeleted,
		Record:  repository.claim,
	}, nil
}

func TestValidateAtomicTransactionBatchCumulativeBudget_ExactBoundaries(t *testing.T) {
	limits := defaultAtomicTransactionBatchBudgetLimits
	tests := []struct {
		dimension string
		limit     int
	}{
		{atomicTransactionBatchBudgetExpandedPostings, limits.expandedPostings},
		{atomicTransactionBatchBudgetExecutionBalances, limits.executionBalances},
		{atomicTransactionBatchBudgetCompletionPlanBytes, limits.completionPlanBytes},
		{atomicTransactionBatchBudgetAccountingRequestBytes, limits.accountingRequestBytes},
		{atomicTransactionBatchBudgetPreparedResponseBytes, limits.preparedResponseBytes},
		{atomicTransactionBatchBudgetRecoveryBytes, limits.recoveryBytes},
		{atomicTransactionBatchBudgetCachedResponseBytes, limits.cachedResponseBytes},
	}

	for _, test := range tests {
		t.Run(test.dimension, func(t *testing.T) {
			require.NoError(t, validateAtomicTransactionBatchCumulativeBudget(
				test.dimension,
				[]int{test.limit - 1, test.limit},
				test.limit,
			))

			err := validateAtomicTransactionBatchCumulativeBudget(
				test.dimension,
				[]int{test.limit - 1, test.limit + 1, test.limit + 100},
				test.limit,
			)
			assertAtomicTransactionBatchBudgetError(
				t,
				err,
				test.dimension,
				1,
				test.limit+1,
				test.limit,
			)
		})
	}
}

func TestAtomicTransactionBatchDefaultBudgetLimits_ReleaseGate(t *testing.T) {
	t.Parallel()

	assert.Equal(t, atomicTransactionBatchBudgetLimits{
		expandedPostings:       100,
		executionBalances:      150,
		completionPlanBytes:    256 * 1024,
		accountingRequestBytes: 256 * 1024,
		preparedResponseBytes:  1024 * 1024,
		recoveryBytes:          512 * 1024,
		cachedResponseBytes:    1024 * 1024,
	}, defaultAtomicTransactionBatchBudgetLimits)
}

func TestAtomicTransactionBatchWriteBehindBudgetIncludesIndexesDependenciesCapturesAndFeeAttributes(t *testing.T) {
	envelope := writeBehindEnvelopeFixture(t)
	predecessor := TransactionEvidenceReference{
		Kind: TransactionDependencyPredecessor, TenantID: envelope.Record.TenantID,
		OrganizationID: envelope.Record.OrganizationID, LedgerID: envelope.Record.LedgerID,
		TransactionID: envelope.Record.TransactionID, ExecutionID: uuid.New(),
	}
	envelope.Dependencies = []TransactionEvidenceReference{predecessor}
	encodedEnvelope, err := EncodeTransactionWriteBehindEnvelope(envelope)
	require.NoError(t, err)
	legacyRecord, err := json.Marshal(envelope.Record)
	require.NoError(t, err)
	require.Greater(t, len(encodedEnvelope), len(legacyRecord), "recovery budget must include the versioned wrapper and dependency")

	plan, err := DecodeTransactionCompletionPlan([]byte(envelope.Record.Payload))
	require.NoError(t, err)
	field := envelope.Record.TransactionID.String() + ":" + envelope.Record.ExecutionID.String()
	encodedIndex, err := EncodeTransactionEvidenceIndex(TransactionEvidenceIndex{
		FormatVersion: TransactionEvidenceIndexFormatVersion, TenantID: envelope.Record.TenantID,
		OrganizationID: envelope.Record.OrganizationID, LedgerID: envelope.Record.LedgerID,
		TransactionID: envelope.Record.TransactionID, ExecutionID: envelope.Record.ExecutionID,
		Action: plan.Action, ApplicationState: TransactionApplicationConfirmed,
		ReplayState: TransactionReplayReconstructible, DurabilityState: TransactionDurabilityPending,
		RecoveryField: field, ReceiptField: envelope.Record.ExecutionID.String(),
		Dependencies: []TransactionEvidenceReference{predecessor},
	})
	require.NoError(t, err)
	require.Contains(t, string(encodedIndex), `"dependencies":[{`)

	routeID := uuid.NewString()
	rich := &postgresTransaction.Transaction{
		ID: envelope.Record.TransactionID.String(), Description: `fee \"quoted\" \\ route`, RouteID: &routeID,
		Metadata: map[string]any{"midaz:fee": "materialized", "escaped": `a\"b\\c`},
	}
	rawCapture, err := json.Marshal(rich)
	require.NoError(t, err)
	capture := base64.StdEncoding.EncodeToString(rawCapture)
	response, err := json.Marshal(struct {
		Transactions []json.RawMessage `json:"transactions"`
	}{Transactions: []json.RawMessage{rawCapture}})
	require.NoError(t, err)
	richCached, err := encodeAtomicTransactionBatchCachedBudget(
		[]*postgresTransaction.Transaction{rich}, []TransactionCompletionPlan{*plan},
		map[string]string{rich.ID: capture}, response,
	)
	require.NoError(t, err)
	plainCached, err := encodeAtomicTransactionBatchCachedBudget(
		[]*postgresTransaction.Transaction{{ID: rich.ID}}, nil, nil,
		json.RawMessage(`{"transactions":[]}`),
	)
	require.NoError(t, err)
	require.Greater(t, len(richCached), len(plainCached))
	require.Contains(t, string(richCached), capture, "cached budget must charge the exact base64 immutable capture")
}

func TestCreateAtomicTransactionBatchV2_BudgetFailureDeletesClaimBeforeBalanceRead(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000081")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000082")
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-000000000083")
	transactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000084")
	reader := &atomicTransactionBatchSettingsReader{settings: mmodel.LedgerSettings{}}
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	limits := defaultAtomicTransactionBatchBudgetLimits
	limits.expandedPostings = 1
	uc := &UseCase{
		TransactionReader:                     reader,
		AtomicTransactionBatchIdempotencyRepo: repository,
		UUIDv7Generator: orderedAtomicTransactionBatchUUIDs(
			t,
			batchID,
			transactionID,
		),
		Clock: func() time.Time { return time.Date(2026, time.September, 16, 16, 0, 0, 0, time.UTC) },
		atomicTransactionBatchBudgetLimitOverride: &limits,
	}

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), CreateAtomicTransactionBatchV2Input{
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{
			atomicTransactionBatchItemInput(organizationID, ledgerID, "@source", "@destination"),
		},
		CanonicalRequest: []byte(`{"transactions":[{"description":"budget"}]}`),
		IdempotencyKey:   "client-batch-key",
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assertAtomicTransactionBatchBudgetError(
		t,
		err,
		atomicTransactionBatchBudgetExpandedPostings,
		0,
		2,
		1,
	)

	assert.Equal(t, 1, repository.claims)
	assert.Equal(t, 1, repository.deletes)
	assert.Equal(t, "client-batch-key", repository.effectiveKey)
	assert.NotEmpty(t, repository.claim.OwnerToken)
	assert.Equal(t, repository.claim.OwnerToken, repository.deleteOwner)
	assert.Equal(t, txRedis.AtomicTransactionBatchStateClaimed, repository.claim.State)
	assert.Equal(t, batchID, repository.claim.BatchID)
	assert.Equal(t, 0, reader.engineReads, "expanded postings must reject before the shared balance read")
}

func assertAtomicTransactionBatchBudgetError(
	t *testing.T,
	err error,
	dimension string,
	index, observed, limit int,
) {
	t.Helper()
	require.Error(t, err)

	var business pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &business))
	assert.Equal(t, constant.ErrTransactionBatchBudgetExceeded.Error(), business.Code)
	assert.Equal(t, "Transaction Batch Budget Exceeded", business.Title)
	assert.Equal(t, "The transaction batch exceeds the "+dimension+" budget at transaction index "+
		fmt.Sprint(index)+": observed "+fmt.Sprint(observed)+", maximum "+fmt.Sprint(limit)+
		". Please reduce the batch work and try again.", business.Message)

	var carrier *pkg.FieldErrorCarrier
	require.True(t, errors.As(err, &carrier))
	assert.Equal(t, []pkg.FieldError{{
		Location: "body.transactions[" + fmt.Sprint(index) + "]",
		Message:  dimension + " budget observed " + fmt.Sprint(observed) + " exceeds maximum " + fmt.Sprint(limit),
	}}, carrier.FieldErrors())
}
