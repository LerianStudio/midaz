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

func TestPrepareAtomicTransactionBatchCompletionPlans_GroupedPlansCarryTheExecutionMembers(t *testing.T) {
	organizationID := uuid.MustParse("0199a610-0000-7000-8000-000000000001")
	primaryLedgerID := uuid.MustParse("0199a610-0000-7000-8000-000000000002")
	foreignLedgerID := uuid.MustParse("0199a610-0000-7000-8000-000000000003")
	groupID := uuid.MustParse("0199a610-0000-7000-8000-000000000004")
	firstID := uuid.MustParse("0199a610-0000-7000-8000-000000000005")
	secondID := uuid.MustParse("0199a610-0000-7000-8000-000000000006")
	executionID := uuid.MustParse("0199a610-0000-7000-8000-000000000007")

	for _, test := range []struct {
		name    string
		groupID *uuid.UUID
		action  string
		pending bool
	}{
		{name: "cross-ledger direct", groupID: &groupID, action: constant.ActionDirect},
		{name: "cross-ledger hold of two origins", groupID: &groupID, action: constant.ActionHold, pending: true},
		{name: "ungrouped batch", action: constant.ActionDirect},
	} {
		t.Run(test.name, func(t *testing.T) {
			primaryRef := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: primaryLedgerID}
			foreignRef := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: foreignLedgerID}
			reader := &atomicTransactionBatchSettingsReader{
				settingsByRef: map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{
					primaryRef: {CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}},
					foreignRef: {CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}},
				},
				balances: []*mmodel.Balance{
					atomicTransactionBatchTestBalance(organizationID, primaryLedgerID, "0199a610-0000-7000-8000-000000000011", "@source", "BRL"),
					atomicTransactionBatchTestBalance(organizationID, primaryLedgerID, "0199a610-0000-7000-8000-000000000012", "@destination", "BRL"),
					atomicTransactionBatchTestBalance(organizationID, foreignLedgerID, "0199a610-0000-7000-8000-000000000013", "@source", "BRL"),
					atomicTransactionBatchTestBalance(organizationID, foreignLedgerID, "0199a610-0000-7000-8000-000000000014", "@destination", "BRL"),
				},
			}
			ids := []uuid.UUID{firstID, secondID, executionID}
			if test.groupID == nil {
				ids = append([]uuid.UUID{groupID}, ids...)
			}
			uc := &UseCase{
				TransactionReader: reader,
				UUIDv7Generator:   orderedAtomicTransactionBatchUUIDs(t, ids...),
				Clock:             func() time.Time { return time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC) },
			}
			items := []CreateAtomicTransactionBatchV2ItemInput{
				atomicTransactionBatchItemInput(organizationID, primaryLedgerID, "@source", "@destination"),
				atomicTransactionBatchItemInput(organizationID, foreignLedgerID, "@source", "@destination"),
			}
			for index := range items {
				items[index].Action = test.action
				items[index].Order = index + 1
				items[index].OriginalIndex = index
				items[index].Transaction.Pending = test.pending
			}

			run, err := uc.initializeAtomicTransactionBatchV2(context.Background(), CreateAtomicTransactionBatchV2Input{
				Transactions: items, GroupID: test.groupID, CrossLedgerGroup: test.groupID != nil,
			})
			require.NoError(t, err)
			require.NoError(t, uc.prepareAtomicTransactionBatchItems(context.Background(), nil, nil, run))
			require.NoError(t, uc.prepareAtomicTransactionBatchCompletionPlans(context.Background(), run))

			var want []TransactionCompletionMember
			if test.groupID != nil {
				want = []TransactionCompletionMember{
					{TransactionID: firstID, OrganizationID: organizationID, LedgerID: primaryLedgerID},
					{TransactionID: secondID, OrganizationID: organizationID, LedgerID: foreignLedgerID},
				}
			}

			for index := range run.items {
				plan, err := DecodeTransactionCompletionPlan(run.items[index].completionPlanPayload)
				require.NoError(t, err)
				assert.Equal(t, want, plan.ExecutionMembers, "item %d", index)
				assert.Equal(t, want, run.items[index].completionPlan.ExecutionMembers, "item %d", index)
			}
		})
	}
}

