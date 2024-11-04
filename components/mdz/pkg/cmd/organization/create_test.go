package organization

import (
	"bytes"
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
		flagsCreate: flagsCreate{
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

	gotOrg := model.OrganizationCreate{
		ID:              "123",
		LegalName:       "Test Organization",
		DoingBusinessAs: "The ledger.io",
		LegalDocument:   "48784548000104",
		Address: model.Address{
			Country: "BR",
		},
		Status: model.Status{
			Description: ptr.StringPtr("Test Ledger"),
		},
	}

	mockRepo.EXPECT().Create(gomock.Any()).DoAndReturn(func(org model.Organization) (*model.OrganizationCreate, error) {
		assert.Equal(t, "Test Organization", org.LegalName)
		assert.Equal(t, "The ledger.io", org.DoingBusinessAs)
		assert.Equal(t, "48784548000104", org.LegalDocument)
		assert.Equal(t, "BR", org.Address.Country)
		assert.Equal(t, ptr.StringPtr("Test Ledger"), org.Status.Description)

		return &gotOrg, nil
	})

	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The organization_id 123 has been successfully created")
}
