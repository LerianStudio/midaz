package balance

import (
	"bytes"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdBalanceDelete(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockBalance(ctrl)

		balanceID := "01932165-b21d-7e6a-b0fc-d5f625c42a72"
		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"

		balanceFactory := factoryBalanceDelete{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoBalance: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			flagsDelete: flagsDelete{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				BalanceID:      balanceID,
			},
		}

		cmd := newCmdBalanceDelete(&balanceFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--balance-id", balanceID,
		})

		mockRepo.EXPECT().Delete(organizationID, ledgerID, balanceID).Return(nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := balanceFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Balance has been successfully deleted.")
	})

	t.Run("happy path without flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockBalance(ctrl)

		balanceID := "01932165-b21d-7e6a-b0fc-d5f625c42a72"
		organizationID := "0192fc1d-f34d-78c9-9654-83e497349241"
		ledgerID := "01930218-bfb7-74fe-ba00-e52a17e9fb4e"

		inputCounter := 0
		inputResponses := []string{organizationID, ledgerID, balanceID}

		balanceFactory := factoryBalanceDelete{
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

		cmd := newCmdBalanceDelete(&balanceFactory)

		mockRepo.EXPECT().Delete(organizationID, ledgerID, balanceID).Return(nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := balanceFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The Balance has been successfully deleted.")
	})
}