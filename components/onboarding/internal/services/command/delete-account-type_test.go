// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

var errAccTypeRepository = errors.New("repository error")

func TestDeleteAccountTypeByIDSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	mockAccountTypeRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(nil).
		Times(1)

	err := uc.DeleteAccountTypeByID(context.Background(), organizationID, ledgerID, id)

	require.NoError(t, err)
}

func TestDeleteAccountTypeByIDNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	expectedErr := pkg.ValidateBusinessError(constant.ErrAccountTypeNotFound, reflect.TypeOf(mmodel.AccountType{}).Name())

	mockAccountTypeRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(services.ErrDatabaseItemNotFound).
		Times(1)

	err := uc.DeleteAccountTypeByID(context.Background(), organizationID, ledgerID, id)

	require.Error(t, err)
	require.ErrorContains(t, err, expectedErr.Error())
}

func TestDeleteAccountTypeByIDError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountTypeRepo := accounttype.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountTypeRepo: mockAccountTypeRepo,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	id := uuid.New()

	expectedErr := errAccTypeRepository

	mockAccountTypeRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(expectedErr).
		Times(1)

	err := uc.DeleteAccountTypeByID(context.Background(), organizationID, ledgerID, id)

	require.Error(t, err)
	require.ErrorIs(t, err, expectedErr)
}
