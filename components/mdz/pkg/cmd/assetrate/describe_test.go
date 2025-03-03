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

func Test_newCmdAssetRateDescribe(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockAssetRate(ctrl)

		externalID := "ext-rate-12345"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
		assetCode := "USD"
		targetAssetCode := "BRL"
		source := "API"
		value := 4.97
		now := time.Now().UTC()

		rateFactory := factoryAssetRateDescribe{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoAssetRate: mockRepo,
			tuiInput: func(message string) (string, error) {
				return organizationID, nil
			},
			flagsDescribe: flagsDescribe{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				ExternalID:     externalID,
			},
		}

		cmd := newCmdAssetRateDescribe(&rateFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--external-id", externalID,
		})

		result := &mmodel.AssetRate{
			AssetCode:       assetCode,
			TargetAssetCode: targetAssetCode,
			Value:           value,
			ExternalID:      externalID,
			OrganizationID:  organizationID,
			LedgerID:        ledgerID,
			Source:          mpointers.String(source),
			TTL:             mpointers.Int(86400),
			Date:            now,
			CreatedAt:       now,
			UpdatedAt:       now,
			Metadata: map[string]any{
				"provider": "central-bank",
				"market":   "spot",
			},
		}

		mockRepo.EXPECT().GetByExternalID(organizationID, ledgerID, externalID).Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := rateFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, externalID)
		assert.Contains(t, output, assetCode)
		assert.Contains(t, output, targetAssetCode)
		assert.Contains(t, output, source)
	})

	t.Run("happy path without flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockAssetRate(ctrl)

		externalID := "ext-rate-12345"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"
		assetCode := "USD"
		targetAssetCode := "BRL"
		source := "API"
		value := 4.97
		now := time.Now().UTC()

		inputCounter := 0
		inputResponses := []string{organizationID, ledgerID, externalID}

		rateFactory := factoryAssetRateDescribe{
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
		}

		cmd := newCmdAssetRateDescribe(&rateFactory)

		result := &mmodel.AssetRate{
			AssetCode:       assetCode,
			TargetAssetCode: targetAssetCode,
			Value:           value,
			ExternalID:      externalID,
			OrganizationID:  organizationID,
			LedgerID:        ledgerID,
			Source:          mpointers.String(source),
			TTL:             mpointers.Int(86400),
			Date:            now,
			CreatedAt:       now,
			UpdatedAt:       now,
			Metadata: map[string]any{
				"provider": "central-bank",
				"market":   "spot",
			},
		}

		mockRepo.EXPECT().GetByExternalID(organizationID, ledgerID, externalID).Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := rateFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, externalID)
		assert.Contains(t, output, assetCode)
		assert.Contains(t, output, targetAssetCode)
		assert.Contains(t, output, source)
	})
}