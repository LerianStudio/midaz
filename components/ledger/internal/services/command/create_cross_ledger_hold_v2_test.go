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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestCreateCrossLedgerHoldV2_PersistsIntentAndExecutesOnlyOrigins(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	organizationID := uuid.MustParse("0199a300-0000-7000-8000-000000000001")
	ledgerA := uuid.MustParse("0199a300-0000-7000-8000-000000000002")
	ledgerB := uuid.MustParse("0199a300-0000-7000-8000-000000000003")
	groupID := uuid.MustParse("0199a300-0000-7000-8000-000000000004")
	now := time.Date(2026, time.September, 22, 15, 0, 0, 0, time.UTC)
	settings := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
	reader := &atomicTransactionBatchSettingsReader{settingsByRef: map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{
		{organizationID: organizationID, ledgerID: ledgerA}: settings,
		{organizationID: organizationID, ledgerID: ledgerB}: settings,
	}}

	var capturedBatch CreateAtomicTransactionBatchV2Input
	uc := &UseCase{
		TransactionGroupRepo: repo,
		TransactionReader:    reader,
		UUIDv7Generator:      func() (uuid.UUID, error) { return groupID, nil },
		Clock:                func() time.Time { return now },
		createAtomicTransactionBatchV2: func(_ context.Context, input CreateAtomicTransactionBatchV2Input) (*CreateAtomicTransactionBatchV2Result, error) {
			capturedBatch = input
			return &CreateAtomicTransactionBatchV2Result{BatchID: groupID, Transactions: []*transaction.Transaction{{ID: uuid.NewString()}}}, nil
		},
	}

	repo.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, group *transactiongroup.TransactionGroup) error {
		assert.Equal(t, groupID, group.ID)
		assert.Equal(t, organizationID, group.OrganizationID)
		assert.Equal(t, ledgerA, group.LedgerID)
		assert.Equal(t, constant.PENDING, group.Status)
		assert.Equal(t, "BRL", group.AssetCode)
		assert.Equal(t, now, group.CreatedAt)

		intent, err := decodeCrossLedgerGroupIntent(group.Intent)
		require.NoError(t, err)
		require.Len(t, intent.Parts, 2)
		assert.Equal(t, CrossLedgerGroupRoleOrigin, intent.Parts[0].Role)
		assert.Equal(t, ledgerA, intent.Parts[0].LedgerID)
		assert.Equal(t, CrossLedgerGroupRoleDestination, intent.Parts[1].Role)
		assert.Equal(t, ledgerB, intent.Parts[1].LedgerID)

		return nil
	})

	result, err := uc.CreateCrossLedgerHoldV2(context.Background(), CreateCrossLedgerTransactionV2Input{
		Transaction: crossLedgerTestTransaction("100",
			[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "100", true)},
			[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "100", false)}),
		Scopes: CrossLedgerTransactionScopes{
			Debits:  []CrossLedgerLegScope{{OrganizationID: organizationID, LedgerID: ledgerA}},
			Credits: []CrossLedgerLegScope{{OrganizationID: organizationID, LedgerID: ledgerB}},
		},
		IdempotencyKey: "hold-key",
		IdempotencyTTL: time.Minute,
	})
	require.NoError(t, err)
	assert.Equal(t, groupID, result.BatchID)
	require.Len(t, capturedBatch.Transactions, 1)
	assert.Equal(t, ledgerA, capturedBatch.Transactions[0].LedgerID)
	assert.Equal(t, constant.ActionHold, capturedBatch.Transactions[0].Action)
	assert.True(t, capturedBatch.Transactions[0].Transaction.Pending)
	require.NotNil(t, capturedBatch.GroupID)
	assert.Equal(t, groupID, *capturedBatch.GroupID)
	assert.True(t, capturedBatch.CrossLedgerGroup)
	assert.Equal(t, "hold-key", capturedBatch.IdempotencyKey)
	assert.Equal(t, 1, reader.callsByRef[atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: ledgerA}])
	assert.Equal(t, 1, reader.callsByRef[atomicTransactionBatchLedgerRef{organizationID: organizationID, ledgerID: ledgerB}])
}

func TestCreateCrossLedgerHoldV2_RejectsRouteValidationBeforePersistence(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	organizationID := uuid.New()
	ledgerA := uuid.New()
	ledgerB := uuid.New()
	settings := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
	routeSettings := settings
	routeSettings.Accounting.ValidateRoutes = true
	uc := &UseCase{
		TransactionGroupRepo: repo,
		TransactionReader: &atomicTransactionBatchSettingsReader{settingsByRef: map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{
			{organizationID: organizationID, ledgerID: ledgerA}: settings,
			{organizationID: organizationID, ledgerID: ledgerB}: routeSettings,
		}},
		UUIDv7Generator: func() (uuid.UUID, error) { return uuid.New(), nil },
		Clock:           func() time.Time { return time.Date(2026, time.September, 22, 16, 0, 0, 0, time.UTC) },
	}

	result, err := uc.CreateCrossLedgerHoldV2(context.Background(), CreateCrossLedgerTransactionV2Input{
		Transaction: crossLedgerTestTransaction("10",
			[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "10", true)},
			[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "10", false)}),
		Scopes: CrossLedgerTransactionScopes{
			Debits:  []CrossLedgerLegScope{{OrganizationID: organizationID, LedgerID: ledgerA}},
			Credits: []CrossLedgerLegScope{{OrganizationID: organizationID, LedgerID: ledgerB}},
		},
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), constant.ErrCrossLedgerRouteValidationUnsupported.Error())
}

