// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestBuildCrossLedgerRevertBatchInput_ReversesOrderAndLinksEveryOrigin(t *testing.T) {
	t.Parallel()

	revertedGroupID := uuid.MustParse("0199517b-43ee-7000-8000-000000000001")
	newGroupID := uuid.MustParse("0199517b-43ee-7000-8000-000000000002")
	organizationID := uuid.MustParse("0199517b-43ee-7000-8000-000000000003")
	ledgerA := uuid.MustParse("0199517b-43ee-7000-8000-000000000004")
	ledgerB := uuid.MustParse("0199517b-43ee-7000-8000-000000000005")
	originA := revertibleOrigin()
	originA.ID = "0199517b-43ee-7000-8000-000000000006"
	originA.OrganizationID = organizationID.String()
	originA.LedgerID = ledgerA.String()
	originB := revertibleOrigin()
	originB.ID = "0199517b-43ee-7000-8000-000000000007"
	originB.OrganizationID = organizationID.String()
	originB.LedgerID = ledgerB.String()
	executionA := uuid.MustParse("0199517b-43ee-7000-8000-000000000008")
	executionB := uuid.MustParse("0199517b-43ee-7000-8000-000000000009")
	exceptionID := uuid.MustParse("0199517b-43ee-7000-8000-00000000000a")

	parts := []preparedCrossLedgerRevertPart{
		{
			origin:     originA,
			reversal:   originA.TransactionRevert(),
			dependency: originDependencyReference("tenant-a", organizationID, ledgerA, uuid.MustParse(originA.ID), executionA),
		},
		{
			origin:     originB,
			reversal:   originB.TransactionRevert(),
			dependency: originDependencyReference("tenant-a", organizationID, ledgerB, uuid.MustParse(originB.ID), executionB),
		},
	}

	got, err := buildCrossLedgerRevertBatchInput(RevertTransactionInput{
		OrganizationID:          organizationID,
		LedgerID:                ledgerA,
		TransactionID:           uuid.MustParse(originA.ID),
		AccountBlockExceptionID: &exceptionID,
	}, revertedGroupID, newGroupID, parts)
	require.NoError(t, err)
	require.NotNil(t, got.GroupID)
	assert.Equal(t, newGroupID, *got.GroupID)
	assert.True(t, got.CrossLedgerGroup)
	assert.NotEmpty(t, got.IdempotencyKey)
	assert.Equal(t, crossLedgerRevertIdempotencyKey(organizationID, ledgerA, revertedGroupID), got.IdempotencyKey)
	otherMember, err := buildCrossLedgerRevertBatchInput(RevertTransactionInput{
		OrganizationID: organizationID,
		LedgerID:       ledgerB,
		TransactionID:  uuid.MustParse(originB.ID),
	}, revertedGroupID, newGroupID, parts)
	require.NoError(t, err)
	assert.Equal(t, got.IdempotencyKey, otherMember.IdempotencyKey,
		"either member of a group must address the same idempotency claim")
	require.Len(t, got.Transactions, 2)

	assert.Equal(t, originB.ID, got.Transactions[0].ParentTransactionID.String())
	assert.Equal(t, originA.ID, got.Transactions[1].ParentTransactionID.String())
	assert.Equal(t, []TransactionEvidenceReference{parts[1].dependency}, got.Transactions[0].Dependencies)
	assert.Equal(t, []TransactionEvidenceReference{parts[0].dependency}, got.Transactions[1].Dependencies)
	assert.Equal(t, []int{1, 2}, []int{got.Transactions[0].Order, got.Transactions[1].Order})
	assert.Equal(t, []int{1, 0}, []int{got.Transactions[0].OriginalIndex, got.Transactions[1].OriginalIndex})
	assert.Nil(t, got.Transactions[0].AccountBlockExceptionID)
	require.NotNil(t, got.Transactions[1].AccountBlockExceptionID)
	assert.Equal(t, exceptionID, *got.Transactions[1].AccountBlockExceptionID)
}

