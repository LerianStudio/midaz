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

// CountLedgers returns the total count of active ledgers for an organization.
//
// Counts total ledgers in PostgreSQL for the given organization. Excludes soft-deleted ledgers.
// Used for X-Total-Count header and pagination metadata.
//
// Returns: Total count of active ledgers, or error if query fails
// OpenTelemetry: Creates span "query.count_ledgers"
func (uc *UseCase) CountLedgers(ctx context.Context, organizationID uuid.UUID) (int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_ledgers")
	defer span.End()

	logger.Infof("Counting ledgers for organization: %s", organizationID)

	count, err := uc.LedgerRepo.Count(ctx, organizationID)
	if err != nil {
		logger.Errorf("Error counting ledgers on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrNoLedgersFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			logger.Warnf("No ledgers found for organization: %s", organizationID.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count ledgers on repo", err)

			return 0, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to count ledgers on repo", err)

		return 0, err
	}

	return count, nil
}
