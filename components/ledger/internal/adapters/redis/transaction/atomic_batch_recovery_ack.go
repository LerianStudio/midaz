// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom extends the
// protected exact ACK with the batch terminal CAS. When this member becomes the
// final durable member, the same Lua invocation seals idempotency before it
// deletes the last retry trigger. Status 3 means the caller did not yet provide
// a terminal response; status 4 means its optimistic receipt token changed.
func (rr *RedisConsumerRepository) CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
	ctx context.Context,
	source RecoveryQueueSource,
	organizationID, ledgerID uuid.UUID,
	field, expectedPayload string,
	terminal bool,
	completedAt time.Time,
	receiptToken string,
	transactions map[uuid.UUID]json.RawMessage,
) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if source != RecoveryQueueSourceEngineRecover {
		return 0, errors.New("atomic transaction batch recovery acknowledgment requires engine recovery source")
	}

	transactionRaw, executionRaw, err := protectedRecoveryIDs(
		organizationID,
		ledgerID,
		field,
		expectedPayload,
		completedAt,
	)
	if err != nil {
		return 0, err
	}
	executionID, err := uuid.Parse(executionRaw)
	if err != nil {
		return 0, fmt.Errorf("parse atomic transaction batch recovery execution ID: %w", err)
	}

	recordKey, record, err := rr.getAtomicTransactionBatchByExecutionID(
		ctx,
		organizationID,
		ledgerID,
		executionID,
	)
	if err != nil {
		return 0, err
	}
	if record == nil {
		return 0, errors.New("atomic transaction batch execution index is missing")
	}

	nextPayload := ""
	if record.State == AtomicTransactionBatchStateApplied && receiptToken != "" {
		response, err := buildAtomicTransactionBatchResponse(*record, transactions)
		if err != nil {
			return 0, err
		}
		next := *record
		next.State = AtomicTransactionBatchStateComplete
		next.Response = response
		if err := validateAtomicTransactionBatchIdempotencyRecord(next); err != nil {
			return 0, err
		}
		payload, err := json.Marshal(next)
		if err != nil {
			return 0, fmt.Errorf("marshal atomic transaction batch recovery finalization: %w", err)
		}
		nextPayload = string(payload)
	}

	scope := organizationID.String() + ":" + ledgerID.String()
	queueKey, err := recoveryQueueKey(source)
	if err != nil {
		return 0, err
	}
	indexKey, err := tenantKeyFromContextOrError(
		ctx,
		utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID),
	)
	if err != nil {
		return 0, err
	}
	keys, err := tenantKeysFromContext(ctx, []string{
		queueKey,
		queueKey,
		"engine:" + cachepolicy.HashTag + ":receipts:" + scope,
		"engine:" + cachepolicy.HashTag + ":guards:" + scope,
		"engine:" + cachepolicy.HashTag + ":protection:" + scope,
		EngineRecoveryCleanupSchedule,
	})
	if err != nil {
		return 0, fmt.Errorf("resolve atomic batch recovery acknowledgment keys: %w", err)
	}
	// The execution lookup and index builder already returned tenant-scoped
	// keys. Applying tenantKeysFromContext to them again would point Lua at a
	// different namespace and make every finalization proof fail closed.
	keys = append(keys, recordKey, indexKey)

	counterField, err := tenantKeyFromContextOrError(ctx, field)
	if err != nil {
		return 0, fmt.Errorf("resolve atomic batch recovery attempt field: %w", err)
	}
	client, err := rr.conn.GetClient(ctx)
	if err != nil {
		return 0, fmt.Errorf("get atomic batch recovery acknowledgment client: %w", err)
	}

	terminalFlag := "0"
	if terminal {
		terminalFlag = "1"
	}
	result, err := acknowledgeEngineRecoveryScript.Run(
		ctx,
		client,
		keys,
		field,
		expectedPayload,
		counterField,
		transactionRaw,
		executionRaw,
		terminalFlag,
		completedAt.UnixMilli(),
		"0",
		receiptToken,
		record.OwnerToken,
		nextPayload,
		organizationID.String(),
		ledgerID.String(),
	).Int64()
	if err != nil {
		return 0, fmt.Errorf("acknowledge atomic transaction batch recovery: %w", err)
	}

	switch result {
	case RecoveryAckMissing,
		RecoveryAckDeleted,
		RecoveryAckReplaced,
		RecoveryAckFinalizationRequired,
		RecoveryAckReceiptChanged:
		return result, nil
	default:
		return 0, errors.New("invalid atomic transaction batch recovery acknowledgment result")
	}
}
