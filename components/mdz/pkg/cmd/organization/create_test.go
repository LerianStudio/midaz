package organization

import (
	"bytes"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
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
		repoOrganization: mockRepo,
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
			Metadata:        "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
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
		"--metadata", "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
	})

	metadata := map[string]any{
		"chave1": "valor1",
		"chave2": 2,
		"chave3": true,
	}

	gotOrg := &mmodel.Organization{
		ID:              "123",
		LegalName:       "Test Organization",
		DoingBusinessAs: ptr.StringPtr("The ledger.io"),
		LegalDocument:   "48784548000104",
		Address: mmodel.Address{
			Country: "BR",
		},
		Status: mmodel.Status{
			Description: ptr.StringPtr("Test Ledger"),
		},
		Metadata: metadata,
	}

	mockRepo.EXPECT().Create(gomock.Any()).Return(gotOrg, nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The organization_id 123 has been successfully created")
}
