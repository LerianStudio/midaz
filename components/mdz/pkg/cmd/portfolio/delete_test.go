package portfolio

import (
	"bytes"
	"go.uber.org/mock/gomock"
	"testing"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdPortfolioDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repository.NewMockPortfolio(ctrl)

	portfolioFactory := factoryPortfolioDelete{
		factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
			Out: &bytes.Buffer{},
			Err: &bytes.Buffer{},
		}},
		repoPortfolio:  mockRepo,
		OrganizationID: "321",
		LedgerID:       "123",
		PortfolioID:    "444",
	}

	cmd := newCmdPortfolioDelete(&portfolioFactory)
	cmd.SetArgs([]string{
		"--organization-id", "321",
		"--ledger-id", "123",
		"--portfolio-id", "444",
	})

	mockRepo.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err := cmd.Execute()
	assert.NoError(t, err)

	output := portfolioFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
	assert.Contains(t, output, "The Portfolio ID 444 has been successfully deleted.")
}
