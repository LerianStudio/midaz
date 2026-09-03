// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestUpdateAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		portfolioID    *uuid.UUID
		accountID      uuid.UUID
		input          *mmodel.UpdateAccountInput
		mockSetup      func()
		expectErr      bool
	}{
		{
			name:           "Success - Account updated with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			accountID:      uuid.New(),
			input: &mmodel.UpdateAccountInput{
				Name: "Updated Account",
				Status: mmodel.Status{
					Code: "active",
				},
				SegmentID: nil,
				Metadata:  map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), mmodel.HolderOffV1).
					Return(&mmodel.Account{ID: "123", Type: "internal"}, nil)
				mockAccountRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{ID: "123", Name: "Updated Account", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:           "Error - Account not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			accountID:      uuid.New(),
			input: &mmodel.UpdateAccountInput{
				Name: "Nonexistent Account",
				Status: mmodel.Status{
					Code: "active",
				},
				SegmentID: nil,
				Metadata:  nil,
			},
			mockSetup: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), mmodel.HolderOffV1).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr: true,
		},
		{
			name:           "Error - Failed to update metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			accountID:      uuid.New(),
			input: &mmodel.UpdateAccountInput{
				Name: "Updated Account",
				Status: mmodel.Status{
					Code: "active",
				},
				SegmentID: nil,
				Metadata:  map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), mmodel.HolderOffV1).
					Return(&mmodel.Account{ID: "123", Type: "internal"}, nil)
				mockAccountRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{ID: "123", Name: "Updated Account", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("metadata update error"))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.UpdateAccount(ctx, tt.organizationID, tt.ledgerID, tt.portfolioID, tt.accountID, tt.input, mmodel.HolderOffV1)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.input.Name, result.Name)
				assert.Equal(t, tt.input.Status, result.Status)
			}
		})
	}
}

// TestUpdateAccount_BlockedProvidedTrue covers the blocked=true PATCH end to
// end. The flag no longer rides the generic SET list: it travels the dedicated
// block-state path, so the source-of-truth write, the balance-wide projection
// UPDATE and the cache eviction all happen, and the returned account still
// carries the new state.
//
// The delegation's own guarantees (single event, no-op convergence, no
// propagation when the field is absent) are pinned in
// update_account_block_delegation_test.go.
func TestUpdateAccount_BlockedProvidedTrue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:            mockAccountRepo,
		BalanceRepo:            mockBalanceRepo,
		TransactionRedisRepo:   mockRedisRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	blocked := true

	// Read twice: once by UpdateAccount for the external guard and the merge
	// base, once by the block-state helper it delegates to.
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), mmodel.HolderOffV1).
		Return(&mmodel.Account{ID: accountID.String(), Type: "internal"}, nil).
		Times(2)

	var sawBlockedWrite bool

	mockAccountRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
			if acc.Blocked != nil {
				if !*acc.Blocked {
					t.Fatalf("expected the delegated block write to carry true")
				}

				sawBlockedWrite = true
			}

			return &mmodel.Account{ID: accountID.String(), Name: "Updated Account", Status: mmodel.Status{Code: "active"}, Blocked: acc.Blocked}, nil
		}).
		Times(2)

	mockRedisRepo.EXPECT().
		AddBlockedAccount(gomock.Any(), organizationID, ledgerID, accountID).
		Return(nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		UpdateAccountBlockedByAccountID(gomock.Any(), organizationID, ledgerID, accountID, true).
		Return(nil).
		Times(1)
	mockBalanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return(nil, nil).
		Times(1)

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)
	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	inp := &mmodel.UpdateAccountInput{
		Name:     "Updated Account",
		Status:   mmodel.Status{Code: "active"},
		Metadata: map[string]any{"key": "value"},
		Blocked:  &blocked,
	}

	ctx := context.Background()
	result, err := uc.UpdateAccount(ctx, organizationID, ledgerID, nil, accountID, inp, mmodel.HolderOffV1)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, sawBlockedWrite, "the block transition must reach the account row")

	if result.Blocked == nil || !*result.Blocked {
		t.Fatalf("expected result.Blocked true, got nil/false")
	}
}

// Test that omitting blocked does not send a value to repository (remains nil)
func TestUpdateAccount_BlockedOmitted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), mmodel.HolderOffV1).
		Return(&mmodel.Account{ID: accountID.String(), Type: "internal"}, nil)

	mockAccountRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
			if acc.Blocked != nil {
				t.Fatalf("expected acc.Blocked to be nil when omitted")
			}
			return &mmodel.Account{ID: accountID.String(), Name: "Updated Account"}, nil
		})

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)
	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	inp := &mmodel.UpdateAccountInput{
		Name:     "Updated Account",
		Status:   mmodel.Status{Code: "active"},
		Metadata: map[string]any{"key": "value"},
		// Blocked omitted
	}

	ctx := context.Background()
	result, err := uc.UpdateAccount(ctx, organizationID, ledgerID, nil, accountID, inp, mmodel.HolderOffV1)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// Test that updating an external account is forbidden
func TestUpdateAccount_ExternalForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:            mockAccountRepo,
		OnboardingMetadataRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), mmodel.HolderOffV1).
		Return(&mmodel.Account{ID: accountID.String(), Type: "external"}, nil)

	inp := &mmodel.UpdateAccountInput{Name: "Updated"}
	ctx := context.Background()
	result, err := uc.UpdateAccount(ctx, organizationID, ledgerID, nil, accountID, inp, mmodel.HolderOffV1)

	assert.Error(t, err)
	assert.Nil(t, result)
}
