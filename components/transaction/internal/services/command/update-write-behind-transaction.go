package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// UpdateWriteBehindTransaction re-serializes and updates the transaction in the write-behind cache.
// Called after cancel/commit to reflect the updated status and operations. Errors are logged but
// do not block the transaction flow.
func (uc *UseCase) UpdateWriteBehindTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, tran *transaction.Transaction) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.update_write_behind_transaction")
	defer span.End()

	logger.Infof("Updating transaction in write-behind cache")

	data, err := msgpack.Marshal(tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal transaction for write-behind cache update", err)
		logger.Warnf("Failed to marshal transaction for write-behind cache update: %v", err)
		return
	}

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	if err := uc.RedisRepo.SetBytes(ctx, key, data, 0); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update transaction in write-behind cache", err)
		logger.Warnf("Failed to update transaction in write-behind cache: %v", err)
		return
	}

	logger.Infof("Transaction updated in write-behind cache: %s", key)
}
