package command

import (
	"context"
	"errors"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestUpdateOrganizationByIDSuccess is responsible to test UpdateOrganizationByID with success
func TestUpdateOrganizationByIDSuccess(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	o := &mmodel.Organization{ID: id.String(), UpdatedAt: time.Now()}

	uc := UseCase{
		OrganizationRepo: organization.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*organization.MockRepository).
		EXPECT().
		Update(gomock.Any(), id, o).
		Return(o, nil).
		Times(1)
	res, err := uc.OrganizationRepo.Update(context.TODO(), id, o)

	assert.Equal(t, o, res)
	assert.Nil(t, err)
}

// TestUpdateOrganizationByIDError is responsible to test UpdateOrganizationByID with error
func TestUpdateOrganizationByIDError(t *testing.T) {
	id := pkg.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"
	o := &mmodel.Organization{ID: id.String(), UpdatedAt: time.Now()}

	uc := UseCase{
		OrganizationRepo: organization.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*organization.MockRepository).
		EXPECT().
		Update(gomock.Any(), id, o).
		Return(nil, errors.New(errMSG))
	res, err := uc.OrganizationRepo.Update(context.TODO(), id, o)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
