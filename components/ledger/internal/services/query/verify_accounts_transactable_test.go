// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestVerifyAccountsTransactable(t *testing.T) {
	organizationID := uuid.New()
	ledgerID := uuid.New()

	falseVal := false
	trueVal := true

	accountActive := func(id uuid.UUID) *mmodel.Account {
		return &mmodel.Account{
			ID:             id.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status:         mmodel.Status{Code: constant.AccountStatusActive},
			Blocked:        &falseVal,
		}
	}

	accountPending := func(id uuid.UUID) *mmodel.Account {
		return &mmodel.Account{
			ID:             id.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status:         mmodel.Status{Code: constant.AccountStatusPendingCRMLink},
			Blocked:        &trueVal,
		}
	}

	accountFailed := func(id uuid.UUID) *mmodel.Account {
		return &mmodel.Account{
			ID:             id.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status:         mmodel.Status{Code: constant.AccountStatusFailedCRMLink},
			Blocked:        &trueVal,
		}
	}

	accountBlocked := func(id uuid.UUID) *mmodel.Account {
		return &mmodel.Account{
			ID:             id.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status:         mmodel.Status{Code: constant.AccountStatusActive},
			Blocked:        &trueVal,
		}
	}

	tests := []struct {
		name        string
		buildIDs    func() []uuid.UUID
		mockAccount func(id uuid.UUID) *mmodel.Account
		expectErr   string
	}{
		{
			name:        "active account passes",
			buildIDs:    func() []uuid.UUID { return []uuid.UUID{uuid.New()} },
			mockAccount: accountActive,
			expectErr:   "",
		},
		{
			name:        "pending CRM link account rejected",
			buildIDs:    func() []uuid.UUID { return []uuid.UUID{uuid.New()} },
			mockAccount: accountPending,
			expectErr:   constant.ErrAccountStatusTransactionRestriction.Error(),
		},
		{
			name:        "failed CRM link account rejected",
			buildIDs:    func() []uuid.UUID { return []uuid.UUID{uuid.New()} },
			mockAccount: accountFailed,
			expectErr:   constant.ErrAccountStatusTransactionRestriction.Error(),
		},
		{
			name:        "active but blocked account rejected",
			buildIDs:    func() []uuid.UUID { return []uuid.UUID{uuid.New()} },
			mockAccount: accountBlocked,
			expectErr:   constant.ErrAccountStatusTransactionRestriction.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockAccountRepo := account.NewMockRepository(ctrl)
			uc := &UseCase{AccountRepo: mockAccountRepo}

			ids := tt.buildIDs()
			acc := tt.mockAccount(ids[0])

			mockAccountRepo.EXPECT().
				ListAccountsByIDs(gomock.Any(), organizationID, ledgerID, ids).
				Return([]*mmodel.Account{acc}, nil).
				Times(1)

			err := uc.VerifyAccountsTransactable(context.Background(), organizationID, ledgerID, ids)
			if tt.expectErr == "" {
				assert.NoError(t, err)
				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectErr)
		})
	}
}

func TestVerifyAccountsTransactable_EmptyIDsIsNoop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	uc := &UseCase{AccountRepo: mockAccountRepo}

	// Repo MUST NOT be called when ids is empty. Any call fails the test.

	err := uc.VerifyAccountsTransactable(context.Background(), uuid.New(), uuid.New(), nil)
	assert.NoError(t, err)
}

func TestVerifyAccountsTransactable_AllActiveAccountsPassEvenWithMultiple(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()

	falseVal := false
	ids := []uuid.UUID{uuid.New(), uuid.New()}

	accounts := []*mmodel.Account{
		{
			ID:             ids[0].String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status:         mmodel.Status{Code: constant.AccountStatusActive},
			Blocked:        &falseVal,
		},
		{
			ID:             ids[1].String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status:         mmodel.Status{Code: constant.AccountStatusActive},
			Blocked:        &falseVal,
		},
	}

	mockAccountRepo := account.NewMockRepository(ctrl)
	uc := &UseCase{AccountRepo: mockAccountRepo}

	mockAccountRepo.EXPECT().
		ListAccountsByIDs(gomock.Any(), organizationID, ledgerID, ids).
		Return(accounts, nil).
		Times(1)

	err := uc.VerifyAccountsTransactable(context.Background(), organizationID, ledgerID, ids)
	assert.NoError(t, err, "regression guard: two ACTIVE accounts must not trigger the gate")
}
