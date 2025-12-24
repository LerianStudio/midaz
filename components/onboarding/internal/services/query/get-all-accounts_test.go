package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
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
		name              string
		setupMocks        func()
		expectBusinessErr bool
		expectInternalErr bool
		expectedAccounts  []*mmodel.Account
	}{
		{
			name: "success - accounts retrieved with metadata",
			setupMocks: func() {
				bFalse := false
				bTrue := true
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, &portfolioID, filter.ToOffsetPagination()).
					Return([]*mmodel.Account{
						{ID: "acc1", Blocked: &bFalse},
						{ID: "acc2", Blocked: &bTrue},
					}, nil).
					Times(1)

				mockMetadataRepo.EXPECT().
					FindByEntityIDs(gomock.Any(), "Account", []string{"acc1", "acc2"}).
					Return([]*mongodb.Metadata{
						{EntityID: "acc1", Data: map[string]any{"key1": "value1"}},
						{EntityID: "acc2", Data: map[string]any{"key2": "value2"}},
					}, nil).
					Times(1)
			},
			expectBusinessErr: false,
			expectInternalErr: false,
			expectedAccounts: []*mmodel.Account{
				func() *mmodel.Account {
					b := false
					return &mmodel.Account{ID: "acc1", Metadata: map[string]any{"key1": "value1"}, Blocked: &b}
				}(),
				func() *mmodel.Account {
					b := true
					return &mmodel.Account{ID: "acc2", Metadata: map[string]any{"key2": "value2"}, Blocked: &b}
				}(),
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
			expectBusinessErr: true,
			expectInternalErr: false,
			expectedAccounts:  nil,
		},
		{
			name: "failure - error retrieving accounts",
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					FindAll(gomock.Any(), organizationID, ledgerID, &portfolioID, filter.ToOffsetPagination()).
					Return(nil, errors.New("failed to retrieve accounts")).
					Times(1)
			},
			expectBusinessErr: false,
			expectInternalErr: true,
			expectedAccounts:  nil,
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
					FindByEntityIDs(gomock.Any(), "Account", []string{"acc1", "acc2"}).
					Return(nil, errors.New("failed to retrieve metadata")).
					Times(1)
			},
			expectBusinessErr: true,
			expectInternalErr: false,
			expectedAccounts:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			result, err := uc.GetAllAccount(ctx, organizationID, ledgerID, &portfolioID, filter)

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
					assert.Equal(t, tt.expectedAccounts[i].ID, account.ID)
					assert.Equal(t, tt.expectedAccounts[i].Metadata, account.Metadata)
					// Assert blocked presence and value
					if tt.expectedAccounts[i].Blocked != nil {
						if account.Blocked == nil {
							t.Fatalf("expected blocked to be non-nil for account %s", account.ID)
						}
						assert.Equal(t, *tt.expectedAccounts[i].Blocked, *account.Blocked)
					}
				}
			}
		})
	}
}
