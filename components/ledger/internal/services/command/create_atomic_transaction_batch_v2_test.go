// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type atomicTransactionBatchSettingsReader struct {
	TransactionReader
	settings       mmodel.LedgerSettings
	err            error
	calls          int
	organizationID uuid.UUID
	ledgerID       uuid.UUID
}

func (reader *atomicTransactionBatchSettingsReader) GetParsedLedgerSettings(
	_ context.Context,
	organizationID, ledgerID uuid.UUID,
) (mmodel.LedgerSettings, error) {
	reader.calls++
	reader.organizationID = organizationID
	reader.ledgerID = ledgerID

	return reader.settings, reader.err
}

func TestInitializeAtomicTransactionBatchV2_FreezesOrderedIDsAndNondecreasingTimestamps(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000001")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000002")
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-000000000003")
	firstTransactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000004")
	secondTransactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000005")

	base := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	reader := &atomicTransactionBatchSettingsReader{settings: mmodel.LedgerSettings{}}
	uc := &UseCase{
		TransactionReader: reader,
		UUIDv7Generator:   orderedAtomicTransactionBatchUUIDs(t, batchID, firstTransactionID, secondTransactionID),
		Clock: orderedAtomicTransactionBatchTimes(
			t,
			base,
			base.Add(-time.Second),
			base.Add(2*time.Second),
			base.Add(time.Second),
			base.Add(4*time.Second),
			base.Add(3*time.Second),
		),
	}

	input := CreateAtomicTransactionBatchV2Input{Transactions: []CreateAtomicTransactionBatchV2ItemInput{
		atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-0", "@destination-0"),
		atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-1", "@destination-1"),
	}}

	run, err := uc.initializeAtomicTransactionBatchV2(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, run.items, 2)

	assert.Equal(t, batchID, run.batchID)
	assert.Equal(t, []int{0, 1}, []int{run.items[0].index, run.items[1].index})
	assert.Equal(t, []uuid.UUID{firstTransactionID, secondTransactionID}, []uuid.UUID{
		run.items[0].transactionID,
		run.items[1].transactionID,
	})
	assert.Equal(t, base, run.items[0].transactionCreatedAt)
	assert.Equal(t, base, run.items[0].transactionUpdatedAt)
	assert.Equal(t, base.Add(2*time.Second), run.items[0].operationUpdatedAt)
	assert.Equal(t, base.Add(2*time.Second), run.items[1].transactionCreatedAt)
	assert.Equal(t, base.Add(4*time.Second), run.items[1].transactionUpdatedAt)
	assert.Equal(t, base.Add(4*time.Second), run.items[1].operationUpdatedAt)
	assert.Equal(t, run.items[0].transactionCreatedAt, run.items[0].transactionDate)
	assert.Equal(t, run.items[1].transactionCreatedAt, run.items[1].transactionDate)

	assert.Equal(t, 1, reader.calls)
	assert.Equal(t, organizationID, reader.organizationID)
	assert.Equal(t, ledgerID, reader.ledgerID)

	// Item state is a deep clone: later fee/default/normalization mutation cannot
	// rewrite the caller's ordered request slice.
	run.items[0].input.Send.Source.From[0].AccountAlias = "@mutated"
	assert.Equal(t, "@source-0", input.Transactions[0].Transaction.Send.Source.From[0].AccountAlias)
}

func TestCreateAtomicTransactionBatchV2_PreservesOrderedResult(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000031")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000032")
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-000000000033")
	firstTransactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000034")
	secondTransactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000035")
	now := time.Date(2026, time.September, 16, 12, 30, 0, 0, time.UTC)
	reader := &atomicTransactionBatchSettingsReader{}
	uc := &UseCase{
		TransactionReader: reader,
		UUIDv7Generator: orderedAtomicTransactionBatchUUIDs(
			t,
			batchID,
			firstTransactionID,
			secondTransactionID,
		),
		Clock: func() time.Time { return now },
	}

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), CreateAtomicTransactionBatchV2Input{
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{
			atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-0", "@destination-0"),
			atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-1", "@destination-1"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Transactions, 2)

	assert.Equal(t, batchID, result.BatchID)
	assert.False(t, result.Replayed)
	assert.Equal(t, firstTransactionID.String(), result.Transactions[0].ID)
	assert.Equal(t, secondTransactionID.String(), result.Transactions[1].ID)
	assert.Equal(t, "@source-0", result.Transactions[0].Source[0])
	assert.Equal(t, "@source-1", result.Transactions[1].Source[0])
	assert.Equal(t, now, result.Transactions[0].CreatedAt)
	assert.Equal(t, now, result.Transactions[1].CreatedAt)
	assert.Equal(t, 1, reader.calls)
}

func TestCreateAtomicTransactionBatchV2_RejectsFirstCommonScopeMismatchBeforeExternalWork(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000011")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000012")
	otherLedgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000013")
	reader := &atomicTransactionBatchSettingsReader{}
	uc := &UseCase{TransactionReader: reader}

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), CreateAtomicTransactionBatchV2Input{
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{
			atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-0", "@destination-0"),
			atomicTransactionBatchItemInput(organizationID, otherLedgerID, "@source-1", "@destination-1"),
			atomicTransactionBatchItemInput(uuid.New(), ledgerID, "@source-2", "@destination-2"),
		},
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 0, reader.calls)
	var scopeError pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &scopeError))
	assert.Equal(t, constant.ErrTransactionScopeMismatch.Error(), scopeError.Code)

	var carrier *pkg.FieldErrorCarrier
	require.True(t, errors.As(err, &carrier))
	assert.Equal(t, []pkg.FieldError{{
		Location: "body.transactions[1]",
		Message:  "transaction scope must match the first batch item",
	}}, carrier.FieldErrors())
}

