// Package command implements write operations (commands) for the onboarding service.
// This file contains the UpdateLedgerByID command implementation.
package command

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

// UpdateLedgerByID updates an existing ledger in the repository.
//
// This method implements the update ledger use case, which:
// 1. Updates the ledger in PostgreSQL
// 2. Updates associated metadata in MongoDB using merge semantics
// 3. Returns the updated ledger with merged metadata
//
// Business Rules:
//   - Only provided fields are updated (partial updates supported)
//   - Organization ID cannot be changed (immutable, validated by parameter)
//   - Name can be updated
//   - Status can be updated
//
// Update Behavior:
//   - Empty strings in input are treated as "clear the field"
//   - Empty status means "don't update status"
//   - Metadata is merged with existing metadata (RFC 7396)
//
// Data Storage:
//   - Primary data: PostgreSQL (ledgers table)
//   - Metadata: MongoDB (merged with existing)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization (used for scoping)
//   - id: UUID of the ledger to update
//   - uli: Update ledger input with fields to update
//
// Returns:
//   - *mmodel.Ledger: Updated ledger with merged metadata
//   - error: Business error if validation fails, database error if persistence fails
//
// Possible Errors:
//   - ErrLedgerIDNotFound: Ledger doesn't exist
//   - Database errors: Connection failures, constraint violations
//
// Example:
//
//	input := &mmodel.UpdateLedgerInput{
//	    Name:   "Treasury Operations - Updated",
//	    Status: mmodel.Status{Code: "ACTIVE"},
//	}
//	ledger, err := useCase.UpdateLedgerByID(ctx, orgID, ledgerID, input)
//
// OpenTelemetry:
//   - Creates span "command.update_ledger_by_id"
//   - Records errors as span events
func (uc *UseCase) UpdateLedgerByID(ctx context.Context, organizationID, id uuid.UUID, uli *mmodel.UpdateLedgerInput) (*mmodel.Ledger, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_ledger_by_id")
	defer span.End()

	logger.Infof("Trying to update ledger: %v", uli)

	ledger := &mmodel.Ledger{
		Name:           uli.Name,
		OrganizationID: organizationID.String(),
		Status:         uli.Status,
	}

	ledgerUpdated, err := uc.LedgerRepo.Update(ctx, organizationID, id, ledger)
	if err != nil {
		logger.Errorf("Error updating ledger on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err = pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			logger.Warnf("Ledger ID not found: %s", id.String())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger on repo by id", err)

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), id.String(), uli.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo", err)

		return nil, err
	}

	ledgerUpdated.Metadata = metadataUpdated

	return ledgerUpdated, nil
}
