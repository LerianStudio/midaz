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
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/google/uuid"
)

// GetAllClusters fetch all Cluster from the repository
func (uc *UseCase) GetAllClusters(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Cluster, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_clusters")
	defer span.End()

	logger.Infof("Retrieving clusters")

	clusters, err := uc.ClusterRepo.FindAll(ctx, organizationID, ledgerID, filter.ToOffsetPagination())
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get clusters on repo", err)

		logger.Errorf("Error getting clusters on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoClustersFound, reflect.TypeOf(mmodel.Cluster{}).Name())
		}

		return nil, err
	}

	if clusters != nil {
		metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Cluster{}).Name(), filter)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to get metadata on repo", err)

			return nil, pkg.ValidateBusinessError(constant.ErrNoClustersFound, reflect.TypeOf(mmodel.Cluster{}).Name())
		}

		metadataMap := make(map[string]map[string]any, len(metadata))

		for _, meta := range metadata {
			metadataMap[meta.EntityID] = meta.Data
		}

		for i := range clusters {
			if data, ok := metadataMap[clusters[i].ID]; ok {
				clusters[i].Metadata = data
			}
		}
	}

	return clusters, nil
}
