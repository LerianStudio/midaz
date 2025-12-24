package command

import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDeleteAccountByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mocks
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalancePort := mbootstrap.NewMockBalancePort(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
		BalancePort: mockBalancePort,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	portfolioID := uuid.New()
	accountID := uuid.New()

	tests := []struct {
		name              string
		portfolioID       *uuid.UUID
		setupMocks        func()
		expectInternalErr bool
		expectBusinessErr bool
		expectedErrMsg    string
	}{
		{
			name:        "success - account deleted",
			portfolioID: &portfolioID,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID).
					Return(&mmodel.Account{ID: accountID.String()}, nil).
					Times(1)

				mockBalancePort.EXPECT().
					DeleteAllBalancesByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
					Return(nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, accountID).
					Return(nil).
					Times(1)
			},
			expectInternalErr: false,
			expectBusinessErr: false,
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
			expectInternalErr: true,
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
			expectBusinessErr: true,
			expectedErrMsg:    "0074",
		},
		{
			name:        "failure - delete operation error",
			portfolioID: &portfolioID,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID).
					Return(&mmodel.Account{ID: accountID.String()}, nil).
					Times(1)

				mockBalancePort.EXPECT().
					DeleteAllBalancesByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
					Return(nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, accountID).
					Return(errors.New("delete error")).
					Times(1)
			},
			expectInternalErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Configuração dos mocks
			tt.setupMocks()

			// Executa a função
			err := uc.DeleteAccountByID(ctx, organizationID, ledgerID, tt.portfolioID, accountID, "token")

			// Validações
			if tt.expectInternalErr {
				assert.Error(t, err)
				var internalErr pkg.InternalServerError
				assert.True(t, errors.As(err, &internalErr), "expected InternalServerError type")
			} else if tt.expectBusinessErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDeleteAccountByIDSuccess is responsible to test DeleteAccountByID with success
func TestDeleteAccountByIDSuccess(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	portfolioID := libCommons.GenerateUUIDv7()
	id := libCommons.GenerateUUIDv7()
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
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	id := libCommons.GenerateUUIDv7()
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
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	portfolioID := libCommons.GenerateUUIDv7()
	id := libCommons.GenerateUUIDv7()
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