func TestCreateAtomicTransactionBatchV2_GroupRevertPlansCarryTheReversalMembers(t *testing.T) {
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	engine := &applyingAtomicTransactionBatchEngine{t: t}
	uc, input, transactionIDs, executionID := atomicTransactionBatchExecutionFixture(t, repository, engine, atomicTransactionBatchExecutionReserver())

	groupID := uuid.MustParse("0199a620-0000-7000-8000-000000000001")
	parentIDs := []uuid.UUID{
		uuid.MustParse("0199a620-0000-7000-8000-000000000002"),
		uuid.MustParse("0199a620-0000-7000-8000-000000000003"),
	}
	uc.UUIDv7Generator = orderedAtomicTransactionBatchUUIDs(t, transactionIDs[0], transactionIDs[1], executionID)
	input.GroupID = &groupID
	input.CrossLedgerGroup = true

	for index := range input.Transactions {
		item := &input.Transactions[index]
		item.Action = constant.ActionRevert
		item.Order = index + 1
		item.OriginalIndex = len(input.Transactions) - 1 - index
		item.ParentTransactionID = &parentIDs[index]
		item.Dependencies = []TransactionEvidenceReference{{
			Kind: TransactionDependencyOrigin, OrganizationID: item.OrganizationID, LedgerID: item.LedgerID,
			TransactionID: parentIDs[index], ExecutionID: uuid.New(),
		}}
	}

	_, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, engine.executions, 1)

	want := []TransactionCompletionMember{
		{TransactionID: transactionIDs[0], OrganizationID: input.Transactions[0].OrganizationID, LedgerID: input.Transactions[0].LedgerID},
		{TransactionID: transactionIDs[1], OrganizationID: input.Transactions[1].OrganizationID, LedgerID: input.Transactions[1].LedgerID},
	}
	for index, record := range engine.executions[0].CompletionPlans {
		plan, err := DecodeTransactionCompletionPlan(record.Payload)
		require.NoError(t, err)
		assert.Equal(t, want, plan.ExecutionMembers,
			"plan %d must list the reversal transactions, never the reverted parents", index)
	}
}

