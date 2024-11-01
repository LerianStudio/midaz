package ledger

import (
	"bytes"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/internal/model"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"gotest.tools/golden"
)

func Test_newCmdLedgerDescribe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockLedger(ctrl)

	ledFactory := factoryLedgerDescribe{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoLedger:     mockRepo,
		organizationID: "123",
		Out:            "",
		JSON:           false,
	}

	ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
	organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"

	cmd := newCmdLedgerDescribe(&ledFactory)
	cmd.SetArgs([]string{
		"--ledger-id", ledgerID,
		"--organization-id", organizationID,
	})

	item := model.LedgerItems{
		ID:             ledgerID,
		Name:           "Romaguera and Sons",
		OrganizationID: organizationID,
		Status: &model.LedgerStatus{
			Code:        ptr.StringPtr("ACTIVE"),
			Description: ptr.StringPtr("Teste Ledger"),
		},
		CreatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
		UpdatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
		DeletedAt: nil,
		Metadata: &model.LedgerMetadata{
			Bitcoin: ptr.StringPtr("1iR2KqpxRFjLsPUpWmpADMC7piRNsMAAjq"),
			Boolean: ptr.BoolPtr(false),
			Chave:   ptr.StringPtr("metadata_chave"),
			Double:  ptr.Float64Ptr(10.5),
			Int:     ptr.IntPtr(1),
		},
	}

	mockRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Return(&item, nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := ledFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
	golden.AssertBytes(t, output, "output_describe.golden")
}
