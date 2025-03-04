package transaction

import (
	"bytes"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdTransactionCreate(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockTransaction(ctrl)

		transactionID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		description := "Test transfer"
		asset := "USD"
		value := int64(100)
		scale := 0
		chartGroup := "TRANSFERS"
		sourceAccount := "01930219-2c25-7a37-a5b9-610d44ae0a28"
		sourceChart := "DEBIT"
		sourceDescription := "Source operation"
		destinationAccount := "01930219-2c25-7a37-a5b9-610d44ae0a29"
		destinationChart := "CREDIT"
		destinationDescription := "Destination operation"

		metadata := map[string]any{
			"key1": "value1",
			"key2": 2,
			"key3": true,
		}

		trnFactory := factoryTransactionCreate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoTransaction: mockRepo,
			tuiInput: func(message string) (string, error) {
				return "", nil
			},
			flagsCreate: flagsCreate{
				OrganizationID:             organizationID,
				LedgerID:                   ledgerID,
				ChartOfAccountsGroupName:   chartGroup,
				Description:                description,
				Asset:                      asset,
				Value:                      value,
				Scale:                      scale,
				SourceAccount:              sourceAccount,
				SourceChartOfAccounts:      sourceChart,
				SourceDescription:          sourceDescription,
				DestinationAccount:         destinationAccount,
				DestinationChartOfAccounts: destinationChart,
				DestinationDescription:     destinationDescription,
				Metadata:                   "{\"key1\": \"value1\", \"key2\": 2, \"key3\": true}",
			},
		}

		cmd := newCmdTransactionCreate(&trnFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--chart-group", chartGroup,
			"--description", description,
			"--asset", asset,
			"--value", "100",
			"--scale", "0",
			"--source-account", sourceAccount,
			"--source-chart", sourceChart,
			"--source-description", sourceDescription,
			"--destination-account", destinationAccount,
			"--destination-chart", destinationChart,
			"--destination-description", destinationDescription,
			"--metadata", "{\"key1\": \"value1\", \"key2\": 2, \"key3\": true}",
		})

		result := &mmodel.Transaction{
			ID:             transactionID,
			Description:    description,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Metadata:       metadata,
		}

		mockRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := trnFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Transaction 01930219-2c25-7a37-a5b9-610d44ae0a27 has been successfully created.")
	})

	t.Run("happy path without informing flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockTransaction(ctrl)

		transactionID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		description := "Test transfer"
		asset := "USD"
		value := int64(100)
		scale := 0
		chartGroup := "TRANSFERS"
		sourceAccount := "01930219-2c25-7a37-a5b9-610d44ae0a28"
		sourceChart := "DEBIT"
		sourceDescription := "Source operation"
		destinationAccount := "01930219-2c25-7a37-a5b9-610d44ae0a29"
		destinationChart := "CREDIT"
		destinationDescription := "Destination operation"

		metadata := map[string]any{
			"key1": "value1",
			"key2": 2,
			"key3": true,
		}

		trnFactory := factoryTransactionCreate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoTransaction: mockRepo,
			flagsCreate: flagsCreate{
				Description:                description,
				Asset:                      asset,
				Value:                      value,
				Scale:                      scale,
				ChartOfAccountsGroupName:   chartGroup,
				SourceAccount:              sourceAccount,
				SourceChartOfAccounts:      sourceChart,
				SourceDescription:          sourceDescription,
				DestinationAccount:         destinationAccount,
				DestinationChartOfAccounts: destinationChart,
				DestinationDescription:     destinationDescription,
				Metadata:                   "{\"key1\": \"value1\", \"key2\": 2, \"key3\": true}",
			},
		}

		var callCount int
		trnFactory.tuiInput = func(message string) (string, error) {
			callCount++
			if callCount == 1 {
				return organizationID, nil
			}
			return ledgerID, nil
		}

		cmd := newCmdTransactionCreate(&trnFactory)
		cmd.SetArgs([]string{
			"--description", description,
			"--asset", asset,
			"--value", "100",
			"--chart-group", chartGroup,
			"--source-account", sourceAccount,
			"--source-chart", sourceChart,
			"--source-description", sourceDescription,
			"--destination-account", destinationAccount,
			"--destination-chart", destinationChart,
			"--destination-description", destinationDescription,
			"--metadata", "{\"key1\": \"value1\", \"key2\": 2, \"key3\": true}",
		})

		result := &mmodel.Transaction{
			ID:             transactionID,
			Description:    description,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Metadata:       metadata,
		}

		mockRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := trnFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Transaction 01930219-2c25-7a37-a5b9-610d44ae0a27 has been successfully created.")
	})
}
