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
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	written := expectAccountConfigurationWrite(t, mockRedisRepo, organizationID, ledgerID, []string{"@acc#default"}, txRedis.BlobCopyWritten)

	mockAccountRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
			if acc.Blocked == nil || !*acc.Blocked {
				t.Fatalf("expected acc.Blocked to be true and non-nil")
			}
			// Echo back the written row with its configuration version.
			return &mmodel.Account{ID: accountID.String(), Name: "Updated Account", Status: mmodel.Status{Code: "active"}, Blocked: acc.Blocked, ConfigVersion: 4}, nil
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

	assert.Equal(t, accounting.AccountConfig{AccountID: accountID, Version: 4, Blocked: true}, *written,
		"the account key carries the written row and its version")
}

// expectAccountConfigurationWrite expects the account configuration key write and
// then the copy into the blobs cached under refs, both with the same configuration,
// and returns that configuration once the key write has seen it.
func expectAccountConfigurationWrite(t *testing.T, redisRepo *txRedis.MockRedisRepository, organizationID, ledgerID uuid.UUID, refs []string, outcome txRedis.BlobCopyOutcome) *accounting.AccountConfig {
	t.Helper()

	written := &accounting.AccountConfig{}

	redisRepo.EXPECT().
		WriteAccountConfig(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, config accounting.AccountConfig) (accounting.AccountConfig, txRedis.ConfigWriteOutcome, error) {
			*written = config

			return config, txRedis.ConfigWritten, nil
		}).
		Times(1)

	redisRepo.EXPECT().
		CopyAccountConfigToBlobs(gomock.Any(), organizationID, ledgerID, refs, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ uuid.UUID, refs []string, config accounting.AccountConfig) (map[string]txRedis.BlobCopyOutcome, error) {
			assert.Equal(t, *written, config, "the blob copy must carry the configuration written to the key, after it")

			outcomes := make(map[string]txRedis.BlobCopyOutcome, len(refs))
			for _, ref := range refs {
				outcomes[ref] = outcome
			}

			return outcomes, nil
		}).
		Times(1)

	return written
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

// TestUpdateAccount_BlockedConfigurationWrite covers the configuration write of
// the Blocked flag after the PostgreSQL update: one account key write, then one
// ListByAccountID and one copy into the account's cached balance blobs when the
// PATCH carries blocked, zero cache work when it does not, and fail-closed
// posture (a listing or Redis failure, or a copy left unconfirmed, fails the
// PATCH even though PostgreSQL already committed — Blocked is a security
// control, and the retry is idempotent).
func TestUpdateAccount_BlockedConfigurationWrite(t *testing.T) {
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
			Return(&mmodel.Account{ID: accountID.String(), Name: "acc", Status: mmodel.Status{Code: "active"}, Blocked: blocked, ConfigVersion: 2}, nil)
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

	t.Run("block patch writes the account key and copies into every cached balance", func(t *testing.T) {
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
		// with the explicit default balance: the copy must be deduped.
		written := expectAccountConfigurationWrite(t, mockRedisRepo, organizationID, ledgerID,
			[]string{"@acc#default", "@acc#savings"}, txRedis.BlobCopyWritten)

		result, err := uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
			&mmodel.UpdateAccountInput{Blocked: boolPtr(true)}, mmodel.HolderOffV1)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, accounting.AccountConfig{AccountID: accountID, Version: 2, Blocked: true}, *written)
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

	errRedis := errors.New("redis unavailable")

	for _, tc := range []struct {
		name     string
		writeErr error
		outcome  txRedis.BlobCopyOutcome
		copyErr  error
		wantErr  error
	}{
		{name: "account key write failure", writeErr: errRedis, wantErr: errRedis},
		{name: "blob copy failure", copyErr: errRedis, wantErr: errRedis},
		{name: "blob copy left unconfirmed", outcome: txRedis.BlobCopyConflict, wantErr: errConfigurationCopyNotConfirmed},
	} {
		t.Run(tc.name+" fails closed and returns the error", func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			uc, mockAccountRepo, mockBalanceRepo, mockRedisRepo, mockMetadataRepo := newMocks(ctrl)

			expectFindAndUpdate(mockAccountRepo, boolPtr(false))
			expectMetadata(mockMetadataRepo)

			mockRedisRepo.EXPECT().
				WriteAccountConfig(gomock.Any(), organizationID, ledgerID, gomock.Any()).
				Return(accounting.AccountConfig{}, txRedis.ConfigWritten, tc.writeErr).
				Times(1)

			if tc.writeErr == nil {
				mockBalanceRepo.EXPECT().
					ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
					Return([]*mmodel.Balance{{ID: uuid.New().String(), Alias: "@acc", Key: "default"}}, nil).
					Times(1)
				mockRedisRepo.EXPECT().
					CopyAccountConfigToBlobs(gomock.Any(), organizationID, ledgerID, []string{"@acc#default"}, gomock.Any()).
					Return(map[string]txRedis.BlobCopyOutcome{"@acc#default": tc.outcome}, tc.copyErr).
					Times(1)
			}

			result, err := uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
				&mmodel.UpdateAccountInput{Blocked: boolPtr(false)}, mmodel.HolderOffV1)
			require.Error(t, err, "PostgreSQL already committed, but transactions would honor a stale flag; the request must fail so the caller retries")
			assert.ErrorIs(t, err, tc.wantErr)
			assert.False(t, pkg.IsBusinessError(err), "a cache failure is a technical error")
			assert.Nil(t, result)
		})
	}

	t.Run("blobs of another balance leave nothing to confirm", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc, mockAccountRepo, mockBalanceRepo, mockRedisRepo, mockMetadataRepo := newMocks(ctrl)

		expectFindAndUpdate(mockAccountRepo, boolPtr(true))
		expectMetadata(mockMetadataRepo)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return([]*mmodel.Balance{{ID: uuid.New().String(), Alias: "@acc", Key: "default"}}, nil).
			Times(1)
		expectAccountConfigurationWrite(t, mockRedisRepo, organizationID, ledgerID, []string{"@acc#default"}, txRedis.BlobCopyOfAnotherBalance)

		result, err := uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
			&mmodel.UpdateAccountInput{Blocked: boolPtr(true)}, mmodel.HolderOffV1)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("balance listing failure fails closed and returns the error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc, mockAccountRepo, mockBalanceRepo, mockRedisRepo, mockMetadataRepo := newMocks(ctrl)

		expectFindAndUpdate(mockAccountRepo, boolPtr(true))
		expectMetadata(mockMetadataRepo)

		mockRedisRepo.EXPECT().
			WriteAccountConfig(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(accounting.AccountConfig{}, txRedis.ConfigWritten, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			ListByAccountID(gomock.Any(), organizationID, ledgerID, accountID).
			Return(nil, errors.New("db unavailable")).
			Times(1)

		result, err := uc.UpdateAccount(context.Background(), organizationID, ledgerID, nil, accountID,
			&mmodel.UpdateAccountInput{Blocked: boolPtr(true)}, mmodel.HolderOffV1)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("account with no balances writes the key and skips the copy", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc, mockAccountRepo, mockBalanceRepo, mockRedisRepo, mockMetadataRepo := newMocks(ctrl)

		expectFindAndUpdate(mockAccountRepo, boolPtr(true))
		expectMetadata(mockMetadataRepo)

		mockRedisRepo.EXPECT().
			WriteAccountConfig(gomock.Any(), organizationID, ledgerID, gomock.Any()).
			Return(accounting.AccountConfig{}, txRedis.ConfigWritten, nil).
			Times(1)

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
