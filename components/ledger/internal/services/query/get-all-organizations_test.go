package query

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"go.uber.org/mock/gomock"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// TestGetAllOrganizationsError is responsible to test GetAllOrganizations with success and error
func TestGetAllOrganizations(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockOrganizationRepo := organization.NewMockRepository(ctrl)
	filter := http.QueryHeader{
		Limit:        10,
		Page:         1,
		SortOrder:    "asc",
		StartDate:    time.Now().AddDate(0, -1, 0),
		EndDate:      time.Now(),
		ToAssetCodes: []string{"BRL"},
	}

	uc := UseCase{
		OrganizationRepo: mockOrganizationRepo,
	}

	t.Run("Success", func(t *testing.T) {
		organizations := []*mmodel.Organization{{}}
		mockOrganizationRepo.
			EXPECT().
			FindAll(gomock.Any(), filter.ToOffsetPagination()).
			Return(organizations, nil).
			Times(1)
		res, err := uc.OrganizationRepo.FindAll(context.TODO(), filter.ToOffsetPagination())

		assert.NoError(t, err)
		assert.Len(t, res, 1)
	})

	t.Run("Error", func(t *testing.T) {
		errMsg := "errDatabaseItemNotFound"
		mockOrganizationRepo.
			EXPECT().
			FindAll(gomock.Any(), filter.ToOffsetPagination()).
			Return(nil, errors.New(errMsg)).
			Times(1)
		res, err := uc.OrganizationRepo.FindAll(context.TODO(), filter.ToOffsetPagination())

		assert.EqualError(t, err, errMsg)
		assert.Nil(t, res)
	})
}
