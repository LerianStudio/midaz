package organization

import (
	"bytes"
	"errors"
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

func Test_newCmdOrganizationDescribe(t *testing.T) {
	tests := []struct {
		name    string
		runTest func(t *testing.T)
	}{
		{
			name: "happy road no metadata",
			runTest: func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				mockRepo := repository.NewMockOrganization(ctrl)

				orgFactory := factoryOrganizationDescribe{
					factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
						Out: &bytes.Buffer{},
						Err: &bytes.Buffer{},
					}},
					repoOrganization: mockRepo,
					organizationID:   "123",
					Out:              "",
					JSON:             false,
				}

				cmd := newCmdOrganizationDescribe(&orgFactory)

				timeNow := time.Now()

				item := model.OrganizationItem{
					ID:                   "123",
					ParentOrganizationID: nil,
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
				}

				mockRepo.EXPECT().GetByID(gomock.Any()).Return(&item, nil)

				err := cmd.Execute()
				assert.NoError(t, err)

				output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
				expectedOut := "FIELDS               VALUES                     " +
					"                             \nID:                  123        " +
					"                                             \nLegal"
				assert.Contains(t, output, expectedOut)
			},
		},
		{
			name: "happy road with metadata",
			runTest: func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				mockRepo := repository.NewMockOrganization(ctrl)

				orgFactory := factoryOrganizationDescribe{
					factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
						Out: &bytes.Buffer{},
						Err: &bytes.Buffer{},
					}},
					repoOrganization: mockRepo,
					organizationID:   "123",
					Out:              "",
					JSON:             false,
				}

				cmd := newCmdOrganizationDescribe(&orgFactory)

				timeNow := time.Now()

				item := model.OrganizationItem{
					ID:                   "125",
					ParentOrganizationID: nil,
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
					Metadata: &model.Metadata{
						Bitcoin: ptr.StringPtr("12312312"),
					},
				}

				mockRepo.EXPECT().GetByID(gomock.Any()).Return(&item, nil)

				err := cmd.Execute()
				assert.NoError(t, err)

				output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
				expectedOut := "FIELDS               VALUES                     " +
					"                             \nID:                  125        " +
					"                                             \nLegal"
				assert.Contains(t, output, expectedOut)
			},
		},
		{
			name: "error get id api",
			runTest: func(t *testing.T) {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				mockRepo := repository.NewMockOrganization(ctrl)

				orgFactory := factoryOrganizationDescribe{
					factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
						Out: &bytes.Buffer{},
						Err: &bytes.Buffer{},
					}},
					repoOrganization: mockRepo,
					organizationID:   "123",
					Out:              "",
					JSON:             false,
				}

				cmd := newCmdOrganizationDescribe(&orgFactory)

				mockRepo.EXPECT().GetByID(gomock.Any()).Return(nil,
					errors.New("error get item organization"))

				err := cmd.Execute()
				assert.EqualError(t, err, "error get item organization")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, tc.runTest)
	}
}
