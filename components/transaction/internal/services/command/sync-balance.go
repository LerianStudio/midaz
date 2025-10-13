package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/google/uuid"
)

func (uc *UseCase) SyncBalance(ctx context.Context, organizationID, ledgerID uuid.UUID, balance mmodel.BalanceRedis) error {
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.sync_balance")
	defer span.End()

	isNewer, err := uc.BalanceRepo.SyncFromRedisIfNewer(ctx, organizationID, ledgerID, balance)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to sync balance from redis", err)

		return err
	}

	if isNewer {
		logger.Infof("Balance is newer, skipping sync")

		return nil
	}

	return nil
}
