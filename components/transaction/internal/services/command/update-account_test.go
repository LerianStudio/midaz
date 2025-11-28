package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestUpdateAccount(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:            mockAccountRepo,
		MetadataOnboardingRepo: mockMetadataRepo,
	}

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		portfolioID    *uuid.UUID
		accountID      uuid.UUID
		input          *mmodel.UpdateAccountInput
		mockSetup      func()
		expectErr      bool
	}{
		{
			name:           "Success - Account updated with metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			accountID:      uuid.New(),
			input: &mmodel.UpdateAccountInput{
				Name: "Updated Account",
				Status: mmodel.Status{
					Code: "active",
				},
				SegmentID: nil,
				Metadata:  map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{ID: "123", Type: "internal"}, nil)
				mockAccountRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{ID: "123", Name: "Updated Account", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mongodb.Metadata{Data: map[string]any{"existing_key": "existing_value"}}, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil)
			},
			expectErr: false,
		},
		{
			name:           "Error - Account not found",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			accountID:      uuid.New(),
			input: &mmodel.UpdateAccountInput{
				Name: "Nonexistent Account",
				Status: mmodel.Status{
					Code: "active",
				},
				SegmentID: nil,
				Metadata:  nil,
			},
			mockSetup: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, services.ErrDatabaseItemNotFound)
			},
			expectErr: true,
		},
		{
			name:           "Error - Failed to update metadata",
			organizationID: uuid.New(),
			ledgerID:       uuid.New(),
			portfolioID:    nil,
			accountID:      uuid.New(),
			input: &mmodel.UpdateAccountInput{
				Name: "Updated Account",
				Status: mmodel.Status{
					Code: "active",
				},
				SegmentID: nil,
				Metadata:  map[string]any{"key": "value"},
			},
			mockSetup: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{ID: "123", Type: "internal"}, nil)
				mockAccountRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&mmodel.Account{ID: "123", Name: "Updated Account", Status: mmodel.Status{Code: "active"}, Metadata: nil}, nil)
				mockMetadataRepo.EXPECT().
					FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, nil)
				mockMetadataRepo.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(errors.New("metadata update error"))
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			result, err := uc.UpdateAccount(ctx, tt.organizationID, tt.ledgerID, tt.portfolioID, tt.accountID, tt.input)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.input.Name, result.Name)
				assert.Equal(t, tt.input.Status, result.Status)
			}
		})
	}
}

// Test updating blocked flag when provided (true)
func TestUpdateAccount_BlockedProvidedTrue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:            mockAccountRepo,
		MetadataOnboardingRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()
	blocked := true

	// Expectations
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "internal"}, nil)

	mockAccountRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
			if acc.Blocked == nil || !*acc.Blocked {
				t.Fatalf("expected acc.Blocked to be true and non-nil")
			}
			// Echo back
			return &mmodel.Account{ID: accountID.String(), Name: "Updated Account", Status: mmodel.Status{Code: "active"}, Blocked: acc.Blocked}, nil
		})

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)
	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	inp := &mmodel.UpdateAccountInput{
		Name:     "Updated Account",
		Status:   mmodel.Status{Code: "active"},
		Metadata: map[string]any{"key": "value"},
		Blocked:  &blocked,
	}

	ctx := context.Background()
	result, err := uc.UpdateAccount(ctx, organizationID, ledgerID, nil, accountID, inp)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	if result.Blocked == nil || !*result.Blocked {
		t.Fatalf("expected result.Blocked true, got nil/false")
	}
}

// Test that omitting blocked does not send a value to repository (remains nil)
func TestUpdateAccount_BlockedOmitted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:            mockAccountRepo,
		MetadataOnboardingRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "internal"}, nil)

	mockAccountRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ uuid.UUID, _ *uuid.UUID, _ uuid.UUID, acc *mmodel.Account) (*mmodel.Account, error) {
			if acc.Blocked != nil {
				t.Fatalf("expected acc.Blocked to be nil when omitted")
			}
			return &mmodel.Account{ID: accountID.String(), Name: "Updated Account"}, nil
		})

	mockMetadataRepo.EXPECT().
		FindByEntity(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)
	mockMetadataRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil)

	inp := &mmodel.UpdateAccountInput{
		Name:     "Updated Account",
		Status:   mmodel.Status{Code: "active"},
		Metadata: map[string]any{"key": "value"},
		// Blocked omitted
	}

	ctx := context.Background()
	result, err := uc.UpdateAccount(ctx, organizationID, ledgerID, nil, accountID, inp)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// Test that updating an external account is forbidden
func TestUpdateAccount_ExternalForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo:            mockAccountRepo,
		MetadataOnboardingRepo: mockMetadataRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	mockAccountRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "external"}, nil)

	inp := &mmodel.UpdateAccountInput{Name: "Updated"}
	ctx := context.Background()
	result, err := uc.UpdateAccount(ctx, organizationID, ledgerID, nil, accountID, inp)

	assert.Error(t, err)
	assert.Nil(t, result)
}