func TestRevertCrossLedgerGroupV2_RejectsIncompleteGroupBeforeBatchWork(t *testing.T) {
	groupID := uuid.NewString()
	origin := revertibleOrigin()
	origin.GroupID = &groupID
	reader := &revertReader{
		byID:         origin,
		groupMembers: []*transaction.Transaction{origin},
	}
	uc := newRevertUseCase(t, reader)

	result, revertedGroupID, err := uc.RevertCrossLedgerGroupV2(context.Background(), RevertTransactionInput{
		OrganizationID: uuid.MustParse(origin.OrganizationID),
		LedgerID:       uuid.MustParse(origin.LedgerID),
		TransactionID:  uuid.MustParse(origin.ID),
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, uuid.Nil, revertedGroupID)
	assert.Contains(t, err.Error(), constant.ErrCrossLedgerGroupIncomplete.Error())
	assert.Zero(t, reader.getBalancesCalls)
}

func TestRevertCrossLedgerGroupV2_LocatesIneligibleMember(t *testing.T) {
	groupID := uuid.NewString()
	first := revertibleOrigin()
	first.GroupID = &groupID
	second := revertibleOrigin()
	second.GroupID = &groupID
	revertGroupID := uuid.NewString()
	reader := &revertReader{
		byID:         first,
		groupMembers: []*transaction.Transaction{first, second},
		parent:       &transaction.Transaction{ID: uuid.NewString(), GroupID: &revertGroupID},
	}
	uc := newRevertUseCase(t, reader)

	result, revertedGroupID, err := uc.RevertCrossLedgerGroupV2(context.Background(), RevertTransactionInput{
		OrganizationID: uuid.MustParse(first.OrganizationID),
		LedgerID:       uuid.MustParse(first.LedgerID),
		TransactionID:  uuid.MustParse(first.ID),
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, uuid.Nil, revertedGroupID)
	var conflict pkg.EntityConflictError
	require.True(t, errors.As(err, &conflict))
	assert.Equal(t, constant.ErrTransactionIDHasAlreadyParentTransaction.Error(), conflict.Code)

	var carrier *pkg.FieldErrorCarrier
	require.True(t, errors.As(err, &carrier))
	assert.Equal(t, []pkg.FieldError{{
		Location: "body.transactions[0]",
		Message:  "transaction " + first.ID + " in ledger " + first.LedgerID + " is not revertible",
	}}, carrier.FieldErrors())
	assert.Zero(t, reader.getBalancesCalls)
}

func TestValidateCrossLedgerRevertMembers_RejectsIncompleteOrUnrelatedSets(t *testing.T) {
	t.Parallel()

	groupID := uuid.MustParse("0199517b-43ee-7000-8000-000000000021")
	groupText := groupID.String()
	requestedID := uuid.MustParse("0199517b-43ee-7000-8000-000000000022")
	otherID := uuid.MustParse("0199517b-43ee-7000-8000-000000000023")
	wrongGroup := uuid.MustParse("0199517b-43ee-7000-8000-000000000024").String()

	tests := []struct {
		name    string
		members []*transaction.Transaction
	}{
		{name: "one member", members: []*transaction.Transaction{{ID: requestedID.String(), GroupID: &groupText}}},
		{name: "requested member absent", members: []*transaction.Transaction{
			{ID: otherID.String(), GroupID: &groupText},
			{ID: uuid.NewString(), GroupID: &groupText},
		}},
		{name: "mismatched group", members: []*transaction.Transaction{
			{ID: requestedID.String(), GroupID: &groupText},
			{ID: otherID.String(), GroupID: &wrongGroup},
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCrossLedgerRevertMembers(requestedID, groupID, test.members)
			require.Error(t, err)

			var business pkg.UnprocessableOperationError
			require.True(t, errors.As(err, &business))
			assert.Equal(t, constant.ErrCrossLedgerGroupIncomplete.Error(), business.Code)
		})
	}
}
