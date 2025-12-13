package command

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// UpdateLedgerByID update a ledger from the repository.
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

			return nil, fmt.Errorf("validation failed: %w", err)
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger on repo by id", err)

		return nil, fmt.Errorf("validation failed: %w", err)
	}

	metadataUpdated, err := uc.UpdateMetadata(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), id.String(), uli.Metadata)
	if err != nil {
		logger.Errorf("Error updating metadata: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update metadata on repo", err)

		return nil, fmt.Errorf("operation failed: %w", err)
	}

	ledgerUpdated.Metadata = metadataUpdated

	return ledgerUpdated, nil
}