func TestMeasureAtomicTransactionBatchBudgets_ChargesTheEngineReceiptAndIndexScopes(t *testing.T) {
	organizationID := uuid.MustParse("0199a620-0000-7000-8000-000000000001")
	primaryLedgerID := uuid.MustParse("0199a620-0000-7000-8000-000000000002")
	foreignLedgerID := uuid.MustParse("0199a620-0000-7000-8000-000000000003")
	primaryRef := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: primaryLedgerID}
	foreignRef := atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: foreignLedgerID}
	reader := &atomicTransactionBatchSettingsReader{
		settingsByRef: map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{
			primaryRef: {CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}},
			foreignRef: {CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}},
		},
		balances: []*mmodel.Balance{
			atomicTransactionBatchTestBalance(organizationID, primaryLedgerID, "0199a620-0000-7000-8000-000000000011", "@source", "BRL"),
			atomicTransactionBatchTestBalance(organizationID, primaryLedgerID, "0199a620-0000-7000-8000-000000000012", "@destination", "BRL"),
			atomicTransactionBatchTestBalance(organizationID, foreignLedgerID, "0199a620-0000-7000-8000-000000000013", "@source", "BRL"),
			atomicTransactionBatchTestBalance(organizationID, foreignLedgerID, "0199a620-0000-7000-8000-000000000014", "@destination", "BRL"),
		},
	}
	uc := &UseCase{
		TransactionReader: reader,
		UUIDv7Generator: orderedAtomicTransactionBatchUUIDs(
			t,
			uuid.MustParse("0199a620-0000-7000-8000-000000000004"),
			uuid.MustParse("0199a620-0000-7000-8000-000000000005"),
			uuid.MustParse("0199a620-0000-7000-8000-000000000006"),
			uuid.MustParse("0199a620-0000-7000-8000-000000000007"),
		),
		Clock: func() time.Time { return time.Date(2026, time.September, 25, 12, 0, 0, 0, time.UTC) },
	}
	items := []CreateAtomicTransactionBatchV2ItemInput{
		atomicTransactionBatchItemInput(organizationID, primaryLedgerID, "@source", "@destination"),
		atomicTransactionBatchItemInput(organizationID, foreignLedgerID, "@source", "@destination"),
	}

	run, err := uc.initializeAtomicTransactionBatchV2(context.Background(), CreateAtomicTransactionBatchV2Input{Transactions: items})
	require.NoError(t, err)
	require.True(t, run.multiScope)
	require.NoError(t, uc.prepareAtomicTransactionBatchItems(context.Background(), nil, nil, run))

	t.Run("index entries name the receipt scope", func(t *testing.T) {
		for index := range run.items {
			item := &run.items[index]
			encoded, err := EncodeTransactionEvidenceIndex(atomicTransactionBatchMeasuredIndex(
				run, item, item.transactionID.String()+":"+run.executionID.String(), nil,
			))
			require.NoError(t, err)

			var fields map[string]any
			require.NoError(t, json.Unmarshal(encoded, &fields))
			assert.Equal(t, run.organizationID.String(), fields["receiptOrganizationId"], "item %d", index)
			assert.Equal(t, run.ledgerID.String(), fields["receiptLedgerId"], "item %d", index)
		}
	})

	t.Run("the receipt protection charges one scope per transaction", func(t *testing.T) {
		multi, err := measureAtomicTransactionBatchBudgets(run)
		require.NoError(t, err)

		run.multiScope = false
		single, err := measureAtomicTransactionBatchBudgets(run)
		run.multiScope = true
		require.NoError(t, err)

		coordinationScope := TransactionCompletionRecord{
			CoordinationOrganizationID: &run.coordinationOrganizationID, CoordinationLedgerID: &run.coordinationLedgerID,
			ReceiptOrganizationID: &run.organizationID, ReceiptLedgerID: &run.ledgerID,
		}
		withScope, err := json.Marshal(coordinationScope)
		require.NoError(t, err)
		withoutScope, err := json.Marshal(TransactionCompletionRecord{})
		require.NoError(t, err)
		recoveryScopeBytes := len(withScope) - len(withoutScope)

		scopes := make([]atomicTransactionBatchMeasuredScope, 0, len(run.items))
		for index := range run.items {
			scopes = append(scopes, atomicTransactionBatchMeasuredScope{
				OrganizationID: run.items[index].organizationID, LedgerID: run.items[index].ledgerID,
			})
			encodedScopes, err := json.Marshal(scopes)
			require.NoError(t, err)

			want := (index+1)*recoveryScopeBytes + len(`,"scopes":`) + len(encodedScopes)
			assert.Equal(t, want, multi.preparedResponseBytes[index]-single.preparedResponseBytes[index], "item %d", index)
		}
	})

	t.Run("the prepared budget holds at its exact boundary", func(t *testing.T) {
		measured, err := measureAtomicTransactionBatchBudgets(run)
		require.NoError(t, err)
		prepared := measured.preparedResponseBytes[len(measured.preparedResponseBytes)-1]

		limits := defaultAtomicTransactionBatchBudgetLimits
		limits.preparedResponseBytes = prepared
		uc.atomicTransactionBatchBudgetLimitOverride = &limits
		require.NoError(t, uc.prepareAndEnforceAtomicTransactionBatchBudgets(context.Background(), run))

		limits.preparedResponseBytes = prepared - 1
		err = uc.prepareAndEnforceAtomicTransactionBatchBudgets(context.Background(), run)
		assertAtomicTransactionBatchBudgetError(
			t, err, atomicTransactionBatchBudgetPreparedResponseBytes, len(run.items)-1, prepared, prepared-1,
		)
		assert.Equal(t, atomicTransactionBatchBudgetPreparedResponseBytes, run.rejectionDimension)
	})
}
