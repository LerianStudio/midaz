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
	tracer := pkg.NewTracerFromContext(ctx)

	// Start time for duration measurement
	startTime := time.Now()

	ctx, span := tracer.Start(ctx, "command.create_ledger")
	defer span.End() // Fix: Use defer to ensure span is ended properly

	// Record operation metrics
	uc.recordOnboardingMetrics(ctx, "ledger", "create",
		attribute.String("ledger_name", cli.Name),
		attribute.String("organization_id", organizationID.String()))

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
		mopentelemetry.HandleSpanError(&span, "Failed to find ledger by name", err)

		logger.Errorf("Error creating ledger: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "ledger", "find_error",
			attribute.String("ledger_name", cli.Name),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	ledger := &mmodel.Ledger{
		OrganizationID: organizationID.String(),
		Name:           cli.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	led, err := uc.LedgerRepo.Create(ctx, ledger)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create ledger", err)

		logger.Errorf("Error creating ledger: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "ledger", "creation_error",
			attribute.String("ledger_name", cli.Name),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	takeName := reflect.TypeOf(mmodel.Ledger{}).Name()

	metadata, err := uc.CreateMetadata(ctx, takeName, led.ID, cli.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create ledger metadata", err)

		logger.Errorf("Error creating ledger metadata: %v", err)

		// Record error
		uc.recordOnboardingError(ctx, "ledger", "metadata_error",
			attribute.String("ledger_id", led.ID),
			attribute.String("error_detail", err.Error()))

		return nil, err
	}

	led.Metadata = metadata

	// Record successful completion and duration
	uc.recordOnboardingDuration(ctx, startTime, "ledger", "create", "success",
		attribute.String("ledger_id", led.ID),
		attribute.String("ledger_name", led.Name))

	return led, nil
}
