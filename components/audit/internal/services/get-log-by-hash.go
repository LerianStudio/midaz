package services

import (
	"context"
	"encoding/json"
	"github.com/LerianStudio/midaz/components/audit/internal/adapters/rabbitmq/operation"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
)

func (uc *UseCase) GetLogByHash(ctx context.Context, treeID int64, identityHash string) (operation.Operation, error) {
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "services.get_log_by_hash")
	defer span.End()

	log, err := uc.TrillianRepo.GetLogByHash(ctx, treeID, identityHash)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get log by hash", err)

		logger.Errorf("Error getting log by hash: %v", err)

		return operation.Operation{}, err
	}

	var op operation.Operation

	err = json.Unmarshal(log.LeafValue, &op)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to unmarshal log", err)

		logger.Errorf("Error unmarshalling log: %v", err)

		return operation.Operation{}, err
	}

	return op, nil

}
