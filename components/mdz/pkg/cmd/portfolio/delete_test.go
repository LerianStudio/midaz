package portfolio

import (
	"bytes"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

// \1 performs an operation
func Test_newCmdPortfolioDelete(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
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
		assert.Contains(t, output, "The Portfolio 444 has been successfully deleted.")
	})

	t.Run("happy path informing no flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockPortfolio(ctrl)

		portfolioFactory := factoryPortfolioDelete{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoPortfolio: mockRepo,
		}

		portfolioFactory.tuiInput = func(message string) (string, error) {
			return "01933f96-ed04-7c57-be5b-c091388830f8", nil
		}

		cmd := newCmdPortfolioDelete(&portfolioFactory)
		cmd.SetArgs([]string{})

		mockRepo.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := portfolioFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Portfolio 01933f96-ed04-7c57-be5b-c091388830f8 has been successfully deleted.")
	})
}
