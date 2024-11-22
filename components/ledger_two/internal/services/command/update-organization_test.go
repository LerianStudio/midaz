package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mmodel"
	mock "github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/mock/onboarding/organization"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestUpdateOrganizationByIDSuccess is responsible to test UpdateOrganizationByID with success
func TestUpdateOrganizationByIDSuccess(t *testing.T) {
	id := common.GenerateUUIDv7()
	organization := &mmodel.Organization{ID: id.String(), UpdatedAt: time.Now()}

	uc := UseCase{
		OrganizationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), id, organization).
		Return(organization, nil).
		Times(1)
	res, err := uc.OrganizationRepo.Update(context.TODO(), id, organization)

	assert.Equal(t, organization, res)
	assert.Nil(t, err)
}

// TestUpdateOrganizationByIDError is responsible to test UpdateOrganizationByID with error
func TestUpdateOrganizationByIDError(t *testing.T) {
	id := common.GenerateUUIDv7()
	errMSG := "errDatabaseItemNotFound"
	organization := &mmodel.Organization{ID: id.String(), UpdatedAt: time.Now()}

	uc := UseCase{
		OrganizationRepo: mock.NewMockRepository(gomock.NewController(t)),
	}

	uc.OrganizationRepo.(*mock.MockRepository).
		EXPECT().
		Update(gomock.Any(), id, organization).
		Return(nil, errors.New(errMSG))
	res, err := uc.OrganizationRepo.Update(context.TODO(), id, organization)

	assert.NotEmpty(t, err)
	assert.Equal(t, err.Error(), errMSG)
	assert.Nil(t, res)
}
