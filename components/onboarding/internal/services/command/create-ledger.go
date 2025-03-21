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

// CreateLedger creates a new ledger and persists data in the repository.
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID uuid.UUID, cli *mmodel.CreateLedgerInput) (*mmodel.Ledger, error) {
	logger := pkg.NewLoggerFromContext(ctx)

	ledgerID := pkg.GenerateUUIDv7().String()
	op := uc.Telemetry.NewLedgerOperation("create", ledgerID)

	op.WithAttributes(
		attribute.String("ledger_name", cli.Name),
		attribute.String("organization_id", organizationID.String()),
	)

	op.RecordSystemicMetric(ctx)
	ctx = op.StartTrace(ctx)

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
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "find_error", err)

		return nil, err
	}

	ledger := &mmodel.Ledger{
		ID:             ledgerID,
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
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "creation_error", err)

		return nil, err
	}

	takeName := reflect.TypeOf(mmodel.Ledger{}).Name()

	metadata, err := uc.CreateMetadata(ctx, takeName, led.ID, cli.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&op.span, "Failed to create ledger metadata", err)
		logger.Errorf("Error creating ledger metadata: %v", err)
		op.WithAttribute("error_detail", err.Error())
		op.RecordError(ctx, "metadata_error", err)

		return nil, err
	}

	led.Metadata = metadata

	op.End(ctx, "success")

	return led, nil
}
