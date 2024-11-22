package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	mock "github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/mock/onboarding/organization"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateOrganizationSuccess is responsible to test CreateOrganization with success
func TestCreateOrganizationSuccess(t *testing.T) {
	id := common.GenerateUUIDv7().String()
	organization := &mmodel.Organization{ID: id}

	uc := UseCase{
		OrganizationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), organization).
		Return(organization, nil).
		Times(1)
	res, err := uc.OrganizationRepo.Create(context.TODO(), organization)

	assert.Equal(t, organization, res)
	assert.Nil(t, err)
}

// TestCreateOrganizationError is responsible to test CreateOrganization with error
func TestCreateOrganizationError(t *testing.T) {
	organization := &mmodel.Organization{}
	errMSG := "err to create organization on database"

	uc := UseCase{
		OrganizationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*mock.MockRepository).
		EXPECT().
		Create(gomock.Any(), organization).
		Return(nil, errors.New(errMSG))
	res, err := uc.OrganizationRepo.Create(context.TODO(), organization)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
