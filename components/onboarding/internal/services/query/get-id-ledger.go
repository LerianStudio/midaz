// Package query implements read operations (queries) for the onboarding service.
// This file contains the query for retrieving a ledger by its ID.
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

// GetLedgerByID retrieves a single ledger by its ID, enriched with metadata.
//
// This use case fetches a ledger from the PostgreSQL database and its corresponding
// metadata from MongoDB, then merges them into a single response.
// Soft-deleted ledgers are excluded from the result.
//
// Parameters:
//   - ctx: The context for tracing, logging, and cancellation.
//   - organizationID: The UUID of the organization that owns the ledger.
//   - id: The UUID of the ledger to retrieve.
//
// Returns:
//   - *mmodel.Ledger: The ledger with its metadata, or nil if not found.
//   - error: An error if the ledger is not found or if the query fails.
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
			// FIXME: This error handling is incorrect. It returns an ErrLedgerIDNotFound, but the error
			// is related to fetching metadata, not the ledger itself. The function should either
			// return the ledger without metadata or a more appropriate metadata-specific error.
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
