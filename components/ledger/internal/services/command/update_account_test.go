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
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
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

// Test updating blocked flag when provided (true)
func TestUpdateAccount_BlockedProvidedTrue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRedisRepo := txRedis.NewMockRedisRepository(ctrl)
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

	// Expectations
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), mmodel.HolderOffV1).
		Return(&mmodel.Account{ID: accountID.String(), Type: "internal"}, nil)

	mockBalanceRepo.EXPECT().
		ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
		Return([]*mmodel.Balance{{ID: uuid.New().String(), Alias: "@acc", Key: "default"}}, nil)

	mockRedisRepo.EXPECT().
		UpdateBalanceCacheBlocked(gomock.Any(), organizationID, ledgerID, []string{"@acc#default"}, true).
		Return(nil)

	mockAccountRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
			if acc.Blocked == nil || !*acc.Blocked {
				t.Fatalf("expected acc.Blocked to be true and non-nil")
			}
			// Echo back
			return &mmodel.Account{ID: accountID.String(), Name: "Updated Account", Status: mmodel.Status{Code: "active"}, Blocked: acc.Blocked}, nil
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
		Blocked:  &blocked,
	}

	ctx := context.Background()
	result, err := uc.UpdateAccount(ctx, organizationID, ledgerID, nil, accountID, inp, mmodel.HolderOffV1)

	assert.NoError(t, err)
	assert.NotNil(t, result)
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

// TestUpdateAccount_BlockedCachePropagation covers the in-place rewrite of the
// Blocked flag on the account's cached balance blobs after the PostgreSQL
// update: one ListByAccountID + one atomic multi-key rewrite when the PATCH
// carries blocked, zero cache work when it does not, and best-effort posture
// (a Redis failure never fails the request — PostgreSQL is the source of
// truth and hydration/TTL heal the cache).
func TestUpdateAccount_BlockedCachePropagation(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	newMocks := func(ctrl *gomock.Controller) (*UseCase, *account.MockRepository, *balance.MockRepository, *txRedis.MockRedisRepository, *mongodb.MockRepository) {
		mockAccountRepo := account.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRedisRepo := txRedis.NewMockRedisRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)

		return &UseCase{
			AccountRepo:            mockAccountRepo,
			BalanceRepo:            mockBalanceRepo,
			TransactionRedisRepo:   mockRedisRepo,
			OnboardingMetadataRepo: mockMetadataRepo,
		}, mockAccountRepo, mockBalanceRepo, mockRedisRepo, mockMetadataRepo
	}

	expectFindAndUpdate := func(mockAccountRepo *account.MockRepository, blocked *bool) {
		mockAccountRepo.EXPECT().
			Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), mmodel.HolderOffV1).
			Return(&mmodel.Account{ID: accountID.String(), Type: "internal"}, nil)
		mockAccountRepo.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&mmodel.Account{ID: accountID.String(), Name: "acc", Status: mmodel.Status{Code: "active"}, Blocked: blocked}, nil)
	}

	expectMetadata := func(mockMetadataRepo *mongodb.MockRepository) {
		mockMetadataRepo.EXPECT().
			FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()
		mockMetadataRepo.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()
	}

	boolPtr := func(b bool) *bool { return &b }

	t.Run("block patch rewrites every cached balance of the account", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc, mockAccountRepo, mockBalanceRepo, mockRedisRepo, mockMetadataRepo := newMocks(ctrl)

		expectFindAndUpdate(mockAccountRepo, boolPtr(true))
		expectMetadata(mockMetadataRepo)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{
				{ID: uuid.New().String(), Alias: "@acc", Key: "default"},
				{ID: uuid.New().String(), Alias: "@acc", Key: "savings"},
				{ID: uuid.New().String(), Alias: "@acc", Key: ""}, // legacy row without key
			}, nil).
			Times(1)

		// The legacy empty-key row normalizes to "@acc#default" and collides
		// with the explicit default balance: the rewrite call must be deduped.
		mockRedisRepo.EXPECT().
			UpdateBalanceCacheBlocked(gomock.Any(), organizationID, ledgerID,
				[]string{"@acc#default", "@acc#savings"}, true).
			Return(nil).
			Times(1)

		result, err := uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
			&mmodel.UpdateAccountInput{Blocked: boolPtr(true)}, mmodel.HolderOffV1)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("patch without blocked performs zero cache work", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// No EXPECT on BalanceRepo / TransactionRedisRepo: any call fails the test.
		uc, mockAccountRepo, _, _, mockMetadataRepo := newMocks(ctrl)

		expectFindAndUpdate(mockAccountRepo, nil)
		expectMetadata(mockMetadataRepo)

		result, err := uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
			&mmodel.UpdateAccountInput{Name: "renamed"}, mmodel.HolderOffV1)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("cache rewrite failure is best-effort and never fails the request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc, mockAccountRepo, mockBalanceRepo, mockRedisRepo, mockMetadataRepo := newMocks(ctrl)

		expectFindAndUpdate(mockAccountRepo, boolPtr(false))
		expectMetadata(mockMetadataRepo)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{{ID: uuid.New().String(), Alias: "@acc", Key: "default"}}, nil).
			Times(1)

		mockRedisRepo.EXPECT().
			UpdateBalanceCacheBlocked(gomock.Any(), organizationID, ledgerID, []string{"@acc#default"}, false).
			Return(errors.New("redis unavailable")).
			Times(1)

		result, err := uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
			&mmodel.UpdateAccountInput{Blocked: boolPtr(false)}, mmodel.HolderOffV1)
		assert.NoError(t, err, "PostgreSQL is the source of truth; the cache rewrite is best-effort")
		assert.NotNil(t, result)
	})

	t.Run("balance listing failure is best-effort and never fails the request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc, mockAccountRepo, mockBalanceRepo, _, mockMetadataRepo := newMocks(ctrl)

		expectFindAndUpdate(mockAccountRepo, boolPtr(true))
		expectMetadata(mockMetadataRepo)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return(nil, errors.New("db unavailable")).
			Times(1)

		result, err := uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
			&mmodel.UpdateAccountInput{Blocked: boolPtr(true)}, mmodel.HolderOffV1)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("account with no balances skips the rewrite call", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc, mockAccountRepo, mockBalanceRepo, _, mockMetadataRepo := newMocks(ctrl)

		expectFindAndUpdate(mockAccountRepo, boolPtr(true))
		expectMetadata(mockMetadataRepo)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{}, nil).
			Times(1)

		result, err := uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
			&mmodel.UpdateAccountInput{Blocked: boolPtr(true)}, mmodel.HolderOffV1)
		assert.NoError(t, err)
		assert.NotNil(t, result)
	})
}
