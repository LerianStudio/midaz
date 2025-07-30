package query

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"reflect"
)

// CountAssets returns the total count of assets for a specific ledger in an organization
func (uc *UseCase) CountAssets(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)
	reqId := libCommons.NewHeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_assets")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	logger.Infof("Counting assets for organization: %s, ledger: %s", organizationID, ledgerID)

	count, err := uc.AssetRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count assets on repo", err)
		logger.Errorf("Error counting assets on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return 0, pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())
		}

		return 0, err
	}

	return count, nil
}
