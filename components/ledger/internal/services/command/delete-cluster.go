package command

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

// DeleteClusterByID delete a cluster from the repository by ids.
func (uc *UseCase) DeleteClusterByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.delete_cluster_by_id")
	defer span.End()

	logger.Infof("Remove cluster for id: %s", id.String())

	if err := uc.ClusterRepo.Delete(ctx, organizationID, ledgerID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to delete cluster on repo by id", err)

		logger.Errorf("Error deleting cluster on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrClusterIDNotFound, reflect.TypeOf(mmodel.Cluster{}).Name())
		}

		return err
	}

	return nil
}
