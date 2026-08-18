// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
)

func TestCompleteRevertClaim_AdoptsOldBackupAndNeverOverwritesAnotherReverse(t *testing.T) {
	t.Parallel()

	t.Run("old backup is adopted then completed", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		repo := revertclaim.NewMockRepository(ctrl)
		organizationID := uuid.New()
		ledgerID := uuid.New()
		originID := uuid.New()
		reverseID := uuid.New()
		legacyFenceKey := "legacy-fence-key"
		claim := &revertclaim.Claim{
			OrganizationID:       organizationID,
			LedgerID:             ledgerID,
			OriginTransactionID:  originID,
			ReverseTransactionID: reverseID,
			State:                revertclaim.StateClaimed,
		}

		repo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID,
			&legacyFenceKey, nil).Return(claim, true, nil)
		repo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID,
			revertclaim.StateCompleted, nil).Return(nil)

		uc := &UseCase{RevertClaimRepo: repo}
		require.NoError(t, uc.CompleteRevertClaim(context.Background(), organizationID, ledgerID, originID,
			reverseID, &legacyFenceKey, nil))
	})

	t.Run("mismatched durable reservation is reconciliation not overwrite", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		repo := revertclaim.NewMockRepository(ctrl)
		organizationID := uuid.New()
		ledgerID := uuid.New()
		originID := uuid.New()
		backupReverseID := uuid.New()
		reservedReverseID := uuid.New()
		claim := &revertclaim.Claim{
			OrganizationID:       organizationID,
			LedgerID:             ledgerID,
			OriginTransactionID:  originID,
			ReverseTransactionID: reservedReverseID,
			State:                revertclaim.StateReconciliationRequired,
		}

		repo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, backupReverseID, nil, nil).Return(claim, false, nil)

		uc := &UseCase{RevertClaimRepo: repo}
		err := uc.CompleteRevertClaim(context.Background(), organizationID, ledgerID, originID, backupReverseID, nil, nil)
		require.ErrorContains(t, err, reservedReverseID.String())
		require.ErrorContains(t, err, backupReverseID.String())
	})
}
