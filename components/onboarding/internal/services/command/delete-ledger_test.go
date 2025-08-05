package command

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
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
			expectedErr: errors.New("The provided ledger ID does not exist in our records. Please verify the ledger ID and try again."),
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockLedgerRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID).
					Return(errors.New("failed to delete ledger")).
					Times(1)
			},
			expectedErr: errors.New("failed to delete ledger"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := uc.DeleteLedgerByID(ctx, organizationID, ledgerID)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
