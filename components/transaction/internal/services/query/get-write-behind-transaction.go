package query

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// GetWriteBehindTransaction retrieves a transaction from the write-behind cache in Redis.
// Returns the deserialized transaction with Body and Operations already populated.
// Returns (nil, err) on cache miss or deserialization failure.
func (uc *UseCase) GetWriteBehindTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_write_behind_transaction")
	defer span.End()

	logger.Infof("Looking up transaction in write-behind cache")

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID.String())

	data, err := uc.RedisRepo.GetBytes(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction not found in write-behind cache", err)
		logger.Infof("Transaction not found in write-behind cache: %s", key)
		return nil, err
	}

	var tran transaction.Transaction

	if err := msgpack.Unmarshal(data, &tran); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to unmarshal transaction from write-behind cache", err)
		logger.Warnf("Failed to unmarshal transaction from write-behind cache: %v", err)
		return nil, err
	}

	logger.Infof("Transaction found in write-behind cache: %s", key)

	return &tran, nil
}
