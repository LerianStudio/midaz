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

// GetLedgerByID retrieves a single ledger by ID with metadata.
//
// This method implements the get ledger query use case, which:
// 1. Fetches the ledger from PostgreSQL by ID
// 2. Fetches associated metadata from MongoDB
// 3. Merges metadata into the ledger object
// 4. Returns the enriched ledger
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization (used for scoping)
//   - id: UUID of the ledger to retrieve
//
// Returns:
//   - *mmodel.Ledger: Ledger with metadata
//   - error: Business error if not found or query fails
//
// Possible Errors:
//   - ErrLedgerIDNotFound: Ledger doesn't exist or is deleted
//
// OpenTelemetry:
//   - Creates span "query.get_ledger_by_id"
func (uc *UseCase) GetLedgerByID(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_ledger_by_id")
	defer span.End()

	logger.Infof("Retrieving ledger for id: %s", id.String())

	ledger, err := uc.LedgerRepo.Find(ctx, organizationID, id)
	if err != nil {
		logger.Errorf("Error getting ledger on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get ledger on repo by id", err)

			logger.Warn("No ledger found")

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get ledger on repo by id", err)

		return nil, err
	}

	if ledger != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), id.String())
		if err != nil {
			err := pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb ledger", err)

			logger.Warn("No metadata found")

			return nil, err
		}

		if metadata != nil {
			ledger.Metadata = metadata.Data
		}
	}

	return ledger, nil
}
