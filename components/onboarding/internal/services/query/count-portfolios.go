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

// CountPortfolios returns the total count of active portfolios for a ledger.
//
// Counts total portfolios in PostgreSQL for the given organization and ledger. Excludes soft-deleted portfolios.
// Used for X-Total-Count header and pagination metadata.
//
// Returns: Total count of active portfolios, or error if query fails
// OpenTelemetry: Creates span "query.count_portfolios"
func (uc *UseCase) CountPortfolios(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_portfolios")
	defer span.End()

	logger.Infof("Counting portfolios for organization %s and ledger %s", organizationID, ledgerID)

	count, err := uc.PortfolioRepo.Count(ctx, organizationID, ledgerID)
	if err != nil {
		logger.Errorf("Error counting portfolios on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoPortfoliosFound, reflect.TypeOf(mmodel.Portfolio{}).Name())

			logger.Warnf("No portfolios found for organization: %s", organizationID.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count portfolios on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count portfolios on repo", err)

		return 0, err
	}

	logger.Infof("Found %d portfolios for organization %s and ledger %s", count, organizationID, ledgerID)

	return count, nil
}
