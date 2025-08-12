package query

import (
	"context"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// GetAssetRateByExternalID gets data in the repository.
func (uc *UseCase) GetAssetRateByExternalID(ctx context.Context, organizationID, ledgerID, externalID uuid.UUID) (*assetrate.AssetRate, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_asset_rate_by_external_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
		attribute.String("app.request.external_id", externalID.String()),
	)

	logger.Infof("Trying to get asset rate by external id: %s", externalID.String())

	assetRate, err := uc.AssetRateRepo.FindByExternalID(ctx, organizationID, ledgerID, externalID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get asset rate by external id on repository", err)

		logger.Errorf("Error getting asset rate: %v", err)

		return nil, err
	}

	if assetRate != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(assetrate.AssetRate{}).Name(), assetRate.ID)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb asset rate", err)

			logger.Errorf("Error get metadata on mongodb asset rate: %v", err)

			return nil, err
		}

		if metadata != nil {
			assetRate.Metadata = metadata.Data
		}
	}

	return assetRate, nil
}
