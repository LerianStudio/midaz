package query

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

// TestGetOrganizationByIDSuccess is responsible to test GetOrganizationByID with success
func TestGetOrganizationByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	organization := &mmodel.Organization{ID: id.String()}

	uc := UseCase{
		OrganizationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), id).
		Return(organization, nil).
		Times(1)
	res, err := uc.OrganizationRepo.Find(context.TODO(), id)

	assert.Equal(t, res, organization)
	assert.Nil(t, err)
}

// TestGetOrganizationByIDError is responsible to test GetOrganizationByID with error
func TestGetOrganizationByIDError(t *testing.T) {
	id := common.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"

	uc := UseCase{
		OrganizationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*mock.MockRepository).
		EXPECT().
		Find(gomock.Any(), id).
		Return(nil, errors.New(errMSG)).
		Times(1)
	res, err := uc.OrganizationRepo.Find(context.TODO(), id)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
