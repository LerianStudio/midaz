package assetrate

import (
	"bytes"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/components/mdz/internal/domain/repository"
	"github.com/LerianStudio/midaz/components/mdz/pkg/factory"
	"github.com/LerianStudio/midaz/components/mdz/pkg/iostreams"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mpointers"

	"github.com/stretchr/testify/assert"
)

func Test_newCmdAssetRateList(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockAssetRate(ctrl)

		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
		assetCode := "USD"
		limit := 2
		page := 1

		rateFactory := factoryAssetRateList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoAssetRate: mockRepo,
			tuiInput: func(message string) (string, error) {
				return assetCode, nil
			},
			flagsListAll: flagsListAll{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				AssetCode:      assetCode,
				Limit:          limit,
				Page:           page,
			},
		}

		cmd := newCmdAssetRateList(&rateFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--asset-code", assetCode,
			"--limit", "2",
			"--page", "1",
		})

		now := time.Now().UTC()

		result := &mmodel.AssetRates{
			Items: []mmodel.AssetRate{
				{
					From:       "USD",
					To: "BRL",
					Rate:           4.97,
					ExternalID:      "ext-rate-12345",
					OrganizationID:  organizationID,
					LedgerID:        ledgerID,
					Source:          mpointers.String("API"),
					TTL: 86400,
					CreatedAt:       now,
					UpdatedAt:       now,
				},
				{
					From:       "USD",
					To: "EUR",
					Rate:           0.92,
					ExternalID:      "ext-rate-67890",
					OrganizationID:  organizationID,
					LedgerID:        ledgerID,
					Source:          mpointers.String("API"),
					TTL: 86400,
					CreatedAt:       now,
					UpdatedAt:       now,
				},
			},
			Limit: limit,
			Page:  page,
		}

		mockRepo.EXPECT().GetByAssetCode(organizationID, ledgerID, assetCode, limit, page, "", "", "").Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := rateFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "USD")
		assert.Contains(t, output, "BRL")
		assert.Contains(t, output, "EUR")
		assert.Contains(t, output, "4.97")
		assert.Contains(t, output, "0.92")
	})

	t.Run("happy path without flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockAssetRate(ctrl)

		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
		assetCode := "USD"
		limit := 10
		page := 1

		inputCounter := 0
		inputResponses := []string{organizationID, ledgerID, assetCode}

		rateFactory := factoryAssetRateList{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoAssetRate: mockRepo,
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

		cmd := newCmdAssetRateList(&rateFactory)

		now := time.Now().UTC()

		result := &mmodel.AssetRates{
			Items: []mmodel.AssetRate{
				{
					From:       "USD",
					To: "BRL",
					Rate:           4.97,
					ExternalID:      "ext-rate-12345",
					OrganizationID:  organizationID,
					LedgerID:        ledgerID,
					Source:          mpointers.String("API"),
					TTL: 86400,
					CreatedAt:       now,
					UpdatedAt:       now,
				},
				{
					From:       "USD",
					To: "EUR",
					Rate:           0.92,
					ExternalID:      "ext-rate-67890",
					OrganizationID:  organizationID,
					LedgerID:        ledgerID,
					Source:          mpointers.String("API"),
					TTL: 86400,
					CreatedAt:       now,
					UpdatedAt:       now,
				},
			},
			Limit: limit,
			Page:  page,
		}

		mockRepo.EXPECT().GetByAssetCode(organizationID, ledgerID, assetCode, limit, page, "", "", "").Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := rateFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "USD")
		assert.Contains(t, output, "BRL")
		assert.Contains(t, output, "EUR")
		assert.Contains(t, output, "4.97")
		assert.Contains(t, output, "0.92")
	})
}