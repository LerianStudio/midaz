package query

import (
	"context"
	"errors"
	"testing"

	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
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
		name              string
		setupMocks        func()
		expectBusinessErr bool
		expectInternalErr bool
		expectedAccounts  []*mmodel.Account
	}{
		{
			name: "success - accounts retrieved",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, aliases).
					Return([]*mmodel.Account{
						{ID: uuid.New().String(), Alias: libPointers.String("alias1")},
						{ID: uuid.New().String(), Alias: libPointers.String("alias2")},
					}, nil).
					Times(1)
			},
			expectBusinessErr: false,
			expectInternalErr: false,
			expectedAccounts: []*mmodel.Account{
				{ID: uuid.New().String(), Alias: libPointers.String("alias1")},
				{ID: uuid.New().String(), Alias: libPointers.String("alias2")},
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
			expectBusinessErr: true,
			expectInternalErr: false,
			expectedAccounts:  nil,
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, aliases).
					Return(nil, errors.New("failed to retrieve accounts")).
					Times(1)
			},
			expectBusinessErr: false,
			expectInternalErr: true,
			expectedAccounts:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.ListAccountsByAlias(ctx, organizationID, ledgerID, aliases)

			if tt.expectInternalErr {
				assert.Error(t, err)
				var internalErr pkg.InternalServerError
				assert.True(t, errors.As(err, &internalErr), "expected InternalServerError type")
				assert.Nil(t, result)
			} else if tt.expectBusinessErr {
				assert.Error(t, err)
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
