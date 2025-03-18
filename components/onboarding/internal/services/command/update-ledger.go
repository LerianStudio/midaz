package command

import (
	"context"
	"errors"
	"reflect"
	"time"

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
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.update_ledger_by_id")
	defer span.End()

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "ledger", "update",
		attribute.String("ledger_id", id.String()),
		attribute.String("organization_id", organizationID.String()))

	logger.Infof("Trying to update ledger: %v", uli)

	ledger := &mmodel.Ledger{
		Name:           uli.Name,
		OrganizationID: organizationID.String(),
		Status:         uli.Status,
	}

	ledgerUpdated, err := uc.LedgerRepo.Update(ctx, organizationID, id, ledger)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update ledger on repo", err)

		logger.Errorf("Error updating ledger on repo by id: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "ledger", "update_error",
			attribute.String("ledger_id", id.String()),
			attribute.String("error_detail", err.Error()))

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())
		}

		return nil, err
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), id.String(), uli.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update metadata on repo", err)

		// Record error
		uc.recordOnboardingError(ctx, "ledger", "update_metadata_error",
			attribute.String("ledger_id", id.String()),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	ledgerUpdated.Metadata = metadataUpdated

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "ledger", "update", "success",
		attribute.String("ledger_id", id.String()),
		attribute.String("ledger_name", ledgerUpdated.Name))

	return ledgerUpdated, nil
}
