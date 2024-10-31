package ledger

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

func Test_newCmdLedgerCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockLedger(ctrl)

	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"

	name := "Romaguera and Sons"
	code := "ACTIVE"
	description := "Teste Ledger"
	bitcoin := "1iR2KqpxRFjLsPUpWmpADMC7piRNsMAAjq"
	bool := "false"
	chave := "metadata_chave"
	double := "10.5"
	int := "1"

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
			Bitcoin:        bitcoin,
			Boolean:        bool,
			Chave:          chave,
			Double:         double,
			Int:            int,
		},
	}

	cmd := newCmdLedgerCreate(&orgFactory)
	cmd.SetArgs([]string{
		"--organization-id", organizationID,
		"--name", name,
		"--code", code,
		"--description", description,
		"--bitcoin", bitcoin,
		"--boolean", bool,
		"--chave", chave,
		"--double", double,
		"--int", int,
	})

	result := &model.LedgerCreate{
		ID:             ledgerID,
		Name:           name,
		OrganizationID: organizationID,
		Status: model.LedgerStatus{
			Code:        ptr.StringPtr(code),
			Description: ptr.StringPtr(description),
		},
		Metadata: model.LedgerMetadata{
			Bitcoin: ptr.StringPtr(bitcoin),
			Boolean: ptr.BoolPtr(true),
			Chave:   ptr.StringPtr(chave),
			Double:  ptr.Float64Ptr(10.5),
			Int:     ptr.IntPtr(1),
		},
	}

	mockRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(result, nil)
	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The ledger_id 0192e251-328d-7390-99f5-5c54980115ed has been successfully created")

}
