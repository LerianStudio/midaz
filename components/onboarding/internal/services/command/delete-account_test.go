package command

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
)

func TestDeleteAccountByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mocks
	mockAccountRepo := account.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolioID := uuid.New()
	accountID := uuid.New()

	tests := []struct {
		name        string
		portfolioID *uuid.UUID
		setupMocks  func()
		expectedErr error
	}{
		{
			name:        "success - account deleted",
			portfolioID: &portfolioID,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID).
					Return(&mmodel.Account{ID: accountID.String()}, nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, accountID).
					Return(nil).
					Times(1)
			},
			expectedErr: nil,
		},
		{
			name:        "failure - account not found",
			portfolioID: nil,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID).
					Return(nil, services.ErrDatabaseItemNotFound).
					Times(1)
			},
			expectedErr: errors.New("errDatabaseItemNotFound"),
		},
		{
			name:        "failure - forbidden external account manipulation",
			portfolioID: nil,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID).
					Return(&mmodel.Account{ID: accountID.String(), Type: "external"}, nil).
					Times(1)
			},
			expectedErr: errors.New("0074 - Accounts of type 'external' cannot be deleted or modified as they are used for traceability with external systems. Please review your request and ensure operations are only performed on internal accounts."),
		},
		{
			name:        "failure - delete operation error",
			portfolioID: &portfolioID,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID).
					Return(&mmodel.Account{ID: accountID.String()}, nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, accountID).
					Return(errors.New("delete error")).
					Times(1)
			},
			expectedErr: errors.New("delete error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Configuração dos mocks
			tt.setupMocks()

			// Executa a função
			err := uc.DeleteAccountByID(ctx, organizationID, ledgerID, tt.portfolioID, accountID)

			// Validações
			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDeleteAccountByIDSuccess is responsible to test DeleteAccountByID with success
func TestDeleteAccountByIDSuccess(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	portfolioID := pkg.GenerateUUIDv7()
	id := pkg.GenerateUUIDv7()
	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(nil).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	assert.Nil(t, err)
}

// TestDeleteAccountByIDWithoutPortfolioSuccess is responsible to test DeleteAccountByID without portfolio with success
func TestDeleteAccountByIDWithoutPortfolioSuccess(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	id := pkg.GenerateUUIDv7()
	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, nil, id).
		Return(nil).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, nil, id)

	assert.Nil(t, err)
}

// TestDeleteAccountByIDError is responsible to test DeleteAccountByID with error
func TestDeleteAccountByIDError(t *testing.T) {
	organizationID := pkg.GenerateUUIDv7()
	ledgerID := pkg.GenerateUUIDv7()
	portfolioID := pkg.GenerateUUIDv7()
	id := pkg.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	uc.AccountRepo.(*account.MockRepository).
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
