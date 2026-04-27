// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestVerifyAccountsTransactable_RepoError_PropagatesUnwrapped covers the infrastructure
// failure path: ListAccountsByIDs returns a generic error, and the caller must receive
// the same error untouched. Wrapping at this layer would lose the underlying cause for
// incident response.
func TestVerifyAccountsTransactable_RepoError_PropagatesUnwrapped(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	ids := []uuid.UUID{uuid.New()}

	boom := errors.New("driver: connection reset by peer")

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		ListAccountsByIDs(gomock.Any(), organizationID, ledgerID, ids).
		Return(nil, boom).
		Times(1)

	uc := &UseCase{AccountRepo: mockAccountRepo}

	err := uc.VerifyAccountsTransactable(context.Background(), organizationID, ledgerID, ids)

	require.ErrorIs(t, err, boom, "infra errors must surface with the original cause for incident response")
}

// TestVerifyAccountsTransactable_NilAccountInSlice_Skipped exercises the defensive
// `if acc == nil { continue }` branch. A repository that returns a sparse slice (rare
// but possible if a join short-circuits on a deleted row) must not crash the gate.
func TestVerifyAccountsTransactable_NilAccountInSlice_Skipped(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	ids := []uuid.UUID{uuid.New(), uuid.New()}

	falseVal := false

	// First entry is nil (defensive skip), second is a valid ACTIVE account so the
	// overall result is no-error.
	accounts := []*mmodel.Account{
		nil,
		{
			ID:             ids[1].String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status:         mmodel.Status{Code: constant.AccountStatusActive},
			Blocked:        &falseVal,
		},
	}

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		ListAccountsByIDs(gomock.Any(), organizationID, ledgerID, ids).
		Return(accounts, nil).
		Times(1)

	uc := &UseCase{AccountRepo: mockAccountRepo}

	err := uc.VerifyAccountsTransactable(context.Background(), organizationID, ledgerID, ids)
	assert.NoError(t, err, "nil entries must be skipped, not panic")
}

// TestVerifyAccountsTransactable_BlockedNilPointerTreatedAsFalse asserts that an account
// with Blocked == nil (rather than &false) is treated as not blocked. The check uses
// `acc.Blocked != nil && *acc.Blocked`, so a nil pointer is the not-blocked path.
func TestVerifyAccountsTransactable_BlockedNilPointerTreatedAsFalse(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	ids := []uuid.UUID{uuid.New()}

	accounts := []*mmodel.Account{
		{
			ID:             ids[0].String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status:         mmodel.Status{Code: constant.AccountStatusActive},
			Blocked:        nil, // explicitly nil
		},
	}

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockAccountRepo.EXPECT().
		ListAccountsByIDs(gomock.Any(), organizationID, ledgerID, ids).
		Return(accounts, nil).
		Times(1)

	uc := &UseCase{AccountRepo: mockAccountRepo}

	err := uc.VerifyAccountsTransactable(context.Background(), organizationID, ledgerID, ids)
	assert.NoError(t, err, "Blocked=nil must be treated as not blocked, not as ineligible")
}
