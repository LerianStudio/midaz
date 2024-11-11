package command

import (
	"context"
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/google/uuid"
	"reflect"
	"time"

	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
)

// CreateLedger creates a new ledger persists data in the repository.
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID uuid.UUID, cli *l.CreateLedgerInput) (*l.Ledger, error) {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_ledger")
	span.End()

	logger.Infof("Trying to create ledger: %v", cli)

	var status l.Status
	if cli.Status.IsEmpty() || common.IsNilOrEmpty(&cli.Status.Code) {
		status = l.Status{
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

	ledger := &l.Ledger{
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

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(l.Ledger{}).Name(), led.ID, cli.Metadata)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create ledger metadata", err)

		logger.Errorf("Error creating ledger metadata: %v", err)

		return nil, err
	}

	led.Metadata = metadata

	return led, nil
}
