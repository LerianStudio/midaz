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

func Test_newCmdOperationList(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockOperation(ctrl)

		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
		limit := 2
		page := 1

		operationFactory := factoryOperationList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoOperation: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			flagsListAll: flagsListAll{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Limit:          limit,
				Page:           page,
			},
		}

		cmd := newCmdOperationList(&operationFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--limit", "2",
			"--page", "1",
		})

		result := &mmodel.Operations{
			Items: []mmodel.Operation{
				{
					ID:             "01932167-d43f-8g8c-d2ge-f7h848e64c94",
					TransactionID:  "01932161-h6df-8g2c-b83g-74ee8g7405f4",
					AccountID:      "01932159-f4bd-7e0a-971e-52cc6e528312",
					AssetCode:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Type:           "DEBIT",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status: mmodel.OperationStatus{
						Code:        "COMPLETED",
						Description: ptr.StringPtr("Operation completed successfully"),
					},
					CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
				{
					ID:             "01932168-e54g-9h9d-e3hf-g8i959f75d05",
					TransactionID:  "01932161-h6df-8g2c-b83g-74ee8g7405f4",
					AccountID:      "01932160-g5ce-7f1b-982f-63dd7f639423",
					AssetCode:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Type:           "CREDIT",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status: mmodel.OperationStatus{
						Code:        "COMPLETED",
						Description: ptr.StringPtr("Operation completed successfully"),
					},
					CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
			},
			Limit: limit,
			Page:  page,
		}

		mockRepo.EXPECT().Get(organizationID, ledgerID, limit, page, "", "", "").Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := operationFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "01932167-d43f-8g8c-d2ge-f7h848e64c94")
		assert.Contains(t, output, "01932168-e54g-9h9d-e3hf-g8i959f75d05")
		assert.Contains(t, output, "DEBIT")
		assert.Contains(t, output, "CREDIT")
		assert.Contains(t, output, "500.00")
	})

	t.Run("happy path with account-id flag", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockOperation(ctrl)

		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
		accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"
		limit := 2
		page := 1

		operationFactory := factoryOperationList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoOperation: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			flagsListAll: flagsListAll{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Limit:          limit,
				Page:           page,
			},
		}

		cmd := newCmdOperationList(&operationFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--account-id", accountID,
			"--limit", "2",
			"--page", "1",
		})

		result := &mmodel.Operations{
			Items: []mmodel.Operation{
				{
					ID:             "01932167-d43f-8g8c-d2ge-f7h848e64c94",
					TransactionID:  "01932161-h6df-8g2c-b83g-74ee8g7405f4",
					AccountID:      accountID,
					AssetCode:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Type:           "DEBIT",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status: mmodel.OperationStatus{
						Code:        "COMPLETED",
						Description: ptr.StringPtr("Operation completed successfully"),
					},
					CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
				{
					ID:             "01932169-f65h-0i0e-f4ig-h9j060g86e16",
					TransactionID:  "01932162-i7eg-9h3d-c94h-85ff9h8516g5",
					AccountID:      accountID,
					AssetCode:        "01930365-4d46-7a09-a503-b932714f85af",
					Type:           "CREDIT",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status: mmodel.OperationStatus{
						Code:        "COMPLETED",
						Description: ptr.StringPtr("Operation completed successfully"),
					},
					CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
			},
			Limit: limit,
			Page:  page,
		}

		mockRepo.EXPECT().GetByAccount(organizationID, ledgerID, accountID, limit, page, "", "", "").Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := operationFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "01932167-d43f-8g8c-d2ge-f7h848e64c94")
		assert.Contains(t, output, "01932169-f65h-0i0e-f4ig-h9j060g86e16")
		assert.Contains(t, output, "DEBIT")
		assert.Contains(t, output, "CREDIT")
		assert.Contains(t, output, "500.00")
		assert.Contains(t, output, "200.50")
	})

	t.Run("happy path without flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockOperation(ctrl)

		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
		limit := 10
		page := 1

		inputCounter := 0
		inputResponses := []string{organizationID, ledgerID}

		operationFactory := factoryOperationList{
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
			flagsListAll: flagsListAll{
				Limit: limit,
				Page:  page,
			},
		}

		cmd := newCmdOperationList(&operationFactory)

		result := &mmodel.Operations{
			Items: []mmodel.Operation{
				{
					ID:             "01932167-d43f-8g8c-d2ge-f7h848e64c94",
					TransactionID:  "01932161-h6df-8g2c-b83g-74ee8g7405f4",
					AccountID:      "01932159-f4bd-7e0a-971e-52cc6e528312",
					AssetCode:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Type:           "DEBIT",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status: mmodel.OperationStatus{
						Code:        "COMPLETED",
						Description: ptr.StringPtr("Operation completed successfully"),
					},
					CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
				{
					ID:             "01932168-e54g-9h9d-e3hf-g8i959f75d05",
					TransactionID:  "01932161-h6df-8g2c-b83g-74ee8g7405f4",
					AccountID:      "01932160-g5ce-7f1b-982f-63dd7f639423",
					AssetCode:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Type:           "CREDIT",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Status: mmodel.OperationStatus{
						Code:        "COMPLETED",
						Description: ptr.StringPtr("Operation completed successfully"),
					},
					CreatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt: time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
			},
			Limit: limit,
			Page:  page,
		}

		mockRepo.EXPECT().Get(organizationID, ledgerID, limit, page, "", "", "").Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := operationFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "01932167-d43f-8g8c-d2ge-f7h848e64c94")
		assert.Contains(t, output, "01932168-e54g-9h9d-e3hf-g8i959f75d05")
		assert.Contains(t, output, "DEBIT")
		assert.Contains(t, output, "CREDIT")
		assert.Contains(t, output, "500.00")
	})
}