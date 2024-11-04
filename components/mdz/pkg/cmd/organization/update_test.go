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

func TestNewCmdOrganizationUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockOrganization(ctrl)

	orgFactory := factoryOrganizationUpdate{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoOrganiztion: mockRepo,
		tuiInput: func(message string) (string, error) {
			return "name", nil
		},
		flagsUpdate: flagsUpdate{
			LegalName:       "Test Organization",
			DoingBusinessAs: "The ledger.io",
			LegalDocument:   "48784548000104",
			Code:            "ACTIVE",
			Description:     "Teste Ledger",
			Country:         "BR",
		},
	}

	cmd := newCmdOrganizationUpdate(&orgFactory)
	cmd.SetArgs([]string{
		"--organization-id", "123",
		"--legal-name", "Test Organization",
		"--doing-business-as", "The ledger.io",
		"--legal-document", "48784548000104",
		"--description", "Test Ledger",
		"--country", "BR",
	})

	gotOrg := &model.OrganizationItem{
		ID:              "123",
		LegalName:       "Test Organization",
		DoingBusinessAs: "The ledger.io",
		LegalDocument:   "48784548000104",
		Address: model.Address{
			Country: "BR",
		},
		Status: &model.Status{
			Description: ptr.StringPtr("Test Ledger"),
		},
	}

	mockRepo.EXPECT().Update(gomock.Any(), gomock.Any()).Return(gotOrg, nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The Organization 123 has been successfully updated.")
}