func TestCreateCrossLedgerHoldV2_DeletesIntentOnlyForPrePublicationFailure(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	organizationID := uuid.New()
	ledgerA := uuid.New()
	ledgerB := uuid.New()
	groupID := uuid.New()
	settings := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
	uc := &UseCase{
		TransactionGroupRepo: repo,
		TransactionReader: &atomicTransactionBatchSettingsReader{settingsByRef: map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{
			{organizationID: organizationID, ledgerID: ledgerA}: settings,
			{organizationID: organizationID, ledgerID: ledgerB}: settings,
		}},
		UUIDv7Generator: func() (uuid.UUID, error) { return groupID, nil },
		Clock:           func() time.Time { return time.Date(2026, time.September, 22, 17, 0, 0, 0, time.UTC) },
		createAtomicTransactionBatchV2: func(context.Context, CreateAtomicTransactionBatchV2Input) (*CreateAtomicTransactionBatchV2Result, error) {
			return nil, markAtomicTransactionBatchPrePublication(errors.New("preparation failed"))
		},
	}
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
	repo.EXPECT().Delete(gomock.Any(), groupID).Return(nil)

	result, err := uc.CreateCrossLedgerHoldV2(context.Background(), CreateCrossLedgerTransactionV2Input{
		Transaction: crossLedgerTestTransaction("10",
			[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "10", true)},
			[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "10", false)}),
		Scopes: CrossLedgerTransactionScopes{
			Debits:  []CrossLedgerLegScope{{OrganizationID: organizationID, LedgerID: ledgerA}},
			Credits: []CrossLedgerLegScope{{OrganizationID: organizationID, LedgerID: ledgerB}},
		},
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "preparation failed")
}

func TestCreateCrossLedgerHoldV2_ReplayDiscardsOnlyTheNewIntent(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	organizationID := uuid.New()
	ledgerA := uuid.New()
	ledgerB := uuid.New()
	newGroupID := uuid.New()
	originalGroupID := uuid.New()
	settings := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
	uc := &UseCase{
		TransactionGroupRepo: repo,
		TransactionReader: &atomicTransactionBatchSettingsReader{settingsByRef: map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{
			{organizationID: organizationID, ledgerID: ledgerA}: settings,
			{organizationID: organizationID, ledgerID: ledgerB}: settings,
		}},
		UUIDv7Generator: func() (uuid.UUID, error) { return newGroupID, nil },
		Clock:           func() time.Time { return time.Date(2026, time.September, 22, 18, 0, 0, 0, time.UTC) },
		createAtomicTransactionBatchV2: func(context.Context, CreateAtomicTransactionBatchV2Input) (*CreateAtomicTransactionBatchV2Result, error) {
			return &CreateAtomicTransactionBatchV2Result{
				BatchID:      originalGroupID,
				Transactions: []*transaction.Transaction{{ID: uuid.NewString()}},
				Replayed:     true,
			}, nil
		},
	}
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
	repo.EXPECT().Delete(gomock.Any(), newGroupID).Return(nil)

	result, err := uc.CreateCrossLedgerHoldV2(context.Background(), crossLedgerHoldTestInput(organizationID, ledgerA, ledgerB))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Replayed)
	assert.Equal(t, originalGroupID, result.BatchID)
}

func TestCreateCrossLedgerHoldV2_DiscardsIntentAfterRequestCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	organizationID := uuid.New()
	ledgerA := uuid.New()
	ledgerB := uuid.New()
	groupID := uuid.New()
	settings := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	uc := &UseCase{
		TransactionGroupRepo: repo,
		TransactionReader: &atomicTransactionBatchSettingsReader{settingsByRef: map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{
			{organizationID: organizationID, ledgerID: ledgerA}: settings,
			{organizationID: organizationID, ledgerID: ledgerB}: settings,
		}},
		UUIDv7Generator: func() (uuid.UUID, error) { return groupID, nil },
		Clock:           func() time.Time { return time.Date(2026, time.September, 22, 19, 0, 0, 0, time.UTC) },
		createAtomicTransactionBatchV2: func(context.Context, CreateAtomicTransactionBatchV2Input) (*CreateAtomicTransactionBatchV2Result, error) {
			cancel()
			return nil, markAtomicTransactionBatchPrePublication(context.Canceled)
		},
	}
	repo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)
	repo.EXPECT().Delete(gomock.Any(), groupID).DoAndReturn(func(cleanupCtx context.Context, _ uuid.UUID) error {
		assert.NoError(t, cleanupCtx.Err(), "cleanup must not inherit the request cancellation")

		_, hasDeadline := cleanupCtx.Deadline()
		assert.True(t, hasDeadline, "cleanup must be bounded")

		return errors.New("delete failed")
	})

	result, err := uc.CreateCrossLedgerHoldV2(ctx, crossLedgerHoldTestInput(organizationID, ledgerA, ledgerB))
	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, result)
}

func crossLedgerHoldTestInput(organizationID, ledgerA, ledgerB uuid.UUID) CreateCrossLedgerTransactionV2Input {
	return CreateCrossLedgerTransactionV2Input{
		Transaction: crossLedgerTestTransaction("10",
			[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "10", true)},
			[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "10", false)}),
		Scopes: CrossLedgerTransactionScopes{
			Debits:  []CrossLedgerLegScope{{OrganizationID: organizationID, LedgerID: ledgerA}},
			Credits: []CrossLedgerLegScope{{OrganizationID: organizationID, LedgerID: ledgerB}},
		},
	}
}
