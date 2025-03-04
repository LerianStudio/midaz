package operation

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
)

func Test_newCmdOperationUpdate(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockOperation(ctrl)

		operationID := "01932167-d43f-8g8c-d2ge-f7h848e64c94"
		transactionID := "01932161-h6df-8g2c-b83g-74ee8g7405f4"
		accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"
		assetID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
		description := "Updated operation description"
		metadata := "{\"status\": \"CANCELED\"}"

		operationFactory := factoryOperationUpdate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoOperation: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			flagsUpdate: flagsUpdate{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				TransactionID:  transactionID,
				OperationID:    operationID,
				Description:    description,
				Metadata:       metadata,
			},
		}

		cmd := newCmdOperationUpdate(&operationFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--transaction-id", transactionID,
			"--operation-id", operationID,
			"--description", description,
			"--metadata", metadata,
		})

		updateInput := mmodel.UpdateOperationInput{
			Description: description,
			Metadata:    map[string]any{"status": "CANCELED"},
		}

		result := &mmodel.Operation{
			ID:             operationID,
			TransactionID:  transactionID,
			AccountID:      accountID,
			AssetCode:      assetID,
			Amount:         mmodel.Amount{},
			Type:           "DEBIT",
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Status: mmodel.OperationStatus{
				Code:        "CANCELED",
				Description: ptr.StringPtr("Operation was canceled"),
			},
			CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
		}

		mockRepo.EXPECT().Update(organizationID, ledgerID, transactionID, operationID, gomock.Any()).Do(
			func(_ string, _ string, _ string, _ string, inp mmodel.UpdateOperationInput) {
				assert.Equal(t, updateInput.Description, inp.Description)
			}).Return(result, nil)
		
		err := cmd.Execute()
		assert.NoError(t, err)

		output := operationFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Operation has been successfully updated.")
	})

	t.Run("happy path without flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockOperation(ctrl)

		operationID := "01932167-d43f-8g8c-d2ge-f7h848e64c94"
		transactionID := "01932161-h6df-8g2c-b83g-74ee8g7405f4"
		accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"
		assetID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
		description := "Updated description"

		inputCounter := 0
		inputResponses := []string{organizationID, ledgerID, transactionID, operationID, description}

		operationFactory := factoryOperationUpdate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoOperation: mockRepo,
			tuiInput: func(message string) (string, error) {
				response := inputResponses[inputCounter]
				inputCounter++
				return response, nil
			},
			flagsUpdate: flagsUpdate{
				Metadata: "{}",
			},
		}

		cmd := newCmdOperationUpdate(&operationFactory)

		result := &mmodel.Operation{
			ID:             operationID,
			TransactionID:  transactionID,
			AccountID:      accountID,
			AssetCode:      assetID,
			Amount:         mmodel.Amount{},
			Type:           "DEBIT",
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Status: mmodel.OperationStatus{
				Code:        "CANCELED",
				Description: ptr.StringPtr("Operation was canceled by user request"),
			},
			CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
		}

		mockRepo.EXPECT().Update(organizationID, ledgerID, transactionID, operationID, gomock.Any()).Return(result, nil)
		
		err := cmd.Execute()
		assert.NoError(t, err)

		output := operationFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Operation has been successfully updated.")
	})
}