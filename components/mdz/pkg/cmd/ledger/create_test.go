package ledger

import (
	"bytes"
	"testing"

	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func Test_newCmdLedgerCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockLedger(ctrl)

	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"

	name := "Romaguera and Sons"
	code := "ACTIVE"
	description := "Teste Ledger"

	metadata := map[string]any{
		"chave1": "valor1",
		"chave2": 2,
		"chave3": true,
	}

	orgFactory := factoryLedgerCreate{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoLedger: mockRepo,
		tuiInput: func(message string) (string, error) {
			return name, nil
		},
		flagsCreate: flagsCreate{
			OrganizationID: organizationID,
			Name:           name,
			Code:           code,
			Description:    description,
			Metadata:       "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
		},
	}

	cmd := newCmdLedgerCreate(&orgFactory)
	cmd.SetArgs([]string{
		"--organization-id", organizationID,
		"--name", name,
		"--code", code,
		"--description", description,
		"--metadata", "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
	})

	result := &mmodel.Ledger{
		ID:             ledgerID,
		Name:           name,
		OrganizationID: organizationID,
		Status: mmodel.Status{
			Code:        code,
			Description: ptr.StringPtr(description),
		},
		Metadata: metadata,
	}

	mockRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(result, nil)
	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The ledger_id 0192e251-328d-7390-99f5-5c54980115ed has been successfully created")

}
