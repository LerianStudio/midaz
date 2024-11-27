package ledger

import (
	"bytes"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdLedgerDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockLedger(ctrl)

	orgFactory := factoryLedgerDelete{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoLedger:     mockRepo,
		organizationID: "321",
		ledgerID:       "123",
	}

	cmd := newCmdLedgerDelete(&orgFactory)
	cmd.SetArgs([]string{
		"--organization-id", "321",
		"--ledger-id", "123",
	})

	mockRepo.EXPECT().Delete(gomock.Any(), gomock.Any()).Return(nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The Ledger 123 has been successfully deleted.")
}
