package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
)

// CreateLedger creates a new ledger persists data in the repository.
func (uc *UseCase) CreateLedger(ctx context.Context, organizationID string, cli *l.CreateLedgerInput) (*l.Ledger, error) {
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

	ledger := &l.Ledger{
		OrganizationID: organizationID,
		Name:           cli.Name,
		Status:         status,
	}

	led, err := uc.LedgerRepo.Create(ctx, ledger)
	if err != nil {
		logger.Errorf("Error creating ledger: %v", err)
		return nil, err
	}

	if cli.Metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, cli.Metadata); err != nil {
			return nil, err
		}

		meta := m.Metadata{
			EntityID:   led.ID,
			EntityName: reflect.TypeOf(l.Ledger{}).Name(),
			Data:       cli.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}

		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(l.Ledger{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating ledger metadata: %v", err)
			return nil, err
		}

		led.Metadata = cli.Metadata
	}

	return led, nil
}
