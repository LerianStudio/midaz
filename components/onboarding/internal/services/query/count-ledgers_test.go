package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCountLedgers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)
	organizationID := uuid.New()

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	tests := []struct {
		name           string
		mockSetup      func()
		expectErr      bool
		expectedError  error
		expectedResult int64
	}{
		{
			name: "Success - Count ledgers",
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					Count(gomock.Any(), organizationID).
					Return(int64(42), nil)
			},
			expectErr:      false,
			expectedResult: 42,
		},
		{
			name: "Error - No ledgers found",
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					Count(gomock.Any(), organizationID).
					Return(int64(0), services.ErrDatabaseItemNotFound)
			},
			expectErr:     true,
			expectedError: pkg.ValidateBusinessError(constant.ErrNoLedgersFound, "Ledger"),
		},
		{
			name: "Error - Database error",
			mockSetup: func() {
				mockLedgerRepo.EXPECT().
					Count(gomock.Any(), organizationID).
					Return(int64(0), errors.New("database error"))
			},
			expectErr:      true,
			expectedResult: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			result, err := uc.CountLedgers(context.Background(), organizationID)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectedError != nil {
					assert.Equal(t, tt.expectedError.Error(), err.Error())
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}
