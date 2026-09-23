// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	operationPostgres "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	transactionPostgres "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type atomicTransactionBatchRecoveryRepositoryFake struct {
	*atomicTransactionBatchClaimRepositoryFake

	candidate *txRedis.AtomicTransactionBatchFinalizationCandidateResult
	err       error
	calls     int
	identity  [4]uuid.UUID
	captures  []json.RawMessage
}

func (repository *atomicTransactionBatchRecoveryRepositoryFake) CaptureAtomicTransactionBatchInitialResponse(
	_ context.Context,
	_, _, _ uuid.UUID,
	_ string,
	transactionID uuid.UUID,
	response json.RawMessage,
) (*txRedis.AtomicTransactionBatchInitialResponseCaptureResult, error) {
	if repository.candidate == nil {
		return nil, errors.New("missing recovery candidate")
	}

	repository.captures = append(repository.captures, append(json.RawMessage(nil), response...))
	if repository.candidate.Record.FormatVersion == txRedis.AtomicTransactionBatchIdempotencyFormatVersion {
		if repository.candidate.Record.InitialResponses == nil {
			repository.candidate.Record.InitialResponses = make(map[string]string)
		}
		repository.candidate.Record.InitialResponses[transactionID.String()] = base64.StdEncoding.EncodeToString(response)
	}

	return &txRedis.AtomicTransactionBatchInitialResponseCaptureResult{
		Outcome: txRedis.AtomicTransactionBatchInitialResponseCaptured,
		Record:  repository.candidate.Record,
	}, nil
}

func TestPrepareAtomicTransactionBatchRecoveryFinalization_V2CapturesDirectWithoutProjectionRead(t *testing.T) {
	fixture := atomicTransactionBatchRecoveryFixture(false)
	fixture.repository.candidate.Record.FormatVersion = txRedis.AtomicTransactionBatchIdempotencyFormatVersion
	fixture.repository.candidate.Candidate = false

	prepared, err := fixture.useCase.PrepareAtomicTransactionBatchRecoveryFinalization(context.Background(), fixture.record, fixture.completion)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	assert.Zero(t, fixture.reader.calls)
	require.Len(t, fixture.repository.captures, 1)

	var public transactionPostgres.Transaction
	require.NoError(t, json.Unmarshal(fixture.repository.captures[0], &public))
	assert.Equal(t, constant.CREATED, public.Status.Code)
}

func TestPrepareAtomicTransactionBatchRecoveryFinalization_V2CapturesPendingHoldWithoutProjectionRead(t *testing.T) {
	fixture := atomicTransactionBatchRecoveryFixture(false)
	fixture.repository.candidate.Record.FormatVersion = txRedis.AtomicTransactionBatchIdempotencyFormatVersion
	fixture.repository.candidate.Candidate = false
	pending := constant.PENDING
	fixture.completion.Outcome.TransactionStatus = pending
	fixture.completion.Record.Transaction.Status = transactionPostgres.Status{Code: pending, Description: &pending}

	prepared, err := fixture.useCase.PrepareAtomicTransactionBatchRecoveryFinalization(context.Background(), fixture.record, fixture.completion)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	assert.Zero(t, fixture.reader.calls)
	require.Len(t, fixture.repository.captures, 1)

	var public transactionPostgres.Transaction
	require.NoError(t, json.Unmarshal(fixture.repository.captures[0], &public))
	assert.Equal(t, constant.PENDING, public.Status.Code)
}

func (repository *atomicTransactionBatchRecoveryRepositoryFake) GetAtomicTransactionBatchFinalizationCandidate(
	_ context.Context,
	organizationID, ledgerID, executionID, transactionID uuid.UUID,
) (*txRedis.AtomicTransactionBatchFinalizationCandidateResult, error) {
	repository.calls++
	repository.identity = [4]uuid.UUID{organizationID, ledgerID, executionID, transactionID}

	return repository.candidate, repository.err
}

type atomicTransactionBatchProjectionReaderFake struct {
	transactions []*transactionPostgres.Transaction
	err          error
	calls        int
	organization uuid.UUID
	ledger       uuid.UUID
	ids          []uuid.UUID
}

