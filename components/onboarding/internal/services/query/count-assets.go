// Package query implements read operations (queries) for the onboarding service.
// This file contains query implementation.

package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// CountAssets returns the total count of active assets for a ledger.
//
// Counts total assets in PostgreSQL for the given organization and ledger. Excludes soft-deleted assets.
// Used for X-Total-Count header and pagination metadata.
//
// Returns: Total count of active assets, or error if query fails
// OpenTelemetry: Creates span "query.count_assets"
func (uc *UseCase) CountAssets(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_assets")
	defer span.End()

	logger.Infof("Counting assets for organization: %s, ledger: %s", organizationID, ledgerID)

	count, err := uc.AssetRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		logger.Errorf("Error counting assets on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoAssetsFound, reflect.TypeOf(mmodel.Asset{}).Name())

			logger.Warnf("No assets found for organization: %s", organizationID.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count assets on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count assets on repo", err)

		return 0, err
	}

	return count, nil
}
