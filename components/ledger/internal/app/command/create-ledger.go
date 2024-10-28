package command

import (
	"context"
	"github.com/google/uuid"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common/mlog"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
)

// CreateLedger creates a new ledger persists data in the repository.
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID uuid.UUID, cli *l.CreateLedgerInput) (*l.Ledger, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create ledger: %v", cli)

	var status l.Status
	if cli.Status.IsEmpty() {
		status = l.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cli.Status
	}

	_, err := uc.LedgerRepo.FindByName(ctx, organizationID, cli.Name)
	if err != nil {
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
		logger.Errorf("Error creating ledger: %v", err)
		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(l.Ledger{}).Name(), led.ID, cli.Metadata)
	if err != nil {
		logger.Errorf("Error creating ledger metadata: %v", err)
		return nil, err
	}

	led.Metadata = metadata

	return led, nil
}
