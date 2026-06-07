// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"

	// GetWriteBehindTransaction retrieves a transaction from the write-behind cache in Redis.
	// Returns the deserialized transaction with Body and Operations already populated.
	// Returns (nil, err) on cache miss or deserialization failure.
	libLog "github.com/LerianStudio/lib-observability/log"
)

func (uc *UseCase) GetWriteBehindTransaction(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_write_behind_transaction")
	defer span.End()

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, transactionID.String())

	data, err := uc.TransactionRedisRepo.GetBytes(ctx, key)
	if err != nil {
		libOpentelemetry.HandleSpanEvent(span, "Transaction not found in write-behind cache")

		return nil, err
	}

	var tran transaction.Transaction

	if err := msgpack.Unmarshal(data, &tran); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to unmarshal transaction from write-behind cache", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to unmarshal transaction from write-behind cache", libLog.Err(err))

		return nil, err
	}

	return &tran, nil
}
