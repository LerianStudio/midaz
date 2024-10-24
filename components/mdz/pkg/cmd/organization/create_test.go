package organization

import (
	"bytes"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestRunE(t *testing.T) {
	t.Run("happy road", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockOrganization(ctrl)

		orgFactory := factoryOrganizationCreate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoOrganiztion: mockRepo,
			tuiInput: func(message string) (string, error) {
				return "name", nil
			},
			flags: flags{
				LegalName:       "Test Organization",
				DoingBusinessAs: "The ledger.io",
				LegalDocument:   "48784548000104",
				Code:            "ACTIVE",
				Description:     "Teste Ledger",
				City:            "Test City",
				Country:         "BR",
				Bitcoin:         "1YLHctiipHZupwrT5sGwuYbks5rn64bm",
				Boolean:         "true",
				Double:          "10.5",
				Int:             "1",
			},
		}

		cmd := newCmdOrganizationCreate(&orgFactory)
		cmd.SetArgs([]string{
			"--legal-name", "Test Organization",
			"--doing-business-as", "The ledger.io",
			"--legal-document", "48784548000104",
			"--code", "ACTIVE",
			"--city", "Test City",
			"--description", "Test Ledger",
			"--country", "BR",
			"--bitcoin", "1YLHctiipHZupwrT5sGwuYbks5rn64bm",
			"--boolean", "true",
			"--double", "10.5",
			"--int", "1",
		})

		gotOrg := model.OrganizationResponse{
			ID:              "123",
			LegalName:       "Test Organization",
			DoingBusinessAs: "The ledger.io",
			LegalDocument:   "48784548000104",
			Address: model.Address{
				Country: "BR",
			},
			Status: model.Status{
				Description: "Test Ledger",
			},
		}

		mockRepo.EXPECT().Create(gomock.Any()).DoAndReturn(func(org model.Organization) (*model.OrganizationResponse, error) {
			assert.Equal(t, "Test Organization", org.LegalName)
			assert.Equal(t, "The ledger.io", org.DoingBusinessAs)
			assert.Equal(t, "48784548000104", org.LegalDocument)
			assert.Equal(t, "BR", org.Address.Country)
			assert.Equal(t, "Test Ledger", org.Status.Description)

			return &gotOrg, nil
		})

		err := cmd.Execute()
		assert.NoError(t, err)

		output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The organization_id 123 has been successfully created")
	})

	t.Run("complete with all flags ", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockOrganization(ctrl)

		orgFactory := factoryOrganizationCreate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoOrganiztion: mockRepo,
			tuiInput: func(message string) (string, error) {
				return "name", nil
			},
			flags: flags{
				LegalName:       "Gislason LLC",
				DoingBusinessAs: "The ledger.io",
				LegalDocument:   "48784548000104",
				Code:            "ACTIVE",
				Description:     "Teste Ledger",
				Line1:           "Avenida Paulista, 1234",
				Line2:           "CJ 203",
				ZipCode:         "04696040",
				City:            "West Kennediberg",
				State:           "CZ",
				Country:         "MG",
				Chave:           "metadata_chave",
				Bitcoin:         "1YLHctiipHZupwrT5sGwuYbks5rn64bm",
				Boolean:         "true",
				Double:          "10.5",
				Int:             "1",
			},
		}

		cmd := newCmdOrganizationCreate(&orgFactory)
		cmd.SetArgs([]string{
			"--legal-name", "Gislason LLC",
			"--doing-business-as", "The ledger.io",
			"--legal-document", "48784548000104",
			"--code", "ACTIVE",
			"--description", "Teste Ledger",
			"--line1", "Avenida Paulista, 1234",
			"--line2", "CJ 203",
			"--zip-code", "04696040",
			"--city", "West Kennediberg",
			"--state", "CZ",
			"--country", "MG",
			"--bitcoin", "1YLHctiipHZupwrT5sGwuYbks5rn64bm",
			"--boolean", "true",
			"--chave", "metadata_chave",
			"--double", "10.5",
			"--int", "1",
		})

		gotOrg := model.OrganizationResponse{
			ID:              "123",
			LegalName:       "Gislason LLC",
			DoingBusinessAs: "The ledger.io",
			LegalDocument:   "48784548000104",
			Address: model.Address{
				Line1:   ptr.StringPtr("Avenida Paulista, 1234"),
				Line2:   ptr.StringPtr("CJ 203"),
				ZipCode: ptr.StringPtr("04696040"),
				City:    ptr.StringPtr("West Kennediberg"),
				State:   ptr.StringPtr("CZ"),
				Country: "MG",
			},
			Status: model.Status{
				Code:        ptr.StringPtr("ACTIVE"),
				Description: "Teste Ledger",
			},
			Metadata: model.Metadata{
				Bitcoin: ptr.StringPtr("1YLHctiipHZupwrT5sGwuYbks5rn64bm"),
				Boolean: ptr.BoolPtr(true),
				Chave:   ptr.StringPtr("metadata_chave"),
				Double:  ptr.Float64Ptr(10.5),
				Int:     ptr.IntPtr(1),
			},
		}

		mockRepo.EXPECT().Create(gomock.Any()).DoAndReturn(func(org model.Organization) (*model.OrganizationResponse, error) {
			assert.Equal(t, "Gislason LLC", org.LegalName)
			assert.Equal(t, "The ledger.io", org.DoingBusinessAs)
			assert.Equal(t, "48784548000104", org.LegalDocument)

			assert.Equal(t, "Avenida Paulista, 1234", *org.Address.Line1)
			assert.Equal(t, "CJ 203", *org.Address.Line2)
			assert.Equal(t, "04696040", *org.Address.ZipCode)
			assert.Equal(t, "West Kennediberg", *org.Address.City)
			assert.Equal(t, "CZ", *org.Address.State)
			assert.Equal(t, "MG", org.Address.Country)

			assert.Equal(t, "ACTIVE", *org.Status.Code)
			assert.Equal(t, "Teste Ledger", org.Status.Description)

			assert.Equal(t, "1YLHctiipHZupwrT5sGwuYbks5rn64bm", *org.Metadata.Bitcoin)
			assert.Equal(t, true, *org.Metadata.Boolean)
			assert.Equal(t, "metadata_chave", *org.Metadata.Chave)
			assert.Equal(t, 10.5, *org.Metadata.Double)
			assert.Equal(t, 1, *org.Metadata.Int)

			return &gotOrg, nil
		})

		err := cmd.Execute()
		assert.NoError(t, err)

		output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The organization_id 123 has been successfully created")
	})

	t.Run("error field required", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockOrganization(ctrl)

		errorFieldRequired := errors.New("the field cannot be empty")
		orgFactory := factoryOrganizationCreate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoOrganiztion: mockRepo,
			tuiInput: func(message string) (string, error) {
				return "", errorFieldRequired
			},
			flags: flags{
				LegalName:       "Gislason LLC",
				DoingBusinessAs: "The ledger.io",
				Code:            "ACTIVE",
				Description:     "Teste Ledger",
				Line1:           "Avenida Paulista, 1234",
				Line2:           "CJ 203",
				ZipCode:         "04696040",
				City:            "West Kennediberg",
				State:           "CZ",
				Country:         "MG",
				Chave:           "metadata_chave",
				Bitcoin:         "1YLHctiipHZupwrT5sGwuYbks5rn64bm",
				Boolean:         "true",
				Double:          "10.5",
				Int:             "1",
			},
		}

		cmd := newCmdOrganizationCreate(&orgFactory)
		cmd.SetArgs([]string{
			"--legal-name", "Gislason LLC",
			"--doing-business-as", "The ledger.io",
			"--code", "ACTIVE",
			"--description", "Teste Ledger",
			"--line1", "Avenida Paulista, 1234",
			"--line2", "CJ 203",
			"--zip-code", "04696040",
			"--city", "West Kennediberg",
			"--state", "CZ",
			"--country", "MG",
			"--bitcoin", "1YLHctiipHZupwrT5sGwuYbks5rn64bm",
			"--boolean", "true",
			"--chave", "metadata_chave",
			"--double", "10.5",
			"--int", "1",
		})

		err := cmd.Execute()
		assert.EqualError(t, err, "the field cannot be empty")
	})
}
