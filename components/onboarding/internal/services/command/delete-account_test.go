// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

var (
	errItemNotFound      = errors.New("errDatabaseItemNotFound")
	errDelete            = errors.New("delete error")
	errForbiddenExternal = errors.New("0074 - Accounts of type 'external' cannot be deleted or modified as they are used for traceability with external systems. Please review your request and ensure operations are only performed on internal accounts.") //nolint:revive,staticcheck // business error message
)

func TestDeleteAccountByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Mocks
	mockAccountRepo := account.NewMockRepository(ctrl)
	mockBalanceGRPCRepo := mbootstrap.NewMockBalancePort(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
		BalancePort: mockBalanceGRPCRepo,
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

				mockBalanceGRPCRepo.EXPECT().
					DeleteAllBalancesByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
					Return(nil).
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
			expectedErr: errItemNotFound,
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
			expectedErr: errForbiddenExternal,
		},
		{
			name:        "failure - delete operation error",
			portfolioID: &portfolioID,
			setupMocks: func() {
				mockAccountRepo.EXPECT().
					Find(gomock.Any(), organizationID, ledgerID, nil, accountID).
					Return(&mmodel.Account{ID: accountID.String()}, nil).
					Times(1)

				mockBalanceGRPCRepo.EXPECT().
					DeleteAllBalancesByAccountID(gomock.Any(), organizationID, ledgerID, accountID, gomock.Any()).
					Return(nil).
					Times(1)

				mockAccountRepo.EXPECT().
					Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, accountID).
					Return(errDelete).
					Times(1)
			},
			expectedErr: errDelete,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Configuração dos mocks
			tt.setupMocks()

			// Executa a função
			err := uc.DeleteAccountByID(ctx, organizationID, ledgerID, tt.portfolioID, accountID, "token")

			// Validações
			if tt.expectedErr != nil {
				require.Error(t, err)
				require.ErrorContains(t, err, tt.expectedErr.Error())
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDeleteAccountByIDSuccess is responsible to test DeleteAccountByID with success.
func TestDeleteAccountByIDSuccess(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	portfolioID := libCommons.GenerateUUIDv7()
	id := libCommons.GenerateUUIDv7()
	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	mockRepo, ok := uc.AccountRepo.(*account.MockRepository)
	require.True(t, ok, "expected AccountRepo to be *account.MockRepository")

	mockRepo.
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(nil).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	require.NoError(t, err)
}

// TestDeleteAccountByIDWithoutPortfolioSuccess is responsible to test DeleteAccountByID without portfolio with success.
func TestDeleteAccountByIDWithoutPortfolioSuccess(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	id := libCommons.GenerateUUIDv7()
	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	mockRepo, ok := uc.AccountRepo.(*account.MockRepository)
	require.True(t, ok, "expected AccountRepo to be *account.MockRepository")

	mockRepo.
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, nil, id).
		Return(nil).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, nil, id)

	require.NoError(t, err)
}

// TestDeleteAccountByIDError is responsible to test DeleteAccountByID with error.
func TestDeleteAccountByIDError(t *testing.T) {
	organizationID := libCommons.GenerateUUIDv7()
	ledgerID := libCommons.GenerateUUIDv7()
	portfolioID := libCommons.GenerateUUIDv7()
	id := libCommons.GenerateUUIDv7()

	uc := UseCase{
		AccountRepo: account.NewMockRepository(gomock.NewController(t)),
	}

	mockRepo, ok := uc.AccountRepo.(*account.MockRepository)
	require.True(t, ok, "expected AccountRepo to be *account.MockRepository")

	mockRepo.
		EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, &portfolioID, id).
		Return(errDelete).
		Times(1)
	err := uc.AccountRepo.Delete(context.TODO(), organizationID, ledgerID, &portfolioID, id)

	require.Error(t, err)
	require.ErrorIs(t, err, errDelete)
}
