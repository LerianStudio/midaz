package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/google/uuid"
)

// CreateProduct creates a new product persists data in the repository.
func (uc *UseCase) CreateProduct(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *r.CreateProductInput) (*r.Product, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create product: %v", cpi)

	var status r.Status
	if cpi.Status.IsEmpty() {
		status = r.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	product := &r.Product{
		ID:             common.GenerateUUIDv7().String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	_, err := uc.ProductRepo.FindByName(ctx, organizationID, ledgerID, cpi.Name)
	if err != nil {
		return nil, err
	}

	prod, err := uc.ProductRepo.Create(ctx, product)
	if err != nil {
		logger.Errorf("Error creating product: %v", err)
		return nil, err
	}

	if cpi.Metadata != nil {
		if err := common.CheckMetadataKeyAndValueLength(100, cpi.Metadata); err != nil {
			return nil, common.ValidateBusinessError(err, reflect.TypeOf(r.Product{}).Name())
		}

		meta := m.Metadata{
			EntityID:   prod.ID,
			EntityName: reflect.TypeOf(r.Product{}).Name(),
			Data:       cpi.Metadata,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		}
		if err := uc.MetadataRepo.Create(ctx, reflect.TypeOf(r.Product{}).Name(), &meta); err != nil {
			logger.Errorf("Error into creating product metadata: %v", err)
			return nil, err
		}

		prod.Metadata = cpi.Metadata
	}

	return prod, nil
}