func (reader *atomicTransactionBatchProjectionReaderFake) GetAtomicTransactionBatchProjections(
	_ context.Context,
	organizationID, ledgerID uuid.UUID,
	transactionIDs []uuid.UUID,
) ([]*transactionPostgres.Transaction, error) {
	reader.calls++
	reader.organization = organizationID
	reader.ledger = ledgerID
	reader.ids = append([]uuid.UUID(nil), transactionIDs...)

	return reader.transactions, reader.err
}

type atomicTransactionBatchRecoveryTracerFake struct {
	confirmed []uuid.UUID
}

func (*atomicTransactionBatchRecoveryTracerFake) Reserve(
	context.Context,
	tracer.ReserveRequest,
) (*tracer.ReserveResult, error) {
	return nil, errors.New("unexpected recovery reservation")
}

func (*atomicTransactionBatchRecoveryTracerFake) Confirm(context.Context, uuid.UUID) error {
	return errors.New("unexpected recovery confirmation by reservation")
}

func (*atomicTransactionBatchRecoveryTracerFake) Release(context.Context, uuid.UUID) error {
	return errors.New("unexpected recovery release")
}

func (fake *atomicTransactionBatchRecoveryTracerFake) ConfirmByTransaction(
	_ context.Context,
	transactionID uuid.UUID,
) error {
	fake.confirmed = append(fake.confirmed, transactionID)

	return nil
}

func (*atomicTransactionBatchRecoveryTracerFake) ReleaseByTransaction(context.Context, uuid.UUID) error {
	return errors.New("unexpected recovery release by transaction")
}

func TestPrepareAtomicTransactionBatchRecoveryFinalization_NonBatchPreservesLegacyAck(t *testing.T) {
	fixture := atomicTransactionBatchRecoveryFixture(false)
	fixture.repository.candidate = nil

	prepared, err := fixture.useCase.PrepareAtomicTransactionBatchRecoveryFinalization(
		context.Background(),
		fixture.record,
		fixture.completion,
	)
	require.NoError(t, err)
	assert.Nil(t, prepared)
	assert.Equal(t, 1, fixture.repository.calls)
	assert.Zero(t, fixture.reader.calls)
	assert.Empty(t, fixture.tracer.confirmed)
}

func TestPrepareAtomicTransactionBatchRecoveryFinalization_UsesRecordedCoordinationScope(t *testing.T) {
	fixture := atomicTransactionBatchRecoveryFixture(false)
	fixture.repository.candidate = nil
	coordinationLedgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000100")
	raw, err := json.Marshal(fixture.record)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(raw, &fields))
	fields["coordinationOrganizationId"] = fixture.organizationID.String()
	fields["coordinationLedgerId"] = coordinationLedgerID.String()
	raw, err = json.Marshal(fields)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, fixture.record))

	prepared, err := fixture.useCase.PrepareAtomicTransactionBatchRecoveryFinalization(context.Background(), fixture.record, fixture.completion)
	require.NoError(t, err)
	require.Nil(t, prepared)
	require.Equal(t, [4]uuid.UUID{fixture.organizationID, coordinationLedgerID, fixture.executionID, fixture.transactionIDs[0]}, fixture.repository.identity)

	// Records written before this fix continue to resolve through their own scope.
	legacy := atomicTransactionBatchRecoveryFixture(false)
	legacy.repository.candidate = nil
	prepared, err = legacy.useCase.PrepareAtomicTransactionBatchRecoveryFinalization(context.Background(), legacy.record, legacy.completion)
	require.NoError(t, err)
	require.Nil(t, prepared)
	require.Equal(t, [4]uuid.UUID{legacy.organizationID, legacy.ledgerID, legacy.executionID, legacy.transactionIDs[0]}, legacy.repository.identity)
}