func TestCreateAtomicTransactionBatchV2_ReturnsOnlyFirstStateDependentFailure(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000021")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000022")
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-000000000023")
	transactionIDs := []uuid.UUID{
		uuid.MustParse("01994f13-29b7-7000-8000-000000000024"),
		uuid.MustParse("01994f13-29b7-7000-8000-000000000025"),
		uuid.MustParse("01994f13-29b7-7000-8000-000000000026"),
	}
	now := time.Date(2026, time.September, 16, 13, 0, 0, 0, time.UTC)
	reader := &atomicTransactionBatchSettingsReader{}
	uc := &UseCase{
		TransactionReader: reader,
		UUIDv7Generator: orderedAtomicTransactionBatchUUIDs(
			t,
			batchID,
			transactionIDs[0],
			transactionIDs[1],
			transactionIDs[2],
		),
		Clock: func() time.Time { return now },
	}

	items := []CreateAtomicTransactionBatchV2ItemInput{
		atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-0", "@destination-0"),
		atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-1", "@destination-1"),
		atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-2", "@destination-2"),
	}
	firstFuture := mtransaction.TransactionDate(now.Add(time.Minute))
	secondFuture := mtransaction.TransactionDate(now.Add(2 * time.Minute))
	items[1].Transaction.TransactionDate = &firstFuture
	items[2].Transaction.TransactionDate = &secondFuture

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), CreateAtomicTransactionBatchV2Input{Transactions: items})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 1, reader.calls)
	assertAtomicTransactionBatchValidationCode(t, err, constant.ErrInvalidFutureTransactionDate.Error())

	var carrier *pkg.FieldErrorCarrier
	require.True(t, errors.As(err, &carrier))
	assert.Equal(t, []pkg.FieldError{{
		Location: "body.transactions[1]",
		Message:  "transaction date validation failed",
	}}, carrier.FieldErrors())
}

func atomicTransactionBatchItemInput(
	organizationID, ledgerID uuid.UUID,
	from, to string,
) CreateAtomicTransactionBatchV2ItemInput {
	return CreateAtomicTransactionBatchV2ItemInput{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Transaction: mtransaction.Transaction{
			Description: "ordered batch transaction",
			Send: mtransaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(10),
				Source: mtransaction.Source{From: []mtransaction.FromTo{{
					AccountAlias: from,
					Amount:       &mtransaction.Amount{Value: decimal.NewFromInt(10)},
					IsFrom:       true,
				}}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
					AccountAlias: to,
					Amount:       &mtransaction.Amount{Value: decimal.NewFromInt(10)},
				}}},
			},
		},
	}
}

func orderedAtomicTransactionBatchUUIDs(t *testing.T, values ...uuid.UUID) UUIDv7Generator {
	t.Helper()
	index := 0

	return func() (uuid.UUID, error) {
		t.Helper()
		require.Less(t, index, len(values), "UUIDv7 generator called more often than expected")
		value := values[index]
		index++

		return value, nil
	}
}

func orderedAtomicTransactionBatchTimes(t *testing.T, values ...time.Time) Clock {
	t.Helper()
	index := 0

	return func() time.Time {
		t.Helper()
		require.Less(t, index, len(values), "clock called more often than expected")
		value := values[index]
		index++

		return value
	}
}

func assertAtomicTransactionBatchValidationCode(t *testing.T, err error, code string) {
	t.Helper()

	var validation pkg.ValidationError
	require.True(t, errors.As(err, &validation))
	assert.Equal(t, code, validation.Code)
}
