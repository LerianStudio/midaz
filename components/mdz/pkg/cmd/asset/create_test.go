package asset

import (
	"bytes"
	"testing"

	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func Test_newCmdAssetCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockAsset(ctrl)

	assetID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

	name := "Romaguera and Sons"
	Type := "currency"
	Code := "BRL"

	statusCode := "ACTIVE"
	statusDescription := "Teste Ledger"

	metadata := map[string]any{
		"chave1": "valor1",
		"chave2": 2,
		"chave3": true,
	}

	orgFactory := factoryAssetCreate{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoAsset: mockRepo,
		tuiInput: func(message string) (string, error) {
			return name, nil
		},
		flagsCreate: flagsCreate{
			OrganizationID:    organizationID,
			LedgerID:          ledgerID,
			Name:              name,
			Type:              Type,
			Code:              Code,
			StatusCode:        statusCode,
			StatusDescription: statusDescription,
			Metadata:          "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
		},
	}

	cmd := newCmdAssetCreate(&orgFactory)
	cmd.SetArgs([]string{
		"--organization-id", organizationID,
		"--ledger-id", ledgerID,
		"--name", name,
		"--type", Type,
		"--code", Code,
		"--status-code", statusCode,
		"--status-description", statusDescription,
		"--metadata", "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
	})

	result := &mmodel.Asset{
		ID:             assetID,
		Name:           name,
		Type:           Type,
		Code:           Code,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Status: mmodel.Status{
			Code:        statusCode,
			Description: ptr.StringPtr(statusDescription),
		},
		Metadata: metadata,
	}

	mockRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(result, nil)
	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The Asset ID 01930219-2c25-7a37-a5b9-610d44ae0a27 has been successfully created")
}