func TestPrepareAtomicTransactionBatchRecoveryFinalization_IntermediateMemberAvoidsFullRead(t *testing.T) {
	fixture := atomicTransactionBatchRecoveryFixture(false)
	fixture.repository.candidate.Candidate = false

	prepared, err := fixture.useCase.PrepareAtomicTransactionBatchRecoveryFinalization(
		context.Background(),
		fixture.record,
		fixture.completion,
	)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	assert.Empty(t, prepared.ReceiptToken)
	assert.Nil(t, prepared.Transactions)
	assert.Zero(t, fixture.reader.calls)
	assert.Equal(t, []uuid.UUID{fixture.transactionIDs[0]}, fixture.tracer.confirmed)
	assert.Equal(t, [4]uuid.UUID{
		fixture.organizationID,
		fixture.ledgerID,
		fixture.executionID,
		fixture.transactionIDs[0],
	}, fixture.repository.identity)
}

func TestPrepareAtomicTransactionBatchRecoveryFinalization_LastMemberReadsOnceAndRestoresCreated(t *testing.T) {
	fixture := atomicTransactionBatchRecoveryFixture(false)
	fixture.repository.candidate.Candidate = true
	fixture.repository.candidate.ReceiptToken = `{"executionId":"receipt-token"}`
	fixture.reader.transactions = []*transactionPostgres.Transaction{
		atomicTransactionBatchRecoveredProjection(
			fixture.organizationID,
			fixture.ledgerID,
			fixture.transactionIDs[1],
			false,
		),
		atomicTransactionBatchRecoveredProjection(
			fixture.organizationID,
			fixture.ledgerID,
			fixture.transactionIDs[0],
			false,
		),
	}

	prepared, err := fixture.useCase.PrepareAtomicTransactionBatchRecoveryFinalization(
		context.Background(),
		fixture.record,
		fixture.completion,
	)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	assert.Equal(t, fixture.repository.candidate.ReceiptToken, prepared.ReceiptToken)
	assert.Equal(t, 1, fixture.reader.calls)
	assert.Equal(t, fixture.organizationID, fixture.reader.organization)
	assert.Equal(t, fixture.ledgerID, fixture.reader.ledger)
	assert.Equal(t, fixture.transactionIDs, fixture.reader.ids)
	assert.Equal(t, []uuid.UUID{fixture.transactionIDs[0]}, fixture.tracer.confirmed)
	require.Len(t, prepared.Transactions, 2)
	for _, transactionID := range fixture.transactionIDs {
		var public transactionPostgres.Transaction
		require.NoError(t, json.Unmarshal(prepared.Transactions[transactionID], &public))
		assert.Equal(t, transactionID.String(), public.ID)
		assert.Equal(t, constant.CREATED, public.Status.Code)
		require.NotNil(t, public.Status.Description)
		assert.Equal(t, constant.CREATED, *public.Status.Description)
	}
}

func TestPrepareAtomicTransactionBatchRecoveryFinalization_DuplicateCompleteSkipsReadAndTracerSkipDoesNoWork(t *testing.T) {
	fixture := atomicTransactionBatchRecoveryFixture(true)
	fixture.repository.candidate.Record.State = txRedis.AtomicTransactionBatchStateComplete

	prepared, err := fixture.useCase.PrepareAtomicTransactionBatchRecoveryFinalization(
		context.Background(),
		fixture.record,
		fixture.completion,
	)
	require.NoError(t, err)
	require.NotNil(t, prepared)
	assert.Empty(t, prepared.ReceiptToken)
	assert.Zero(t, fixture.reader.calls)
	assert.Empty(t, fixture.tracer.confirmed)
}

func TestPrepareAtomicTransactionBatchRecoveryFinalization_RejectsProjectionOutsideScope(t *testing.T) {
	fixture := atomicTransactionBatchRecoveryFixture(false)
	fixture.repository.candidate.Candidate = true
	fixture.repository.candidate.ReceiptToken = "receipt"
	fixture.reader.transactions = []*transactionPostgres.Transaction{
		atomicTransactionBatchRecoveredProjection(uuid.New(), fixture.ledgerID, fixture.transactionIDs[0], false),
		atomicTransactionBatchRecoveredProjection(fixture.organizationID, fixture.ledgerID, fixture.transactionIDs[1], false),
	}

	prepared, err := fixture.useCase.PrepareAtomicTransactionBatchRecoveryFinalization(
		context.Background(),
		fixture.record,
		fixture.completion,
	)
	assert.Nil(t, prepared)
	require.ErrorContains(t, err, "is not durably complete")
}

