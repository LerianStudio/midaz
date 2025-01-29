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

// GetAllMetadataClusters fetch all Clusters from the repository
func (uc *UseCase) GetAllMetadataClusters(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Cluster, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_clusters")
	defer span.End()

	logger.Infof("Retrieving clusters")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(mmodel.Cluster{}).Name(), filter)
	if err != nil || metadata == nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get metadata on repo by query params", err)

		return nil, pkg.ValidateBusinessError(constant.ErrNoClustersFound, reflect.TypeOf(mmodel.Cluster{}).Name())
	}

	uuids := make([]uuid.UUID, len(metadata))
	metadataMap := make(map[string]map[string]any, len(metadata))

	for i, meta := range metadata {
		uuids[i] = uuid.MustParse(meta.EntityID)
		metadataMap[meta.EntityID] = meta.Data
	}

	clusters, err := uc.ClusterRepo.FindByIDs(ctx, organizationID, ledgerID, uuids)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get clusters on repo by query params", err)

		logger.Errorf("Error getting clusters on repo by query params: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return nil, pkg.ValidateBusinessError(constant.ErrNoClustersFound, reflect.TypeOf(mmodel.Cluster{}).Name())
		}

		return nil, err
	}

	for i := range clusters {
		if data, ok := metadataMap[clusters[i].ID]; ok {
			clusters[i].Metadata = data
		}
	}

	return clusters, nil
}
