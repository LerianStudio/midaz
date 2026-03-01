// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
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
						{ID: uuid.New().String(), Alias: libPointers.String("alias1")},
						{ID: uuid.New().String(), Alias: libPointers.String("alias2")},
					}, nil).
					Times(1)
			},
			expectedErr: nil,
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
			expectedErr:      errAccountsNotRetrievedByAliases,
			expectedAccounts: nil,
		},
		{
			name: "failure - repository error",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					ListAccountsByAlias(gomock.Any(), organizationID, ledgerID, aliases).
					Return(nil, errFailedToRetrieveAccounts).
					Times(1)
			},
			expectedErr:      errFailedToRetrieveAccounts,
			expectedAccounts: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.ListAccountsByAlias(ctx, organizationID, ledgerID, aliases)

			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErr.Error())
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result, len(tt.expectedAccounts))

				for i, account := range result {
					assert.Equal(t, tt.expectedAccounts[i].Alias, account.Alias)
				}
			}
		})
	}
}
