package command

import (
	"context"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpointers"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/google/uuid"
)

// CreatePortfolio creates a new portfolio persists data in the repository.
func (uc *UseCase) CreatePortfolio(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *p.CreatePortfolioInput) (*p.Portfolio, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	tracer := mopentelemetry.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_portfolio")
	defer span.End()

	logger.Infof("Trying to create portfolio: %v", cpi)

	var status p.Status
	if cpi.Status.IsEmpty() || common.IsNilOrEmpty(&cpi.Status.Code) {
		status = p.Status{
			Code:           "ACTIVE",
			AllowReceiving: mpointers.Bool(true),
			AllowSending:   mpointers.Bool(true),
		}
	} else {
		status = cpi.Status
		if status.AllowReceiving == nil {
			status.AllowReceiving = mpointers.Bool(true)
		}

		if status.AllowSending == nil {
			status.AllowSending = mpointers.Bool(true)
		}
	}

	status.Description = cpi.Status.Description

	portfolio := &p.Portfolio{
		ID:             common.GenerateUUIDv7().String(),
		EntityID:       cpi.EntityID,
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	port, err := uc.PortfolioRepo.Create(ctx, portfolio)
	if err != nil {
		mlog.NewLoggerFromContext(ctx).Errorf("Error creating portfolio: %v", err)

		logger.Errorf("Error creating portfolio: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(p.Portfolio{}).Name(), port.ID, cpi.Metadata)
	if err != nil {
		mlog.NewLoggerFromContext(ctx).Errorf("Error creating portfolio metadata: %v", err)

		logger.Errorf("Error creating portfolio metadata: %v", err)

		return nil, err
	}

	port.Metadata = metadata

	return port, nil
}
