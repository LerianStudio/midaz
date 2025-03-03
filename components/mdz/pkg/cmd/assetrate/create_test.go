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

func Test_newCmdAssetRateCreate(t *testing.T) {
	t.Run("happy path informing all the necessary flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockAssetRate(ctrl)

		externalID := "ext-rate-12345"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		from := "USD"
		to := "BRL"
		rate := "497"
		scale := "2"
		source := "API"
		ttl := "86400"

		metadata := map[string]any{
			"provider": "central-bank",
			"market":   "spot",
			"valid":    true,
		}

		rateFactory := factoryAssetRateCreate{
			factory: &factory.Factory{IOStreams: &iostreams.IOStreams{
				Out: &bytes.Buffer{},
				Err: &bytes.Buffer{},
			}},
			repoAssetRate: mockRepo,
			tuiInput: func(message string) (string, error) {
				return from, nil
			},
			flagsCreate: flagsCreate{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				From:           from,
				To:             to,
				Rate:           rate,
				Scale:          scale,
				Source:         source,
				TTL:            ttl,
				ExternalID:     externalID,
				Metadata:       "{\"provider\": \"central-bank\", \"market\": \"spot\", \"valid\": true}",
			},
		}

		cmd := newCmdAssetRateCreate(&rateFactory)
		cmd.SetArgs([]string{
			"--organization-id", organizationID,
			"--ledger-id", ledgerID,
			"--from", from,
			"--to", to,
			"--rate", rate,
			"--scale", scale,
			"--source", source,
			"--ttl", ttl,
			"--external-id", externalID,
			"--metadata", "{\"provider\": \"central-bank\", \"market\": \"spot\", \"valid\": true}",
		})

		rateInt := 497
		scaleInt := 2
		ttlInt := 86400
		now := time.Now().UTC()

		result := &mmodel.AssetRate{
			AssetCode:       from,
			TargetAssetCode: to,
			Value:           float64(rateInt) / float64(scaleInt*10),
			ExternalID:      externalID,
			OrganizationID:  organizationID,
			LedgerID:        ledgerID,
			Source:          mpointers.String(source),
			TTL:             mpointers.Int(ttlInt),
			Date:            now,
			CreatedAt:       now,
			UpdatedAt:       now,
			Metadata:        metadata,
		}

		mockRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := rateFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The AssetRate has been successfully created.")
	})

	t.Run("happy path without flags", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRepo := repository.NewMockAssetRate(ctrl)

		externalID := "ext-rate-12345"
		organizationID := "0192e250-ed9d-7e5c-a614-9b294151b572"
		ledgerID := "0192e251-328d-7390-99f5-5c54980115ed"

		from := "USD"
		to := "BRL"
		rate := "497"

		metadata := map[string]any{
			"provider": "central-bank",
			"market":   "spot",
			"valid":    true,
		}

		inputCounter := 0
		inputResponses := []string{organizationID, ledgerID, from, to, rate}

		rateFactory := factoryAssetRateCreate{
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
			flagsCreate: flagsCreate{
				Metadata: "{\"provider\": \"central-bank\", \"market\": \"spot\", \"valid\": true}",
			},
		}

		cmd := newCmdAssetRateCreate(&rateFactory)
		cmd.SetArgs([]string{
			"--metadata", "{\"provider\": \"central-bank\", \"market\": \"spot\", \"valid\": true}",
		})

		now := time.Now().UTC()

		result := &mmodel.AssetRate{
			AssetCode:       from,
			TargetAssetCode: to,
			Value:           4.97,
			ExternalID:      externalID,
			OrganizationID:  organizationID,
			LedgerID:        ledgerID,
			Date:            now,
			CreatedAt:       now,
			UpdatedAt:       now,
			Metadata:        metadata,
		}

		mockRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(result, nil)
		err := cmd.Execute()
		assert.NoError(t, err)

		output := rateFactory.factory.IOStreams.Out.(*bytes.Buffer).String()
		assert.Contains(t, output, "The AssetRate has been successfully created.")
	})
}