type atomicTransactionBatchRecoveryTestFixture struct {
	useCase        *UseCase
	repository     *atomicTransactionBatchRecoveryRepositoryFake
	reader         *atomicTransactionBatchProjectionReaderFake
	tracer         *atomicTransactionBatchRecoveryTracerFake
	record         *TransactionCompletionRecord
	completion     TransactionCompletionResult
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	executionID    uuid.UUID
	transactionIDs []uuid.UUID
}

func atomicTransactionBatchRecoveryFixture(tracerSkipped bool) atomicTransactionBatchRecoveryTestFixture {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000101")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000102")
	executionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000103")
	transactionIDs := []uuid.UUID{
		uuid.MustParse("01994f13-29b7-7000-8000-000000000104"),
		uuid.MustParse("01994f13-29b7-7000-8000-000000000105"),
	}
	engineID := executionID
	batchRecord := txRedis.AtomicTransactionBatchIdempotencyRecord{
		FormatVersion:      txRedis.AtomicTransactionBatchLegacyFormatVersion,
		State:              txRedis.AtomicTransactionBatchStateApplied,
		RequestFingerprint: "request-fingerprint",
		OwnerToken:         "owner-token",
		BatchID:            uuid.MustParse("01994f13-29b7-7000-8000-000000000106"),
		ExecutionID:        &engineID,
		TransactionIDs:     append([]uuid.UUID(nil), transactionIDs...),
	}
	repository := &atomicTransactionBatchRecoveryRepositoryFake{
		atomicTransactionBatchClaimRepositoryFake: &atomicTransactionBatchClaimRepositoryFake{},
		candidate: &txRedis.AtomicTransactionBatchFinalizationCandidateResult{
			Record: batchRecord,
		},
	}
	reader := &atomicTransactionBatchProjectionReaderFake{}
	tracerFake := &atomicTransactionBatchRecoveryTracerFake{}
	current := atomicTransactionBatchRecoveredProjection(
		organizationID,
		ledgerID,
		transactionIDs[0],
		tracerSkipped,
	)
	record := &TransactionCompletionRecord{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		ExecutionID:    executionID,
		TransactionID:  transactionIDs[0],
	}
	completion := TransactionCompletionResult{
		Record: TransactionWriteSet{Transaction: current},
		Outcome: TransactionPersistenceOutcome{
			TransactionStatus: constant.APPROVED,
		},
	}

	return atomicTransactionBatchRecoveryTestFixture{
		useCase: &UseCase{
			AtomicTransactionBatchIdempotencyRepo:  repository,
			AtomicTransactionBatchProjectionReader: reader,
			TracerReserver:                         tracerFake,
		},
		repository:     repository,
		reader:         reader,
		tracer:         tracerFake,
		record:         record,
		completion:     completion,
		organizationID: organizationID,
		ledgerID:       ledgerID,
		executionID:    executionID,
		transactionIDs: transactionIDs,
	}
}

func atomicTransactionBatchRecoveredProjection(
	organizationID, ledgerID, transactionID uuid.UUID,
	tracerSkipped bool,
) *transactionPostgres.Transaction {
	amount := decimal.NewFromInt(10)
	approved := constant.APPROVED

	return &transactionPostgres.Transaction{
		ID:             transactionID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status: transactionPostgres.Status{
			Code:        approved,
			Description: &approved,
		},
		Amount:        &amount,
		AssetCode:     "BRL",
		TracerSkipped: tracerSkipped,
		Operations: []*operationPostgres.Operation{{
			ID:            uuid.NewSHA1(transactionID, []byte("operation")).String(),
			TransactionID: transactionID.String(),
			Type:          constant.DEBIT,
		}},
	}
}
