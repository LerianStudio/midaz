// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)

// CreateWriteBehindTransaction stores a transaction in Redis so that it is
// immediately readable via GET even before the async consumer persists it to
// PostgreSQL. This bridges the gap between the synchronous HTTP response
// (which returns the transaction ID) and the eventual database write.
//
// The entry is serialized with msgpack and expires after 24 hours. The key is
// namespaced per tenant automatically by the Redis adapter layer
// (tenantKeyFromContextOrError).
//
// Errors are intentionally swallowed: this is a best-effort cache. The
// transaction will still be persisted via WriteTransaction → RabbitMQ/direct
// DB write regardless of whether the cache entry succeeds.
func (uc *UseCase) CreateWriteBehindTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, tran *transaction.Transaction, transactionInput mtransaction.Transaction) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_write_behind_transaction")
	defer span.End()

	// Attach the original DSL input to the transaction so that
	// GetWriteBehindTransaction callers (and later the commit/cancel flow
	// via tran.Body) can access the full input without a DB round-trip.
	tran.Body = transactionInput

	data, err := msgpack.Marshal(tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal transaction for write-behind cache", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to marshal transaction for write-behind cache", libLog.Err(err))

		return
	}

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	// TTL is 86400 seconds (24 h). SetBytes multiplies the raw value by
	// time.Second internally, so we pass a unitless duration here.
	if err := uc.TransactionRedisRepo.SetBytes(ctx, key, data, time.Duration(86400)); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to store transaction in write-behind cache", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to store transaction in write-behind cache", libLog.Err(err))

		return
	}

	logger.Log(ctx, libLog.LevelInfo, "Transaction stored in write-behind cache", libLog.String("key", key))
}
