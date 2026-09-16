// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const AtomicTransactionBatchIdempotencyFormatVersion = 1

//go:embed scripts/claim_atomic_transaction_batch.lua
var claimAtomicTransactionBatchLua string

var claimAtomicTransactionBatchScript = redisclient.NewScript(claimAtomicTransactionBatchLua)

type AtomicTransactionBatchIdempotencyState string

const (
	AtomicTransactionBatchStateClaimed  AtomicTransactionBatchIdempotencyState = "claimed"
	AtomicTransactionBatchStatePrepared AtomicTransactionBatchIdempotencyState = "prepared"
	AtomicTransactionBatchStateApplied  AtomicTransactionBatchIdempotencyState = "applied"
	AtomicTransactionBatchStateComplete AtomicTransactionBatchIdempotencyState = "complete"
)

// AtomicTransactionBatchIdempotencyRecord is the durable Redis state machine
// for one atomic-batch request. TransactionIDs are always in request order and
// Response contains the terminal public JSON representation only at complete.
type AtomicTransactionBatchIdempotencyRecord struct {
	FormatVersion      int                                    `json:"formatVersion"`
	State              AtomicTransactionBatchIdempotencyState `json:"state"`
	RequestFingerprint string                                 `json:"requestFingerprint"`
	OwnerToken         string                                 `json:"ownerToken"`
	BatchID            uuid.UUID                              `json:"batchId"`
	ExecutionID        *uuid.UUID                             `json:"executionId,omitempty"`
	TransactionIDs     []uuid.UUID                            `json:"transactionIds,omitempty"`
	Response           json.RawMessage                        `json:"response,omitempty"`
}

type AtomicTransactionBatchClaimOutcome string

const (
	AtomicTransactionBatchClaimed             AtomicTransactionBatchClaimOutcome = "claimed"
	AtomicTransactionBatchReplayed            AtomicTransactionBatchClaimOutcome = "replayed"
	AtomicTransactionBatchInProgress          AtomicTransactionBatchClaimOutcome = "in_progress"
	AtomicTransactionBatchFingerprintConflict AtomicTransactionBatchClaimOutcome = "fingerprint_conflict"
)

type AtomicTransactionBatchClaimResult struct {
	Outcome     AtomicTransactionBatchClaimOutcome
	InternalKey string
	Record      AtomicTransactionBatchIdempotencyRecord
}

// AtomicTransactionBatchIdempotencyRepository is the narrow command-facing
// port for the batch state machine, kept separate from the legacy singular
// repository contract.
type AtomicTransactionBatchIdempotencyRepository interface {
	ClaimAtomicTransactionBatch(
		ctx context.Context,
		organizationID, ledgerID uuid.UUID,
		effectiveKey string,
		claim AtomicTransactionBatchIdempotencyRecord,
	) (*AtomicTransactionBatchClaimResult, error)
	TransitionAtomicTransactionBatch(
		ctx context.Context,
		organizationID, ledgerID uuid.UUID,
		effectiveKey, ownerToken string,
		expectedState AtomicTransactionBatchIdempotencyState,
		next AtomicTransactionBatchIdempotencyRecord,
		replayTTL time.Duration,
	) (*AtomicTransactionBatchTransitionResult, error)
	DeleteAtomicTransactionBatchPrePublication(
		ctx context.Context,
		organizationID, ledgerID uuid.UUID,
		effectiveKey, ownerToken string,
	) (*AtomicTransactionBatchDeleteResult, error)
	CleanupAbandonedAtomicTransactionBatch(
		ctx context.Context,
		organizationID, ledgerID uuid.UUID,
		effectiveKey, ownerToken string,
		engineEvidence bool,
	) (*AtomicTransactionBatchDeleteResult, error)
}

// ClaimAtomicTransactionBatch atomically creates the first claimed record or
// classifies the existing record as replay, in-progress, or fingerprint
// conflict. Claimed and nonterminal records intentionally receive no TTL.
func (rr *RedisConsumerRepository) ClaimAtomicTransactionBatch(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey string,
	claim AtomicTransactionBatchIdempotencyRecord,
) (*AtomicTransactionBatchClaimResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.claim_atomic_transaction_batch")
	defer span.End()

	if organizationID == uuid.Nil || ledgerID == uuid.Nil {
		return nil, errors.New("atomic transaction batch idempotency scope is required")
	}
	if strings.TrimSpace(effectiveKey) == "" {
		return nil, errors.New("atomic transaction batch effective idempotency key is required")
	}
	if err := validateAtomicTransactionBatchClaim(claim); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(claim)
	if err != nil {
		return nil, fmt.Errorf("marshal atomic transaction batch claim: %w", err)
	}

	internalKey := utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, effectiveKey)
	redisKey, err := tenantKeyFromContextOrError(ctx, internalKey)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to namespace atomic batch idempotency key", err)

		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get redis", err)

		return nil, err
	}

	raw, err := claimAtomicTransactionBatchScript.Run(
		ctx,
		rds,
		[]string{redisKey},
		string(payload),
		claim.RequestFingerprint,
	).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to claim atomic batch idempotency", err)
		logger.Log(ctx, libLog.LevelError, "Failed to claim atomic batch idempotency", libLog.Err(err))

		return nil, fmt.Errorf("claim atomic transaction batch idempotency: %w", err)
	}

	outcome, storedPayload, err := decodeAtomicTransactionBatchClaimReply(raw)
	if err != nil {
		return nil, err
	}

	var stored AtomicTransactionBatchIdempotencyRecord
	if err := json.Unmarshal([]byte(storedPayload), &stored); err != nil {
		return nil, fmt.Errorf("decode atomic transaction batch idempotency record: %w", err)
	}
	if err := validateAtomicTransactionBatchIdempotencyRecord(stored); err != nil {
		return nil, fmt.Errorf("invalid atomic transaction batch idempotency record: %w", err)
	}

	result := &AtomicTransactionBatchClaimResult{
		Outcome:     outcome,
		InternalKey: internalKey,
		Record:      stored,
	}

	switch outcome {
	case AtomicTransactionBatchClaimed, AtomicTransactionBatchReplayed:
		return result, nil
	case AtomicTransactionBatchInProgress, AtomicTransactionBatchFingerprintConflict:
		return result, atomicTransactionBatchIdempotencyConflictError()
	default:
		return nil, fmt.Errorf("unsupported atomic transaction batch claim outcome %q", outcome)
	}
}

