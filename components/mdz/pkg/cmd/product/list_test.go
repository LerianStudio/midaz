package product

import (
	"bytes"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
	"gotest.tools/golden"
)

func Test_newCmdProductList(t *testing.T) {
	t.Run("with flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockProduct(ctrl)

		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		ledFactory := factoryProductList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoProduct:    mockRepo,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
		}

		cmd := newCmdProductList(&ledFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
		})

		list := &mmodel.Products{
			Page:  1,
			Limit: 2,
			Items: []mmodel.Product{
				{
					ID:   "01930365-4d46-7a09-a503-b932714f85af",
					Name: "2Real",
					Status: mmodel.Status{
						Code:        "ACTIVE",
						Description: ptr.StringPtr("Teste asset 1"),
					},
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 21, 33, 10, 854653000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 21, 33, 10, 854653000, time.UTC),
					DeletedAt:      nil,
					Metadata: map[string]any{
						"bitcoin": "1RuuEjC8CziKy6XbYU6uwsNSYjU7H2Mft",
						"chave":   "metadata_chave",
						"boolean": false,
					},
				},
				{
					ID:   "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Name: "Brazilian Real",
					Status: mmodel.Status{
						Code:        "ACTIVE",
						Description: ptr.StringPtr("Teste asset 1"),
					},
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					DeletedAt:      nil,
					Metadata: map[string]any{
						"bitcoin": "3oDTprwNG37nASsyLzQGLuUBzNac",
						"chave":   "metadata_chave",
						"boolean": false,
					},
				},
			},
		}

		mockRepo.EXPECT().Get(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any()).
			Return(list, nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := ledFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
		golden.AssertBytes(t, output, "output_list.golden")
	})

	t.Run("no flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockProduct(ctrl)

		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		ledFactory := factoryProductList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoProduct:    mockRepo,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
		}

		ledFactory.tuiInput = func(message string) (string, error) {
			return "1234", nil
		}

		cmd := newCmdProductList(&ledFactory)
		cmd.SetArgs([]string{})

		list := &mmodel.Products{
			Page:  1,
			Limit: 2,
			Items: []mmodel.Product{
				{
					ID:   "01930365-4d46-7a09-a503-b932714f85af",
					Name: "2Real",
					Status: mmodel.Status{
						Code:        "ACTIVE",
						Description: ptr.StringPtr("Teste asset 1"),
					},
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 21, 33, 10, 854653000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 21, 33, 10, 854653000, time.UTC),
					DeletedAt:      nil,
					Metadata: map[string]any{
						"bitcoin": "1RuuEjC8CziKy6XbYU6uwsNSYjU7H2Mft",
						"chave":   "metadata_chave",
						"boolean": false,
					},
				},
				{
					ID:   "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Name: "Brazilian Real",
					Status: mmodel.Status{
						Code:        "ACTIVE",
						Description: ptr.StringPtr("Teste asset 1"),
					},
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					DeletedAt:      nil,
					Metadata: map[string]any{
						"bitcoin": "3oDTprwNG37nASsyLzQGLuUBzNac",
						"chave":   "metadata_chave",
						"boolean": false,
					},
				},
			},
		}

		mockRepo.EXPECT().Get(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any()).
			Return(list, nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := ledFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
		golden.AssertBytes(t, output, "output_list.golden")
	})

	t.Run("valid date", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockProduct(ctrl)

		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		ledFactory := factoryProductList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoProduct:    mockRepo,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
		}

		ledFactory.tuiInput = func(message string) (string, error) {
			return "1234", nil
		}

		cmd := newCmdProductList(&ledFactory)
		cmd.SetArgs([]string{"--start-date", "2023-11-01", "--end-date", "2023-11-10"})

		list := &mmodel.Products{
			Page:  1,
			Limit: 2,
			Items: []mmodel.Product{
				{
					ID:   "01930365-4d46-7a09-a503-b932714f85af",
					Name: "2Real",
					Status: mmodel.Status{
						Code:        "ACTIVE",
						Description: ptr.StringPtr("Teste asset 1"),
					},
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 21, 33, 10, 854653000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 21, 33, 10, 854653000, time.UTC),
					DeletedAt:      nil,
					Metadata: map[string]any{
						"bitcoin": "1RuuEjC8CziKy6XbYU6uwsNSYjU7H2Mft",
						"chave":   "metadata_chave",
						"boolean": false,
					},
				},
				{
					ID:   "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Name: "Brazilian Real",
					Status: mmodel.Status{
						Code:        "ACTIVE",
						Description: ptr.StringPtr("Teste asset 1"),
					},
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					DeletedAt:      nil,
					Metadata: map[string]any{
						"bitcoin": "3oDTprwNG37nASsyLzQGLuUBzNac",
						"chave":   "metadata_chave",
						"boolean": false,
					},
				},
			},
		}

		mockRepo.EXPECT().Get(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(), gomock.Any()).
			Return(list, nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := ledFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
		golden.AssertBytes(t, output, "output_list.golden")
	})
}
