package command

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

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

	assert.NoError(t, err)
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

	assert.Error(t, err)
	assert.Equal(t, expectedErr.Error(), err.Error())
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

	expectedErr := errors.New("repository error")

	mockAccountTypeRepo.EXPECT().
		Delete(gomock.Any(), organizationID, ledgerID, id).
		Return(expectedErr).
		Times(1)

	err := uc.DeleteAccountTypeByID(context.Background(), organizationID, ledgerID, id)

	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}
