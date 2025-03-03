package balance

import (
	"bytes"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdBalanceList(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockBalance(ctrl)

		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
		limit := 2
		page := 1

		balanceFactory := factoryBalanceList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoBalance: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			flagsList: flagsList{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				Limit:          limit,
				Page:           page,
			},
		}

		cmd := newCmdBalanceList(&balanceFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--limit", "2",
			"--page", "1",
		})

		result := &mmodel.Balances{
			Items: []mmodel.Balance{
				{
					ID:             "01932165-b21d-7e6a-b0fc-d5f625c42a72",
					AccountID:      "01932159-f4bd-7e0a-971e-52cc6e528312",
					AssetID:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Amount:         "1000.00",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
				{
					ID:             "01932166-c32e-7f7b-c1fd-e6g737d53b83",
					AccountID:      "01932160-g5ce-7f1b-982f-63dd7f639423",
					AssetID:        "01930365-4d46-7a09-a503-b932714f85af",
					Amount:         "2500.50",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
			},
			Limit: limit,
			Page:  page,
		}

		mockRepo.EXPECT().Get(organizationID, ledgerID, limit, page, "", "", "").Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := balanceFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "01932165-b21d-7e6a-b0fc-d5f625c42a72")
		assert.Contains(t, output, "01932166-c32e-7f7b-c1fd-e6g737d53b83")
		assert.Contains(t, output, "1000.00")
		assert.Contains(t, output, "2500.50")
	})

	t.Run("happy path with account-id flag", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockBalance(ctrl)

		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
		accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"
		limit := 2
		page := 1

		balanceFactory := factoryBalanceList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoBalance: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			flagsList: flagsList{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Limit:          limit,
				Page:           page,
			},
		}

		cmd := newCmdBalanceList(&balanceFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--account-id", accountID,
			"--limit", "2",
			"--page", "1",
		})

		result := &mmodel.Balances{
			Items: []mmodel.Balance{
				{
					ID:             "01932165-b21d-7e6a-b0fc-d5f625c42a72",
					AccountID:      accountID,
					AssetID:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Amount:         "1000.00",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
				{
					ID:             "01932166-c32e-7f7b-c1fd-e6g737d53b83",
					AccountID:      accountID,
					AssetID:        "01930365-4d46-7a09-a503-b932714f85af",
					Amount:         "500.75",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
			},
			Limit: limit,
			Page:  page,
		}

		mockRepo.EXPECT().GetByAccount(organizationID, ledgerID, accountID, limit, page, "", "", "").Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := balanceFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "01932165-b21d-7e6a-b0fc-d5f625c42a72")
		assert.Contains(t, output, "01932166-c32e-7f7b-c1fd-e6g737d53b83")
		assert.Contains(t, output, "1000.00")
		assert.Contains(t, output, "500.75")
	})

	t.Run("happy path without flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockBalance(ctrl)

		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
		limit := 10
		page := 1

		inputCounter := 0
		inputResponses := []string{organizationID, ledgerID}

		balanceFactory := factoryBalanceList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoBalance: mockRepo,
			tuiInput: func(message string) (string, error) {
				response := inputResponses[inputCounter]
				inputCounter++
				return response, nil
			},
			flagsList: flagsList{
				Limit: limit,
				Page:  page,
			},
		}

		cmd := newCmdBalanceList(&balanceFactory)

		result := &mmodel.Balances{
			Items: []mmodel.Balance{
				{
					ID:             "01932165-b21d-7e6a-b0fc-d5f625c42a72",
					AccountID:      "01932159-f4bd-7e0a-971e-52cc6e528312",
					AssetID:        "01930219-2c25-7a37-a5b9-610d44ae0a27",
					Amount:         "1000.00",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
				{
					ID:             "01932166-c32e-7f7b-c1fd-e6g737d53b83",
					AccountID:      "01932160-g5ce-7f1b-982f-63dd7f639423",
					AssetID:        "01930365-4d46-7a09-a503-b932714f85af",
					Amount:         "2500.50",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
					UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
				},
			},
			Limit: limit,
			Page:  page,
		}

		mockRepo.EXPECT().Get(organizationID, ledgerID, limit, page, "", "", "").Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := balanceFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "01932165-b21d-7e6a-b0fc-d5f625c42a72")
		assert.Contains(t, output, "01932166-c32e-7f7b-c1fd-e6g737d53b83")
		assert.Contains(t, output, "1000.00")
		assert.Contains(t, output, "2500.50")
	})
}