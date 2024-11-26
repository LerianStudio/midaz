package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/pkg"

	"github.com/stretchr/testify/assert"
)

// TestDeleteOrganizationByIDSuccess is responsible to test DeleteOrganizationByID with success
func TestDeleteOrganizationByIDSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()

	uc := UseCase{
		OrganizationRepo: organization.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*organization.MockRepository).
		EXPECT().
		Delete(gomock.Any(), id).
		Return(nil).
		Times(1)
	err := uc.OrganizationRepo.Delete(context.TODO(), id)

	assert.Nil(t, err)
}

// TestDeleteOrganizationByIDError is responsible to test DeleteOrganizationByID with error
func TestDeleteOrganizationByIDError(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		OrganizationRepo: organization.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*organization.MockRepository).
		EXPECT().
		Delete(gomock.Any(), id).
		Return(errors.New(errMSG)).
		Times(1)
	err := uc.OrganizationRepo.Delete(context.TODO(), id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
}
