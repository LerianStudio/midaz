package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/google/uuid"
)

// CreatePortfolio creates a new portfolio persists data in the repository.
func (uc *UseCase) CreatePortfolio(ctx context.Context, organizationID, ledgerID string, cpi *p.CreatePortfolioInput) (*p.Portfolio, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create portfolio: %v", cpi)

	var status p.Status
	if cpi.Status.IsEmpty() {
		status = p.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	portfolio := &p.Portfolio{
		ID:             uuid.New().String(),
		EntityID:       cpi.EntityID,
		LedgerID:       ledgerID,
		OrganizationID: organizationID,
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	port, err := uc.PortfolioRepo.Create(ctx, portfolio)
	if err != nil {
		logger.Errorf("Error creating portfolio: %v", err)
		return nil, err
	}

	if cpi.Metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, cpi.Metadata); err != nil {
			return nil, err
		}

		meta := m.Metadata{
			EntityID:   port.ID,
			EntityName: reflect.TypeOf(p.Portfolio{}).Name(),
			Data:       cpi.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(p.Portfolio{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating portfolio metadata: %v", err)
			return nil, err
		}

		port.Metadata = cpi.Metadata
	}

	return port, nil
}
