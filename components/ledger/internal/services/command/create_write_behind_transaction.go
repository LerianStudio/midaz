// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"

	// CreateWriteBehindTransaction stores the transaction in Redis as a write-behind cache entry.
	// This ensures the transaction is immediately readable after creation, even before the async
	// consumer persists it to Postgres. Errors are logged but do not block the transaction flow.
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
)

func (uc *UseCase) CreateWriteBehindTransaction(ctx context.Context, organizationID, ledgerID uuid.UUID, tran *transaction.Transaction, parserDSL pkgTransaction.Transaction) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "command.create_write_behind_transaction")
	defer span.End()

	tran.Body = parserDSL

	data, err := msgpack.Marshal(tran)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal transaction for write-behind cache", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to marshal transaction for write-behind cache: %v", err))

		return
	}

	key := utils.WriteBehindTransactionKey(organizationID, ledgerID, tran.ID)

	// 86400 seconds = 24 hours (SetBytes multiplies by time.Second internally)
	if err := uc.TransactionRedisRepo.SetBytes(ctx, key, data, time.Duration(86400)); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to store transaction in write-behind cache", err)
		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to store transaction in write-behind cache: %v", err))

		return
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Transaction stored in write-behind cache: %s", key))
}
