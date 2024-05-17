package query

import (
	"context"
	"errors"
	"testing"

	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	mock "github.com/LerianStudio/midaz/components/ledger/internal/gen/mock/organization"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// TestGetAllOrganizationsError is responsible to test GetAllOrganizations with success and error
func TestGetAllOrganizations(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockOrganizationRepo := mock.NewMockRepository(ctrl)

	uc := UseCase{
		OrganizationRepo: mockOrganizationRepo,
	}

	t.Run("Success", func(t *testing.T) {
		organizations := []*o.Organization{{}}
		mockOrganizationRepo.
			EXPECT().
			FindAll(gomock.Any()).
			Return(organizations, nil).
			Times(1)
		res, err := uc.OrganizationRepo.FindAll(context.TODO())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockOrganizationRepo.
			EXPECT().
			FindAll(gomock.Any()).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.OrganizationRepo.FindAll(context.TODO())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
