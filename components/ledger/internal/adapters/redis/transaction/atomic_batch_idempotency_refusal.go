// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

//go:embed scripts/abort_atomic_transaction_batch_refusal.lua
var abortAtomicTransactionBatchRefusalLua string

var abortAtomicTransactionBatchRefusalScript = redisclient.NewScript(abortAtomicTransactionBatchRefusalLua)

// ErrAtomicTransactionBatchRefusalProtected means a confirmed business refusal
// could not be proven evidence-free. The batch identity must remain protected.
var ErrAtomicTransactionBatchRefusalProtected = errors.New("atomic transaction batch confirmed refusal remains protected")

type AtomicTransactionBatchRefusalAbortOutcome string

const (
	AtomicTransactionBatchRefusalDeleted             AtomicTransactionBatchRefusalAbortOutcome = "deleted"
	AtomicTransactionBatchRefusalAlreadyDeleted      AtomicTransactionBatchRefusalAbortOutcome = "already_deleted"
	AtomicTransactionBatchRefusalStaleOwner          AtomicTransactionBatchRefusalAbortOutcome = "stale_owner"
	AtomicTransactionBatchRefusalStateConflict       AtomicTransactionBatchRefusalAbortOutcome = "state_conflict"
	AtomicTransactionBatchRefusalExecutionConflict   AtomicTransactionBatchRefusalAbortOutcome = "execution_conflict"
	AtomicTransactionBatchRefusalTransactionConflict AtomicTransactionBatchRefusalAbortOutcome = "transaction_conflict"
	AtomicTransactionBatchRefusalIndexConflict       AtomicTransactionBatchRefusalAbortOutcome = "index_conflict"
	AtomicTransactionBatchRefusalEngineEvidence      AtomicTransactionBatchRefusalAbortOutcome = "engine_evidence"
)

type AtomicTransactionBatchRefusalAbortResult struct {
	Outcome AtomicTransactionBatchRefusalAbortOutcome
	Record  AtomicTransactionBatchIdempotencyRecord
}

// AbortAtomicTransactionBatchConfirmedRefusal removes an applied batch record
// and its execution index only when one Lua CAS proves the caller still owns
// the exact execution and neither its receipt nor any recovery field exists.
func (rr *RedisConsumerRepository) AbortAtomicTransactionBatchConfirmedRefusal(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey, ownerToken string,
	executionID uuid.UUID,
	transactionIDs []uuid.UUID,
) (*AtomicTransactionBatchRefusalAbortResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	ctx, span := tracer.Start(ctx, "redis.abort_atomic_transaction_batch_confirmed_refusal")
	defer span.End()

	if err := validateAtomicTransactionBatchRefusalAbort(
		organizationID,
		ledgerID,
		effectiveKey,
		ownerToken,
		executionID,
		transactionIDs,
	); err != nil {
		return nil, err
	}

	encodedTransactionIDs, err := json.Marshal(transactionIDs)
	if err != nil {
		return nil, fmt.Errorf("encode atomic transaction batch refusal members: %w", err)
	}

	keys, err := tenantKeysFromContext(ctx, []string{
		utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, effectiveKey),
		utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID),
		atomicTransactionBatchEngineReceiptInternalKey(organizationID, ledgerID),
		cachepolicy.EngineRecoverQueue,
	})
	if err != nil {
		return nil, err
	}
	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	raw, err := abortAtomicTransactionBatchRefusalScript.Run(
		ctx,
		rds,
		keys,
		ownerToken,
		executionID.String(),
		string(encodedTransactionIDs),
	).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to abort confirmed atomic batch refusal", err)
		logger.Log(ctx, libLog.LevelError, "Failed to abort confirmed atomic batch refusal", libLog.Err(err))

		return nil, fmt.Errorf("abort confirmed atomic transaction batch refusal: %w", err)
	}

	outcome, payload, err := decodeAtomicTransactionBatchRefusalAbortReply(raw)
	if err != nil {
		return nil, err
	}
	result, err := atomicTransactionBatchRefusalAbortResult(outcome, payload)
	if err != nil {
		return nil, err
	}

	switch outcome {
	case AtomicTransactionBatchRefusalDeleted, AtomicTransactionBatchRefusalAlreadyDeleted:
		return result, nil
	case AtomicTransactionBatchRefusalStaleOwner,
		AtomicTransactionBatchRefusalStateConflict,
		AtomicTransactionBatchRefusalExecutionConflict,
		AtomicTransactionBatchRefusalTransactionConflict,
		AtomicTransactionBatchRefusalIndexConflict,
		AtomicTransactionBatchRefusalEngineEvidence:
		return result, fmt.Errorf("%w: %s", ErrAtomicTransactionBatchRefusalProtected, outcome)
	default:
		return nil, fmt.Errorf("unsupported atomic transaction batch refusal abort outcome %q", outcome)
	}
}

