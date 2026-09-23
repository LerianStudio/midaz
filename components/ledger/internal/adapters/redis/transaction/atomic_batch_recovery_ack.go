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
//
//nolint:gocyclo // the single CAS boundary validates every recovery and finalization prerequisite
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

	coordinationOrganizationID, coordinationLedgerID, receiptOrganizationID, receiptLedgerID, err := atomicTransactionBatchRecoveryScopes(expectedPayload, organizationID, ledgerID, transactionRaw, executionID)
	if err != nil {
		return 0, err
	}

	recordKey, record, err := rr.getAtomicTransactionBatchByExecutionID(
		ctx,
		coordinationOrganizationID,
		coordinationLedgerID,
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

	attemptsKey, err := recoveryAttemptsQueueKey(source)
	if err != nil {
		return 0, err
	}

	indexKey, err := tenantKeyFromContextOrError(
		ctx,
		utils.AtomicTransactionBatchExecutionIndexInternalKey(coordinationOrganizationID, coordinationLedgerID, executionID),
	)
	if err != nil {
		return 0, err
	}

	keys, err := tenantKeysFromContext(ctx, []string{
		queueKey,
		attemptsKey,
		atomicTransactionBatchEngineReceiptInternalKey(receiptOrganizationID, receiptLedgerID),
		"engine:" + cachepolicy.HashTag + ":guards:" + scope,
		"engine:" + cachepolicy.HashTag + ":protection:" + scope,
		EngineRecoveryCleanupSchedule,
		"engine:" + cachepolicy.HashTag + ":evidence:" + scope,
		"engine:" + cachepolicy.HashTag + ":transaction-index:" + scope,
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

	keys, err = appendRecoveryReceiptProtectionKeys(ctx, client, keys, 2, executionRaw)
	if err != nil {
		return 0, err
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
		receiptOrganizationID.String(),
		receiptLedgerID.String(),
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

func atomicTransactionBatchRecoveryScopes(
	expectedPayload string,
	organizationID, ledgerID uuid.UUID,
	transactionRaw string,
	executionID uuid.UUID,
) (uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, error) {
	var envelope struct {
		RawRecord json.RawMessage `json:"record"`
	}
	if err := json.Unmarshal([]byte(expectedPayload), &envelope); err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("decode atomic batch recovery scopes: %w", err)
	}

	if len(envelope.RawRecord) == 0 {
		return organizationID, ledgerID, organizationID, ledgerID, nil
	}

	var record struct {
		OrganizationID             uuid.UUID  `json:"organizationId"`
		LedgerID                   uuid.UUID  `json:"ledgerId"`
		TransactionID              uuid.UUID  `json:"transactionId"`
		ExecutionID                uuid.UUID  `json:"executionId"`
		CoordinationOrganizationID *uuid.UUID `json:"coordinationOrganizationId"`
		CoordinationLedgerID       *uuid.UUID `json:"coordinationLedgerId"`
		ReceiptOrganizationID      *uuid.UUID `json:"receiptOrganizationId"`
		ReceiptLedgerID            *uuid.UUID `json:"receiptLedgerId"`
	}
	if err := json.Unmarshal(envelope.RawRecord, &record); err != nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, fmt.Errorf("decode atomic batch recovery record scope: %w", err)
	}

	if record.OrganizationID != organizationID || record.LedgerID != ledgerID ||
		record.TransactionID.String() != transactionRaw || record.ExecutionID != executionID ||
		(record.CoordinationOrganizationID == nil) != (record.CoordinationLedgerID == nil) ||
		(record.ReceiptOrganizationID == nil) != (record.ReceiptLedgerID == nil) {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, errors.New("atomic batch recovery scope differs")
	}

	coordinationOrganizationID, coordinationLedgerID := organizationID, ledgerID
	if record.CoordinationOrganizationID != nil {
		coordinationOrganizationID, coordinationLedgerID = *record.CoordinationOrganizationID, *record.CoordinationLedgerID
	}

	receiptOrganizationID, receiptLedgerID := organizationID, ledgerID
	if record.ReceiptOrganizationID != nil {
		receiptOrganizationID, receiptLedgerID = *record.ReceiptOrganizationID, *record.ReceiptLedgerID
	}

	if coordinationOrganizationID == uuid.Nil || coordinationLedgerID == uuid.Nil || receiptOrganizationID == uuid.Nil || receiptLedgerID == uuid.Nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, uuid.Nil, errors.New("atomic batch recovery scope is incomplete")
	}

	return coordinationOrganizationID, coordinationLedgerID, receiptOrganizationID, receiptLedgerID, nil
}