func validateAtomicTransactionBatchClaim(claim AtomicTransactionBatchIdempotencyRecord) error {
	if claim.State != AtomicTransactionBatchStateClaimed {
		return errors.New("atomic transaction batch claim must start in claimed state")
	}

	return validateAtomicTransactionBatchIdempotencyRecord(claim)
}

func validateAtomicTransactionBatchIdempotencyRecord(record AtomicTransactionBatchIdempotencyRecord) error {
	if record.FormatVersion != AtomicTransactionBatchIdempotencyFormatVersion {
		return fmt.Errorf("unsupported format version %d", record.FormatVersion)
	}
	if !validAtomicTransactionBatchFingerprint(record.RequestFingerprint) {
		return errors.New("request fingerprint must be lowercase SHA-256 hex")
	}
	if strings.TrimSpace(record.OwnerToken) == "" {
		return errors.New("owner token is required")
	}
	if record.BatchID == uuid.Nil {
		return errors.New("batch ID is required")
	}
	if record.ExecutionID != nil && *record.ExecutionID == uuid.Nil {
		return errors.New("execution ID cannot be nil UUID")
	}
	if err := validateAtomicTransactionBatchTransactionIDs(record.TransactionIDs); err != nil {
		return err
	}

	switch record.State {
	case AtomicTransactionBatchStateClaimed:
		if record.ExecutionID != nil || len(record.TransactionIDs) > 0 || len(record.Response) > 0 {
			return errors.New("claimed record contains prepared or terminal fields")
		}
	case AtomicTransactionBatchStatePrepared:
		if len(record.TransactionIDs) == 0 || len(record.Response) > 0 {
			return errors.New("prepared record requires transaction IDs and no terminal response")
		}
	case AtomicTransactionBatchStateApplied:
		if record.ExecutionID == nil || len(record.TransactionIDs) == 0 || len(record.Response) > 0 {
			return errors.New("applied record requires execution and transaction IDs and no terminal response")
		}
	case AtomicTransactionBatchStateComplete:
		if record.ExecutionID == nil || len(record.TransactionIDs) == 0 || !validAtomicTransactionBatchResponse(record.Response) {
			return errors.New("complete record requires execution and transaction IDs and a JSON response")
		}
	default:
		return fmt.Errorf("unsupported state %q", record.State)
	}

	return nil
}

func validateAtomicTransactionBatchTransactionIDs(transactionIDs []uuid.UUID) error {
	seen := make(map[uuid.UUID]struct{}, len(transactionIDs))
	for _, transactionID := range transactionIDs {
		if transactionID == uuid.Nil {
			return errors.New("transaction ID cannot be nil UUID")
		}
		if _, found := seen[transactionID]; found {
			return fmt.Errorf("transaction ID %s is repeated", transactionID)
		}
		seen[transactionID] = struct{}{}
	}

	return nil
}

func validAtomicTransactionBatchFingerprint(fingerprint string) bool {
	decoded, err := hex.DecodeString(fingerprint)

	return err == nil && len(decoded) == sha256.Size && fingerprint == strings.ToLower(fingerprint)
}

func validAtomicTransactionBatchResponse(response json.RawMessage) bool {
	trimmed := bytes.TrimSpace(response)

	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && json.Valid(trimmed)
}

func decodeAtomicTransactionBatchClaimReply(raw any) (AtomicTransactionBatchClaimOutcome, string, error) {
	values, ok := raw.([]any)
	if !ok || len(values) != 2 {
		return "", "", fmt.Errorf("invalid atomic transaction batch claim reply %T", raw)
	}

	outcome, ok := redisReplyString(values[0])
	if !ok {
		return "", "", errors.New("atomic transaction batch claim reply has invalid outcome")
	}
	payload, ok := redisReplyString(values[1])
	if !ok || payload == "" {
		return "", "", errors.New("atomic transaction batch claim reply has invalid record")
	}

	switch AtomicTransactionBatchClaimOutcome(outcome) {
	case AtomicTransactionBatchClaimed,
		AtomicTransactionBatchReplayed,
		AtomicTransactionBatchInProgress,
		AtomicTransactionBatchFingerprintConflict:
		return AtomicTransactionBatchClaimOutcome(outcome), payload, nil
	default:
		return "", "", fmt.Errorf("unknown atomic transaction batch claim outcome %q", outcome)
	}
}

func redisReplyString(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	default:
		return "", false
	}
}
