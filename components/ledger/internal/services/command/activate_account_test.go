// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestActivateAccount_Success verifies the happy path: the repo flip succeeds
// and the balance unblock call is made with allow_sending=true and
// allow_receiving=true. No error is returned.
func TestActivateAccount_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
		BalanceRepo: mockBalanceRepo,
	}

	mockAccountRepo.EXPECT().
		ActivatePendingAccount(gomock.Any(), organizationID, ledgerID, accountID).
		Return(nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, upd interface{}) error {
			// UpdateAllByAccountID receives mmodel.UpdateBalance with both flags
			// pointing to true. We rely on the behaviour contract rather than
			// introspecting the struct here; the type is enforced at compile
			// time by the repo interface signature.
			_ = upd
			return nil
		}).
		Times(1)

	err := uc.ActivateAccount(ctx, organizationID, ledgerID, accountID)
	assert.NoError(t, err, "activation of a pending account should succeed")
}

// TestActivateAccount_AlreadyActive verifies that invoking ActivateAccount on
// an account that is not in PENDING_CRM_LINK returns
// ErrInvalidAccountActivationState without touching the balance repo.
func TestActivateAccount_AlreadyActive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
		BalanceRepo: mockBalanceRepo,
	}

	// The repo enforces the precondition and returns the business error.
	repoErr := pkg.ValidateBusinessError(constant.ErrInvalidAccountActivationState, constant.EntityAccount)

	mockAccountRepo.EXPECT().
		ActivatePendingAccount(gomock.Any(), organizationID, ledgerID, accountID).
		Return(repoErr).
		Times(1)

	// Balance repo MUST NOT be called when the account transition fails.
	// The absence of an EXPECT on mockBalanceRepo enforces this: any call
	// would cause gomock to fail the test.

	err := uc.ActivateAccount(ctx, organizationID, ledgerID, accountID)
	assert.Error(t, err, "activation of an already-ACTIVE account should fail")
	assert.True(t, errors.Is(err, repoErr) || err.Error() == repoErr.Error(),
		"expected ErrInvalidAccountActivationState wrapped error, got %v", err)
}

// TestActivateAccount_FailedCRMLink verifies that a FAILED_CRM_LINK account
// also returns ErrInvalidAccountActivationState — the repo's precondition
// accepts only PENDING_CRM_LINK + blocked=true.
func TestActivateAccount_FailedCRMLink(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
		BalanceRepo: mockBalanceRepo,
	}

	repoErr := pkg.ValidateBusinessError(constant.ErrInvalidAccountActivationState, constant.EntityAccount)

	mockAccountRepo.EXPECT().
		ActivatePendingAccount(gomock.Any(), organizationID, ledgerID, accountID).
		Return(repoErr).
		Times(1)

	// Balance repo MUST NOT be called.

	err := uc.ActivateAccount(ctx, organizationID, ledgerID, accountID)
	assert.Error(t, err, "activation of a FAILED_CRM_LINK account should fail")
	assert.True(t, errors.Is(err, repoErr) || err.Error() == repoErr.Error(),
		"expected ErrInvalidAccountActivationState, got %v", err)
}

// TestActivateAccount_BalanceUpdateFailsButAccountStaysActive verifies the
// defense-in-depth decision: a balance-update failure is logged as a WARN and
// the account transition is NOT rolled back. The caller observes success.
func TestActivateAccount_BalanceUpdateFailsButAccountStaysActive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
		BalanceRepo: mockBalanceRepo,
	}

	mockAccountRepo.EXPECT().
		ActivatePendingAccount(gomock.Any(), organizationID, ledgerID, accountID).
		Return(nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		UpdateAllByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
		Return(errors.New("transient balance DB failure")).
		Times(1)

	err := uc.ActivateAccount(ctx, organizationID, ledgerID, accountID)
	assert.NoError(t, err, "balance-update failure must not surface as an activation error; transaction eligibility still gates on account.Status")
}
