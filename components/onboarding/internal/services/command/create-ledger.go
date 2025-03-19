package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel/attribute"

	"github.com/google/uuid"
)

// CreateLedger creates a new ledger persists data in the repository.
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID uuid.UUID, cli *mmodel.CreateLedgerInput) (*mmodel.Ledger, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	// Create a new ledger operation with telemetry
	ledgerID := pkg.GenerateUUIDv7().String() // Generate ID for telemetry
	op := uc.Telemetry.NewLedgerOperation("create", ledgerID)

	// Add important attributes for telemetry
	op.WithAttributes(
		attribute.String("ledger_name", cli.Name),
		attribute.String("organization_id", organizationID.String()),
	)

	// Record system metric
	op.RecordSystemicMetric(ctx)

	// Start trace span for this operation
	ctx = op.StartTrace(ctx)

	defer func() {
		// End span will be done by op.End() at the end of the function
	}()

	logger.Infof("Trying to create ledger: %v", cli)

	var status mmodel.Status
	if cli.Status.IsEmpty() || pkg.IsNilOrEmpty(&cli.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cli.Status
	}

	status.Description = cli.Status.Description

	_, err := uc.LedgerRepo.FindByName(ctx, organizationID, cli.Name)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to find ledger by name", err)

		logger.Errorf("Error creating ledger: %v", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "find_error", err)

		return nil, err
	}

	ledger := &mmodel.Ledger{
		ID:             ledgerID, // Use the previously generated ID
		OrganizationID: organizationID.String(),
		Name:           cli.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	led, err := uc.LedgerRepo.Create(ctx, ledger)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create ledger", err)

		logger.Errorf("Error creating ledger: %v", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "creation_error", err)

		return nil, err
	}

	takeName := reflect.TypeOf(mmodel.Ledger{}).Name()

	metadata, err := uc.CreateMetadata(ctx, takeName, led.ID, cli.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create ledger metadata", err)

		logger.Errorf("Error creating ledger metadata: %v", err)

		// Record error
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "metadata_error", err)

		return nil, err
	}

	led.Metadata = metadata

	// Mark operation as successful
	op.End(ctx, "success")

	return led, nil
}
