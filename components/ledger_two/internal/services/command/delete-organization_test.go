package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	mock "github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/mock/onboarding/organization"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestDeleteOrganizationByIDSuccess is responsible to test DeleteOrganizationByID with success
func TestDeleteOrganizationByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()

	uc := UseCase{
		OrganizationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), id).
		Return(nil).
		Times(1)
	err := uc.OrganizationRepo.Delete(context.TODO(), id)

	assert.Nil(t, err)
}

// TestDeleteOrganizationByIDError is responsible to test DeleteOrganizationByID with error
func TestDeleteOrganizationByIDError(t *testing.T) {
	id := common.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		OrganizationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*mock.MockRepository).
		EXPECT().
		Delete(gomock.Any(), id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.OrganizationRepo.Delete(context.TODO(), id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
