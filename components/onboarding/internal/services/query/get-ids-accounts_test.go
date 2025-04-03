package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// \1 performs an operation
func TestListAccountsByIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		accountIDs     []uuid.UUID
		mockSetup      func()
		expectErr      bool
		expectedResult []*mmodel.Account
	}{
		{
			name:           "Success - Retrieve accounts by IDs",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			accountIDs:     []uuid.UUID{uuid.New(), uuid.New()},
			mockSetup: func() {
				mockAccountRepo.EXPECT().
					ListAccountsByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]*mmodel.Account{
						{ID: "account-1", Name: "Account 1", Status: mmodel.Status{Code: "active"}},
						{ID: "account-2", Name: "Account 2", Status: mmodel.Status{Code: "inactive"}},
					}, nil)
			},
			expectErr: false,
			expectedResult: []*mmodel.Account{
				{ID: "account-1", Name: "Account 1", Status: mmodel.Status{Code: "active"}},
				{ID: "account-2", Name: "Account 2", Status: mmodel.Status{Code: "inactive"}},
			},
		},
		{
			name:           "Error - Accounts not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			accountIDs:     []uuid.UUID{uuid.New(), uuid.New()},
			mockSetup: func() {
				mockAccountRepo.EXPECT().
					ListAccountsByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Error - Database error",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			accountIDs:     []uuid.UUID{uuid.New(), uuid.New()},
			mockSetup: func() {
				mockAccountRepo.EXPECT().
					ListAccountsByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("database error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.ListAccountsByIDs(ctx, tt.organizationID, tt.ledgerID, tt.accountIDs)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}
