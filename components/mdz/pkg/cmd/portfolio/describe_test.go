package portfolio

import (
	"bytes"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
	"gotest.tools/golden"
)

// \1 performs an operation
func Test_newCmdPortfolioDescribe(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockPortfolio(ctrl)

		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		portfolioID := "01931b44-6e33-791a-bfad-27992fa15984"

		ledFactory := factoryPortfolioDescribe{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoPortfolio:  mockRepo,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Out:            "",
			JSON:           false,
		}

		metadata := map[string]any{
			"chave1": "valor1",
			"chave2": 2,
			"chave3": true,
		}

		cmd := newCmdPortfolioDescribe(&ledFactory)
		cmd.SetArgs([]string{
			"--ledger-id", ledgerID,
			"--organization-id", organizationID,
			"--portfolio-id", portfolioID,
		})

		item := mmodel.Portfolio{
			ID:             portfolioID,
			Name:           "2Real",
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Status: mmodel.Status{
				Code:        "ACTIVE",
				Description: ptr.StringPtr("Teste Ledger"),
			},
			CreatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
			UpdatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
			DeletedAt: nil,
			Metadata:  metadata,
		}

		mockRepo.EXPECT().GetByID(gomock.Any(), gomock.Any(), gomock.Any()).Return(&item, nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := ledFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
		golden.AssertBytes(t, output, "output_describe.golden")
	})

	t.Run("happy path informing no flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockPortfolio(ctrl)

		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		portfolioID := "01931b44-6e33-791a-bfad-27992fa15984"

		ledFactory := factoryPortfolioDescribe{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoPortfolio:  mockRepo,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Out:            "",
			JSON:           false,
		}

		metadata := map[string]any{
			"chave1": "valor1",
			"chave2": 2,
			"chave3": true,
		}

		ledFactory.tuiInput = func(message string) (string, error) {
			return "01933f96-ed04-7c57-be5b-c091388830f8", nil
		}

		cmd := newCmdPortfolioDescribe(&ledFactory)
		cmd.SetArgs([]string{})

		item := mmodel.Portfolio{
			ID:             portfolioID,
			Name:           "2Real",
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Status: mmodel.Status{
				Code:        "ACTIVE",
				Description: ptr.StringPtr("Teste Ledger"),
			},
			CreatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
			UpdatedAt: time.Date(2024, 10, 31, 11, 23, 45, 165229000, time.UTC),
			DeletedAt: nil,
			Metadata:  metadata,
		}

		mockRepo.EXPECT().GetByID(gomock.Any(), gomock.Any(), gomock.Any()).Return(&item, nil)

		err := cmd.Execute()
		assert.NoError(t, err)

		output := ledFactory.factory.IOStreams.Out.(*bytes.Buffer).Bytes()
		golden.AssertBytes(t, output, "output_describe.golden")
	})
}
