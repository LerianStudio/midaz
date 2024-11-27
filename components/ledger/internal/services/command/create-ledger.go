package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
)

// CreateLedger creates a new ledger persists data in the repository.
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID uuid.UUID, cli *mmodel.CreateLedgerInput) (*mmodel.Ledger, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_ledger")
	span.End()

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

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Ledger{}).Name(), led.ID, cli.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create ledger metadata", err)

		logger.Errorf("Error creating ledger metadata: %v", err)

		return nil, err
	}

	led.Metadata = metadata

	_, err = uc.CreateAuditTree(ctx, organizationID.String(), ledger.ID)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create ledger audit tree", err)

		logger.Errorf("Error creating ledger audit tree: %v", err)

		return nil, err
	}

	return led, nil
}
