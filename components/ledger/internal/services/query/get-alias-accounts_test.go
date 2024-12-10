package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mpointers"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestListAccountsByAlias(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	aliases := []string{"alias1", "alias2"}

	tests := []struct {
		name             string
		setupMocks       func()
		expectedErr      error
		expectedAccounts []*mmodel.Account
	}{
		{
			name: "success - accounts retrieved",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, aliases).
					Return([]*mmodel.Account{
						{ID: uuid.New().String(), Alias: mpointers.String("alias1")},
						{ID: uuid.New().String(), Alias: mpointers.String("alias2")},
					}, nil).
					Times(1)
			},
			expectedErr: nil,
			expectedAccounts: []*mmodel.Account{
				{ID: uuid.New().String(), Alias: mpointers.String("alias1")},
				{ID: uuid.New().String(), Alias: mpointers.String("alias2")},
			},
		},
		{
			name: "failure - accounts not found",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, aliases).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr:      errors.New("The accounts could not be retrieved using the specified aliases. Please verify the aliases for accuracy and try again."),
			expectedAccounts: nil,
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, aliases).
					Return(nil, errors.New("failed to retrieve accounts")).
					Times(1)
			},
			expectedErr:      errors.New("failed to retrieve accounts"),
			expectedAccounts: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.ListAccountsByAlias(ctx, organizationID, ledgerID, aliases)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, len(tt.expectedAccounts), len(result))
				for i, account := range result {
					assert.Equal(t, tt.expectedAccounts[i].Alias, account.Alias)
				}
			}
		})
	}
}
