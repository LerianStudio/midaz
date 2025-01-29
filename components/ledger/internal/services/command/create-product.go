package command

import (
	"context"
	"reflect"
	"time"

	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mmodel"

	"github.com/google/uuid"
)

// CreateProduct creates a new cluster persists data in the repository.
func (uc *UseCase) CreateProduct(ctx context.Context, organizationID, ledgerID uuid.UUID, cpi *mmodel.CreateProductInput) (*mmodel.Product, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_product")
	defer span.End()

	logger.Infof("Trying to create cluster: %v", cpi)

	var status mmodel.Status
	if cpi.Status.IsEmpty() || pkg.IsNilOrEmpty(&cpi.Status.Code) {
		status = mmodel.Status{
			Code: "ACTIVE",
		}
	} else {
		status = cpi.Status
	}

	status.Description = cpi.Status.Description

	product := &mmodel.Product{
		ID:             pkg.GenerateUUIDv7().String(),
		LedgerID:       ledgerID.String(),
		OrganizationID: organizationID.String(),
		Name:           cpi.Name,
		Status:         status,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	_, err := uc.ProductRepo.FindByName(ctx, organizationID, ledgerID, cpi.Name)
	if err != nil {
		pkg.NewLoggerFromContext(ctx).Errorf("Error finding cluster by name: %v", err)

		return nil, err
	}

	prod, err := uc.ProductRepo.Create(ctx, product)
	if err != nil {
		pkg.NewLoggerFromContext(ctx).Errorf("Error creating cluster: %v", err)

		logger.Errorf("Error creating cluster: %v", err)

		return nil, err
	}

	metadata, err := uc.CreateMetadata(ctx, reflect.TypeOf(mmodel.Product{}).Name(), prod.ID, cpi.Metadata)
	if err != nil {
		pkg.NewLoggerFromContext(ctx).Errorf("Error creating cluster metadata: %v", err)

		logger.Errorf("Error creating cluster metadata: %v", err)

		return nil, err
	}

	prod.Metadata = metadata

	return prod, nil
}
