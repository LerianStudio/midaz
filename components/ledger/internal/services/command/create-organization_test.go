package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/organization"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestCreateOrganizationSuccess is responsible to test CreateOrganization with success
func TestCreateOrganizationSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7().String()
	o := &mmodel.Organization{ID: id}

	uc := UseCase{
		OrganizationRepo: organization.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*organization.MockRepository).
		EXPECT().
		Create(gomock.Any(), o).
		Return(o, nil).
		Times(1)
	res, err := uc.OrganizationRepo.Create(context.TODO(), o)

	assert.Equal(t, o, res)
	assert.Nil(t, err)
}

// TestCreateOrganizationError is responsible to test CreateOrganization with error
func TestCreateOrganizationError(t *testing.T) {
	o := &mmodel.Organization{}
	errMSG := "err to create organization on database"

	uc := UseCase{
		OrganizationRepo: organization.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*organization.MockRepository).
		EXPECT().
		Create(gomock.Any(), o).
		Return(nil, errors.New(errMSG))
	res, err := uc.OrganizationRepo.Create(context.TODO(), o)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
