package query

import (
	"context"
	"errors"
	"reflect"

	"github.com/LerianStudio/midaz/components/ledger/internal/services"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"

	"github.com/google/uuid"
)

// GetClusterByID get a Cluster from the repository by given id.
func (uc *UseCase) GetClusterByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Cluster, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_cluster_by_id")
	defer span.End()

	logger.Infof("Retrieving cluster for id: %s", id.String())

	cluster, err := uc.ClusterRepo.Find(ctx, organizationID, ledgerID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get cluster on repo by id", err)

		logger.Errorf("Error getting cluster on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrClusterIDNotFound, reflect.TypeOf(mmodel.Cluster{}).Name())
		}

		return nil, err
	}

	if cluster != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(mmodel.Cluster{}).Name(), id.String())
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb cluster", err)

			logger.Errorf("Error get metadata on mongodb cluster: %v", err)

			return nil, err
		}

		if metadata != nil {
			cluster.Metadata = metadata.Data
		}
	}

	return cluster, nil
}
