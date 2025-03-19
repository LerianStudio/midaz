package command

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// UpdateLedgerByID update a ledger from the repository.
func (uc *UseCase) UpdateLedgerByID(ctx context.Context, organizationID, id uuid.UUID, uli *mmodel.UpdateLedgerInput) (*mmodel.Ledger, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a new ledger operation with telemetry for update
	op := uc.Telemetry.NewLedgerOperation("update", id.String())

	// Add important attributes for telemetry
	op.WithAttributes(
		attribute.String("ledger_id", id.String()),
		attribute.String("organization_id", organizationID.String()),
	)

	if uli.Name != "" {
		op.WithAttribute("ledger_name", uli.Name)
	}

	// Record system metric
	op.RecordSystemicMetric(ctx)

	// Start trace span for this operation
	ctx = op.StartTrace(ctx)

	defer func() {
		// End span will be done by op.End() at the end of the function
	}()

	logger.Infof("Trying to update ledger: %v", uli)

	ledger := &mmodel.Ledger{
		Name:           uli.Name,
		OrganizationID: organizationID.String(),
		Status:         uli.Status,
	}

	ledgerUpdated, err := uc.LedgerRepo.Update(ctx, organizationID, id, ledger)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to update ledger on repo", err)

		logger.Errorf("Error updating ledger on repo by id: %v", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_error", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		return nil, err
	}

	// Add ledger info to telemetry
	op.WithAttribute("ledger_name", ledgerUpdated.Name)

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), id.String(), uli.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to update metadata on repo", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "update_metadata_error", err)

		return nil, err
	}

	ledgerUpdated.Metadata = metadataUpdated

	// Mark operation as successful
	op.End(ctx, "success")

	return ledgerUpdated, nil
}
