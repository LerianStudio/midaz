// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
)

var (
	errDeleteLedger   = errors.New("failed to delete ledger")
	errLedgerNotFound = errors.New("The provided ledger ID does not exist in our records. Please verify the ledger ID and try again.") //nolint:revive,staticcheck // business error message
)

func TestDeleteLedgerByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name        string
		setupMocks  func()
		expectedErr error
	}{
		{
			name: "success - ledger deleted",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name: "failure - ledger not found",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID).
					Return(services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr: errLedgerNotFound,
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID).
					Return(errDeleteLedger).
					Times(1)
			},
			expectedErr: errDeleteLedger,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteLedgerByID(ctx, organizationID, ledgerID)

			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}
