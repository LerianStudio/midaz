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

func Test_newCmdTransactionDescribe(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockTransaction(ctrl)

		transactionID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		trnFactory := factoryTransactionDescribe{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoTransaction: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			flagsDescribe: flagsDescribe{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				TransactionID:  transactionID,
			},
		}

		cmd := newCmdTransactionDescribe(&trnFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--transaction-id", transactionID,
		})

		result := &mmodel.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Type:           "TRANSFER",
			Description:    "Test transaction",
			Status:         "COMPLETED",
		}

		mockRepo.EXPECT().GetByID(gomock.Any(), gomock.Any(), gomock.Any()).Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := trnFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, transactionID)
	})

	t.Run("happy path without informing flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockTransaction(ctrl)

		transactionID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		trnFactory := factoryTransactionDescribe{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoTransaction: mockRepo,
			flagsDescribe:   flagsDescribe{},
		}

		callCount := 0
		trnFactory.tuiInput = func(message string) (string, error) {
			callCount++
			switch callCount {
			case 1:
				return organizationID, nil
			case 2:
				return ledgerID, nil
			default:
				return transactionID, nil
			}
		}

		cmd := newCmdTransactionDescribe(&trnFactory)
		cmd.SetArgs([]string{})

		result := &mmodel.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Type:           "TRANSFER",
			Description:    "Test transaction",
			Status:         "COMPLETED",
		}

		mockRepo.EXPECT().GetByID(organizationID, ledgerID, transactionID).Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := trnFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, transactionID)
	})
}