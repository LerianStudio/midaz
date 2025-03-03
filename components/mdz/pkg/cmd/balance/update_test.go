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

func Test_newCmdBalanceUpdate(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockBalance(ctrl)

		balanceID := "01932165-b21d-7e6a-b0fc-d5f625c42a72"
		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
		accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"
		assetID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
		amount := "1500.00"

		balanceFactory := factoryBalanceUpdate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoBalance: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			flagsUpdate: flagsUpdate{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				BalanceID:      balanceID,
				Amount:         amount,
			},
		}

		cmd := newCmdBalanceUpdate(&balanceFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--balance-id", balanceID,
			"--amount", amount,
		})

		updateInput := mmodel.UpdateBalance{
			Amount: amount,
		}

		result := &mmodel.Balance{
			ID:             balanceID,
			AccountID:      accountID,
			AssetID:        assetID,
			Amount:         amount,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
		}

		mockRepo.EXPECT().Update(organizationID, ledgerID, balanceID, gomock.Any()).Do(
			func(_ string, _ string, _ string, inp mmodel.UpdateBalance) {
				assert.Equal(t, updateInput.Amount, inp.Amount)
			}).Return(result, nil)
		
		err := cmd.Execute()
		assert.NoError(t, err)

		output := balanceFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Balance has been successfully updated.")
	})

	t.Run("happy path without flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockBalance(ctrl)

		balanceID := "01932165-b21d-7e6a-b0fc-d5f625c42a72"
		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"
		accountID := "01932159-f4bd-7e0a-971e-52cc6e528312"
		assetID := "01930219-2c25-7a37-a5b9-610d44ae0a27"
		amount := "1500.00"

		inputCounter := 0
		inputResponses := []string{organizationID, ledgerID, balanceID, amount}

		balanceFactory := factoryBalanceUpdate{
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
		}

		cmd := newCmdBalanceUpdate(&balanceFactory)

		updateInput := mmodel.UpdateBalance{
			Amount: amount,
		}

		result := &mmodel.Balance{
			ID:             balanceID,
			AccountID:      accountID,
			AssetID:        assetID,
			Amount:         amount,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			CreatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
			UpdatedAt:      time.Date(2024, 11, 06, 15, 30, 24, 421664000, time.UTC),
		}

		mockRepo.EXPECT().Update(organizationID, ledgerID, balanceID, gomock.Any()).Do(
			func(_ string, _ string, _ string, inp mmodel.UpdateBalance) {
				assert.Equal(t, updateInput.Amount, inp.Amount)
			}).Return(result, nil)
		
		err := cmd.Execute()
		assert.NoError(t, err)

		output := balanceFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Balance has been successfully updated.")
	})
}