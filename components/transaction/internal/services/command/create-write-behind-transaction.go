package command

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libTransaction "github.com/LerianStudio/lib-commons/v2/commons/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// CreateWriteBehindTransaction stores the transaction in Redis as a write-behind cache entry.
// This ensures the transaction is immediately readable after creation, even before the async
// consumer persists it to Postgres. Errors are logged but do not block the transaction flow.
func (uc *UseCase) CreateWriteBehindTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, tran *transaction.Transaction, parserDSL libTransaction.Transaction) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_write_behind_transaction")
	defer span.End()

	logger.Infof("Storing transaction in write-behind cache")

	tran.Body = parserDSL

	data, err := msgpack.Marshal(tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to marshal transaction for write-behind cache", err)
		logger.Warnf("Failed to marshal transaction for write-behind cache: %v", err)
		return
	}

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	if err := uc.RedisRepo.SetBytes(ctx, key, data, 0); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to store transaction in write-behind cache", err)
		logger.Warnf("Failed to store transaction in write-behind cache: %v", err)
		return
	}

	logger.Infof("Transaction stored in write-behind cache: %s", key)
}
