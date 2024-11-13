package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mmodel"
	"testing"

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
	limit := 10
	page := 1

	uc := UseCase{
		OrganizationRepo: mockOrganizationRepo,
	}

	t.Run("Success", func(t *testing.T) {
		organizations := []*mmodel.Organization{{}}
		mockOrganizationRepo.
			EXPECT().
			FindAll(gomock.Any(), limit, page).
			Return(organizations, nil).
			Times(1)
		res, err := uc.OrganizationRepo.FindAll(context.TODO(), limit, page)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockOrganizationRepo.
			EXPECT().
			FindAll(gomock.Any(), limit, page).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.OrganizationRepo.FindAll(context.TODO(), limit, page)

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
