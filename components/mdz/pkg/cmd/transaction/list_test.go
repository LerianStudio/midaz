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

func Test_newCmdTransactionList(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockTransaction(ctrl)

		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
		limit := 10
		page := 1
		sortOrder := "DESC"
		startDate := "2023-01-01"
		endDate := "2023-12-31"

		trnFactory := factoryTransactionList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoTransaction: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Limit:          limit,
			Page:           page,
			SortOrder:      sortOrder,
			StartDate:      startDate,
			EndDate:        endDate,
		}

		cmd := newCmdTransactionList(&trnFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--limit", "10",
			"--page", "1",
			"--sort-order", "DESC",
			"--start-date", startDate,
			"--end-date", endDate,
		})

		result := &mmodel.Transactions{
			Data: []*mmodel.Transaction{
				{
					ID:             "01930219-2c25-7a37-a5b9-610d44ae0a27",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Type:           "TRANSFER",
					Description:    "Test transaction 1",
					Status:         "COMPLETED",
				},
				{
					ID:             "01930219-2c25-7a37-a5b9-610d44ae0a28",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Type:           "WITHDRAWAL",
					Description:    "Test transaction 2",
					Status:         "PENDING",
				},
			},
			Limit: limit,
			Page:  page,
		}

		mockRepo.EXPECT().Get(
			organizationID, ledgerID, limit, page, sortOrder, startDate, endDate,
		).Return(result, nil)
		
		err := cmd.Execute()
		assert.NoError(t, err)

		output := trnFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "01930219-2c25-7a37-a5b9-610d44ae0a27")
		assert.Contains(t, output, "01930219-2c25-7a37-a5b9-610d44ae0a28")
	})

	t.Run("happy path without informing flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockTransaction(ctrl)

		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
		limit := 10
		page := 1

		trnFactory := factoryTransactionList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoTransaction: mockRepo,
			Limit: limit,
			Page:  page,
			SortOrder: "desc", // Default value from list.go
		}

		callCount := 0
		trnFactory.tuiInput = func(message string) (string, error) {
			callCount++
			if callCount == 1 {
				return organizationID, nil
			}
			return ledgerID, nil
		}

		cmd := newCmdTransactionList(&trnFactory)
		cmd.SetArgs([]string{})

		result := &mmodel.Transactions{
			Data: []*mmodel.Transaction{
				{
					ID:             "01930219-2c25-7a37-a5b9-610d44ae0a27",
					OrganizationID: organizationID,
					LedgerID:       ledgerID,
					Type:           "TRANSFER", 
					Description:    "Test transaction 1",
					Status:         "COMPLETED",
				},
			},
			Limit: limit,
			Page:  page,
		}

		mockRepo.EXPECT().Get(
			organizationID, ledgerID, limit, page, "desc", "", "",
		).Return(result, nil)
		
		err := cmd.Execute()
		assert.NoError(t, err)

		output := trnFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "01930219-2c25-7a37-a5b9-610d44ae0a27")
	})
}