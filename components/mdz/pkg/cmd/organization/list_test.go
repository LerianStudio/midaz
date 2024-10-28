package organization

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func Test_newCmdOrganizationList(t *testing.T) {
	t.Run("happy road", func(t *testing.T) {
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

		timeNow := time.Now()

		list := model.OrganizationList{
			Items: []model.OrganizationItem{
				{
					ID:                   "123",
					ParentOrganizationID: ptr.StringPtr(""),
					LegalName:            "Test Organization",
					DoingBusinessAs:      "The ledger.io",
					LegalDocument:        "48784548000104",
					Address: model.Address{
						Country: "BR",
					},
					Status: model.Status{
						Description: "Test Ledger",
						Code:        ptr.StringPtr("2123"),
					},
					CreatedAt: timeNow,
					UpdatedAt: timeNow,
				},
			},
			Limit: 10,
			Page:  1,
		}

		mockRepo.EXPECT().Get(gomock.Any(), gomock.Any()).
			Return(&list, nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()

		expectedOut := "ID   PARENT_ORGANIZATION_ID  LEGALNAME          "
		assert.Contains(t, output, expectedOut)
	})

	t.Run("flag --json", func(t *testing.T) {
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
		cmd.SetArgs([]string{"--json"})

		timeNow := time.Now()

		list := model.OrganizationList{
			Items: []model.OrganizationItem{
				{
					ID:                   "123",
					ParentOrganizationID: ptr.StringPtr(""),
					LegalName:            "Test Organization",
					DoingBusinessAs:      "The ledger.io",
					LegalDocument:        "48784548000104",
					Address: model.Address{
						Country: "BR",
					},
					Status: model.Status{
						Description: "Test Ledger",
						Code:        ptr.StringPtr("2123"),
					},
					CreatedAt: timeNow,
					UpdatedAt: timeNow,
				},
			},
			Limit: 10,
			Page:  1,
		}

		mockRepo.EXPECT().Get(gomock.Any(), gomock.Any()).
			Return(&list, nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()

		expectedOut := `{"items":[{"id":"123","parentOrganizationId":"",`
		fmt.Println(output)
		assert.Contains(t, output, expectedOut)
	})

	t.Run("error request api", func(t *testing.T) {
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
		cmd.SetArgs([]string{"--json"})

		mockRepo.EXPECT().Get(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("error generic"))

		err := cmd.Execute()
		assert.EqualError(t, err, "error generic")
	})
}
