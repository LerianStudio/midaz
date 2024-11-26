package account

import (
	"bytes"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdAccountDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockAccount(ctrl)

	orgFactory := factoryAccountDelete{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoAccount:    mockRepo,
		OrganizationID: "321",
		LedgerID:       "123",
		AccountID:      "444",
	}

	cmd := newCmdAccountDelete(&orgFactory)
	cmd.SetArgs([]string{
		"--organization-id", "321",
		"--ledger-id", "123",
		"--portfolio-id", "12312",
		"--account-id", "444",
	})

	mockRepo.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := orgFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The Account ID 444 has been successfully deleted.")
}
