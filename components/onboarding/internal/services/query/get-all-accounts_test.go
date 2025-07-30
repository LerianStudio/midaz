package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"testing"
)

func TestGetAllAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:  mockAccountRepo,
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolioID := uuid.New()

	filter := http.QueryHeader{
		Limit: 10,
		Page:  1,
	}

	tests := []struct {
		name             string
		setupMocks       func()
		expectedErr      error
		expectedAccounts []*mmodel.Account
	}{
		{
			name: "success - accounts retrieved with metadata",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, &portfolioID, filter.ToOffsetPagination()).
					Return([]*mmodel.Account{
						{ID: "acc1"},
						{ID: "acc2"},
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", filter).
					Return([]*mongodb.Metadata{
						{EntityID: "acc1", Data: map[string]any{"key1": "value1"}},
						{EntityID: "acc2", Data: map[string]any{"key2": "value2"}},
					}, nil).
					Times(1)
			},
			expectedErr: nil,
			expectedAccounts: []*mmodel.Account{
				{ID: "acc1", Metadata: map[string]any{"key1": "value1"}},
				{ID: "acc2", Metadata: map[string]any{"key2": "value2"}},
			},
		},
		{
			name: "failure - accounts not found",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, &portfolioID, filter.ToOffsetPagination()).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr:      errors.New("No accounts were found in the search. Please review the search criteria and try again."),
			expectedAccounts: nil,
		},
		{
			name: "failure - error retrieving accounts",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, &portfolioID, filter.ToOffsetPagination()).
					Return(nil, errors.New("failed to retrieve accounts")).
					Times(1)
			},
			expectedErr:      errors.New("failed to retrieve accounts"),
			expectedAccounts: nil,
		},
		{
			name: "failure - metadata retrieval error",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, &portfolioID, filter.ToOffsetPagination()).
					Return([]*mmodel.Account{
						{ID: "acc1"},
						{ID: "acc2"},
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					FindList(gomock.Any(), "Account", filter).
					Return(nil, errors.New("failed to retrieve metadata")).
					Times(1)
			},
			expectedErr:      errors.New("No accounts were found in the search. Please review the search criteria and try again."),
			expectedAccounts: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.GetAllAccount(ctx, organizationID, ledgerID, &portfolioID, filter)

			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, len(tt.expectedAccounts), len(result))
				for i, account := range result {
					assert.Equal(t, tt.expectedAccounts[i].ID, account.ID)
					assert.Equal(t, tt.expectedAccounts[i].Metadata, account.Metadata)
				}
			}
		})
	}
}
