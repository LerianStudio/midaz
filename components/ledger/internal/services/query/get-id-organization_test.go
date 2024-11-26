package query

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/organization"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetOrganizationByIDSuccess is responsible to test GetOrganizationByID with success
func TestGetOrganizationByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	o := &mmodel.Organization{ID: id.String()}

	uc := UseCase{
		OrganizationRepo: organization.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*organization.MockRepository).
		EXPECT().
		Find(gomock.Any(), id).
		Return(o, nil).
		Times(1)
	res, err := uc.OrganizationRepo.Find(context.TODO(), id)

	assert.Equal(t, res, o)
	assert.Nil(t, err)
}

// TestGetOrganizationByIDError is responsible to test GetOrganizationByID with error
func TestGetOrganizationByIDError(t *testing.T) {
	id := common.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		OrganizationRepo: organization.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*organization.MockRepository).
		EXPECT().
		Find(gomock.Any(), id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.OrganizationRepo.Find(context.TODO(), id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
