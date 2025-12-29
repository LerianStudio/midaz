package query

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetAccountByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:  mockAccountRepo,
		MetadataRepo: mockMetadataRepo,
	}

	// Pre-generate an ID to make assertions deterministic
	successAccountID := uuid.New()

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		portfolioID    *uuid.UUID
		accountID      uuid.UUID
		mockSetup      func()
		expectErr      bool
		expectedResult *mmodel.Account
	}{
		{
			name:           "Success - Retrieve account with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			accountID:      uuid.New(),
			mockSetup: func() {
				b := true
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{ID: successAccountID.String(), Name: "Test Account", Status: mmodel.Status{Code: "active"}, Blocked: &b}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"key": "value"}}, nil)
			},
			expectErr: false,
			expectedResult: &mmodel.Account{
				ID:       successAccountID.String(),
				Name:     "Test Account",
				Status:   mmodel.Status{Code: "active"},
				Metadata: map[string]any{"key": "value"},
				Blocked:  func() *bool { x := true; return &x }(),
			},
		},
		{
			name:           "Error - Account not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			accountID:      uuid.New(),
			mockSetup: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr:      true,
			expectedResult: nil,
		},
		{
			name:           "Error - Failed to retrieve metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			accountID:      uuid.New(),
			mockSetup: func() {
				accountID := uuid.New()
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{ID: accountID.String(), Name: "Test Account", Status: mmodel.Status{Code: "active"}}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("metadata retrieval error"))
			},
			expectErr:      true,
			expectedResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.GetAccountByID(ctx, tt.organizationID, tt.ledgerID, tt.portfolioID, tt.accountID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				// Validate critical fields
				assert.Equal(t, tt.expectedResult.ID, result.ID)
				assert.Equal(t, tt.expectedResult.Name, result.Name)
				assert.Equal(t, tt.expectedResult.Status, result.Status)
				assert.Equal(t, tt.expectedResult.Metadata, result.Metadata)

				// Validate Blocked for both nil and non-nil cases
				if tt.expectedResult != nil && tt.expectedResult.Blocked != nil {
					assert.NotNil(t, result.Blocked, "expected blocked to be non-nil")
					assert.Equal(t, *tt.expectedResult.Blocked, *result.Blocked)
				} else if tt.expectedResult != nil {
					assert.Nil(t, result.Blocked, "expected blocked to be nil")
				}
			}
		})
	}
}

func TestGetAccountByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountByID(ctx, uuid.Nil, uuid.New(), nil, uuid.New())
}

func TestGetAccountByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountByID(ctx, uuid.New(), uuid.Nil, nil, uuid.New())
}

func TestGetAccountByID_NilAccountID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil accountID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "accountID must not be nil UUID"),
			"panic message should mention accountID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountByID(ctx, uuid.New(), uuid.New(), nil, uuid.Nil)
}
