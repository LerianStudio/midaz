// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"

	// GetWriteBehindTransaction retrieves a transaction from the write-behind cache in Redis.
	// Returns the deserialized transaction with Body and Operations already populated.
	// Returns (nil, err) on cache miss or deserialization failure.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) GetWriteBehindTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_write_behind_transaction")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Looking up transaction in write-behind cache")

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID.String())

	data, err := uc.TransactionRedisRepo.GetBytes(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanEvent(span, "Transaction not found in write-behind cache")
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Transaction not found in write-behind cache: %s", key))

		return nil, err
	}

	var tran transaction.Transaction

	if err := msgpack.Unmarshal(data, &tran); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to unmarshal transaction from write-behind cache", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to unmarshal transaction from write-behind cache: %v", err))

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Transaction found in write-behind cache: %s", key))

	return &tran, nil
}