func validateAtomicTransactionBatchRefusalAbort(
	organizationID, ledgerID uuid.UUID,
	effectiveKey, ownerToken string,
	executionID uuid.UUID,
	transactionIDs []uuid.UUID,
) error {
	if organizationID == uuid.Nil || ledgerID == uuid.Nil || executionID == uuid.Nil {
		return errors.New("atomic transaction batch refusal abort identity is incomplete")
	}
	if strings.TrimSpace(effectiveKey) == "" || strings.TrimSpace(ownerToken) == "" {
		return errors.New("atomic transaction batch refusal abort ownership is incomplete")
	}
	if err := validateAtomicTransactionBatchTransactionIDs(transactionIDs); err != nil {
		return err
	}
	if len(transactionIDs) == 0 {
		return errors.New("atomic transaction batch refusal abort requires transaction IDs")
	}

	return nil
}

func decodeAtomicTransactionBatchRefusalAbortReply(
	raw any,
) (AtomicTransactionBatchRefusalAbortOutcome, string, error) {
	outcome, payload, err := decodeAtomicTransactionBatchScriptReply(raw)
	if err != nil {
		return "", "", err
	}

	switch AtomicTransactionBatchRefusalAbortOutcome(outcome) {
	case AtomicTransactionBatchRefusalDeleted,
		AtomicTransactionBatchRefusalAlreadyDeleted,
		AtomicTransactionBatchRefusalStaleOwner,
		AtomicTransactionBatchRefusalStateConflict,
		AtomicTransactionBatchRefusalExecutionConflict,
		AtomicTransactionBatchRefusalTransactionConflict,
		AtomicTransactionBatchRefusalIndexConflict,
		AtomicTransactionBatchRefusalEngineEvidence:
		return AtomicTransactionBatchRefusalAbortOutcome(outcome), payload, nil
	default:
		return "", "", fmt.Errorf("unknown atomic transaction batch refusal abort outcome %q", outcome)
	}
}

func atomicTransactionBatchRefusalAbortResult(
	outcome AtomicTransactionBatchRefusalAbortOutcome,
	payload string,
) (*AtomicTransactionBatchRefusalAbortResult, error) {
	result := &AtomicTransactionBatchRefusalAbortResult{Outcome: outcome}
	if payload == "" {
		return result, nil
	}
	if err := json.Unmarshal([]byte(payload), &result.Record); err != nil {
		return nil, fmt.Errorf("decode atomic transaction batch refusal abort record: %w", err)
	}
	if err := validateAtomicTransactionBatchIdempotencyRecord(result.Record); err != nil {
		return nil, fmt.Errorf("invalid atomic transaction batch refusal abort record: %w", err)
	}

	return result, nil
}

func atomicTransactionBatchEngineReceiptInternalKey(organizationID, ledgerID uuid.UUID) string {
	return "engine:" + cachepolicy.HashTag + ":receipts:" + organizationID.String() + ":" + ledgerID.String()
}
