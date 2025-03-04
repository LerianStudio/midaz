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

func Test_newCmdTransactionUpdate(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockTransaction(ctrl)

		transactionID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		description := "Updated transaction description"
		status := "COMPLETED"

		metadata := map[string]any{
			"key1": "value1",
			"key2": 2,
			"key3": true,
		}

		trnFactory := factoryTransactionUpdate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoTransaction: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			flagsUpdate: flagsUpdate{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				TransactionID:  transactionID,
				Description:    description,
				Status:         status,
				Metadata:       "{\"key1\": \"value1\", \"key2\": 2, \"key3\": true}",
			},
		}

		cmd := newCmdTransactionUpdate(&trnFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--transaction-id", transactionID,
			"--description", description,
			"--status", status,
			"--metadata", "{\"key1\": \"value1\", \"key2\": 2, \"key3\": true}",
		})

		result := &mmodel.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Description:    description,
			Status:         status,
			Metadata:       metadata,
		}

		mockRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := trnFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Transaction 01930219-2c25-7a37-a5b9-610d44ae0a27 has been successfully updated.")
	})

	t.Run("happy path without informing organization and ledger flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockTransaction(ctrl)

		transactionID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		description := "Updated transaction description"
		status := "COMPLETED"

		metadata := map[string]any{
			"key1": "value1",
			"key2": 2,
			"key3": true,
		}

		trnFactory := factoryTransactionUpdate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoTransaction: mockRepo,
			flagsUpdate: flagsUpdate{
				TransactionID: transactionID,
				Description:   description,
				Status:        status,
				Metadata:      "{\"key1\": \"value1\", \"key2\": 2, \"key3\": true}",
			},
		}

		callCount := 0
		trnFactory.tuiInput = func(message string) (string, error) {
			callCount++
			if callCount == 1 {
				return organizationID, nil
			}
			return ledgerID, nil
		}

		cmd := newCmdTransactionUpdate(&trnFactory)
		cmd.SetArgs([]string{
			"--transaction-id", transactionID,
			"--description", description,
			"--status", status,
			"--metadata", "{\"key1\": \"value1\", \"key2\": 2, \"key3\": true}",
		})

		result := &mmodel.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Description:    description,
			Status:         status,
			Metadata:       metadata,
		}

		mockRepo.EXPECT().Update(organizationID, ledgerID, transactionID, gomock.Any()).Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := trnFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Transaction 01930219-2c25-7a37-a5b9-610d44ae0a27 has been successfully updated.")
	})
}