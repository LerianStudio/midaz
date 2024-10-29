package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/google/uuid"
)

// CreateProduct creates a new product persists data in the repository.
func (uc *UseCase) CreateProduct(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *r.CreateProductInput) (*r.Product, error) {
	logger := mlog.NewLoggerFromContext(ctx)
	logger.Infof("Trying to create product: %v", cpi)

	var status r.Status
	if cpi.Status.IsEmpty() || common.IsNilOrEmpty(&cpi.Status.Code) {
		status = r.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

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

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(r.Product{}).Name(), prod.ID, cpi.Metadata)
	if err != nil {
		logger.Errorf("Error creating product metadata: %v", err)
		return nil, err
	}

	prod.Metadata = metadata

	return prod, nil
}
