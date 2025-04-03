package account

import (
	"bytes"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/components/mdz/pkg/ptr"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

// \1 performs an operation
func Test_newCmdAccountCreate(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockAccount(ctrl)

		accountID := "01933f96-ed04-7c57-be5b-c091388830f8"
		organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
		ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
		portfolioID := "01933f94-d329-76fe-8de0-40559c7b282d"

		entityID := "59df1ccc-6881-4557-97b8-09c4348f3b37"
		name := "Romaguera and Sons"
		Type := "creditCard"

		statusCode := "CREDIT"
		statusDescription := "Teste Account"

		metadata := map[string]any{
			"chave1": "valor1",
			"chave2": 2,
			"chave3": true,
		}

		orgFactory := factoryAccountCreate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoAccount: mockRepo,
			tuiInput: func(message string) (string, error) {
				return name, nil
			},
			flagsCreate: flagsCreate{
				OrganizationID:    organizationID,
				LedgerID:          ledgerID,
				PortfolioID:       portfolioID,
				Name:              name,
				Type:              Type,
				StatusCode:        statusCode,
				StatusDescription: statusDescription,
				Metadata:          "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
			},
		}

		cmd := newCmdAccountCreate(&orgFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--portfolio-id", portfolioID,
			"--name", name,
			"--type", Type,
			"--status-code", statusCode,
			"--status-description", statusDescription,
			"--metadata", "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
		})

		result := &mmodel.Account{
			ID:              accountID,
			Name:            name,
			ParentAccountID: nil,
			EntityID:        &entityID,
			Type:            Type,
			OrganizationID:  organizationID,
			PortfolioID:     &portfolioID,
			LedgerID:        ledgerID,
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
		assert.Contains(t, output, "The Account 01933f96-ed04-7c57-be5b-c091388830f8 has been successfully created.")
	})

	t.Run("happy path no informing flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockAccount(ctrl)

		accountID := "01933f96-ed04-7c57-be5b-c091388830f8"
		organizationID := "01933f94-67b1-794c-bb13-6b75aed7591a"
		ledgerID := "01933f94-8a8f-7a1e-b4ab-98f35a5f8d61"
		portfolioID := "01933f94-d329-76fe-8de0-40559c7b282d"

		entityID := "59df1ccc-6881-4557-97b8-09c4348f3b37"
		name := "Romaguera and Sons"
		Type := "creditCard"

		statusCode := "CREDIT"
		statusDescription := "Teste Account"

		metadata := map[string]any{
			"chave1": "valor1",
			"chave2": 2,
			"chave3": true,
		}

		orgFactory := factoryAccountCreate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoAccount: mockRepo,
			tuiInput: func(message string) (string, error) {
				return name, nil
			},
			flagsCreate: flagsCreate{
				OrganizationID:    "",
				LedgerID:          "",
				PortfolioID:       "",
				Name:              name,
				Type:              Type,
				StatusCode:        statusCode,
				StatusDescription: statusDescription,
				Metadata:          "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
			},
		}

		orgFactory.tuiInput = func(message string) (string, error) {
			return "01933f96-ed04-7c57-be5b-c091388830f8", nil
		}

		cmd := newCmdAccountCreate(&orgFactory)
		cmd.SetArgs([]string{
			"--name", name,
			"--type", Type,
			"--status-code", statusCode,
			"--status-description", statusDescription,
			"--metadata", "{\"chave1\": \"valor1\", \"chave2\": 2, \"chave3\": true}",
		})

		result := &mmodel.Account{
			ID:              accountID,
			Name:            name,
			ParentAccountID: nil,
			EntityID:        &entityID,
			Type:            Type,
			OrganizationID:  organizationID,
			PortfolioID:     &portfolioID,
			LedgerID:        ledgerID,
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
		assert.Contains(t, output, "The Account 01933f96-ed04-7c57-be5b-c091388830f8 has been successfully created.")

	})
}
