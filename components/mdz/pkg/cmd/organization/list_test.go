package organization

import (
	"bytes"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"gotest.tools/golden"
)

func Test_newCmdOrganizationList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockOrganization(ctrl)

	orgFactory := factoryOrganizationList{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoOrganization: mockRepo,
	}

	cmd := newCmdOrganizationList(&orgFactory)

	gotOrgs := mmodel.Organizations{
		Items: []mmodel.Organization{
			{
				ID:                   "123",
				ParentOrganizationID: ptr.StringPtr(""),
				LegalName:            "Test Organization",
				DoingBusinessAs:      ptr.StringPtr("The ledger.io"),
				LegalDocument:        "48784548000104",
				Address: mmodel.Address{
					Country: "BR",
				},
				Status: mmodel.Status{
					Description: ptr.StringPtr("Test Ledger"),
					Code:        "2123",
				},
				CreatedAt: time.Date(2024, 10, 31, 11, 31, 22, 369928000, time.UTC),
				UpdatedAt: time.Date(2024, 10, 31, 11, 31, 22, 369928000, time.UTC),
			},
		},
		Limit: 10,
		Page:  1,
	}

	mockRepo.EXPECT().Get(gomock.Any(), gomock.Any()).Return(&gotOrgs, nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
	golden.AssertBytes(t, output, "output_list.golden")
}
