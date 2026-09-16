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
	"strconv"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

//go:embed scripts/transition_atomic_transaction_batch.lua
var transitionAtomicTransactionBatchLua string

//go:embed scripts/delete_atomic_transaction_batch.lua
var deleteAtomicTransactionBatchLua string

var (
	transitionAtomicTransactionBatchScript = redisclient.NewScript(transitionAtomicTransactionBatchLua)
	deleteAtomicTransactionBatchScript     = redisclient.NewScript(deleteAtomicTransactionBatchLua)
)

type AtomicTransactionBatchTransitionOutcome string

const (
	AtomicTransactionBatchTransitionUpdated       AtomicTransactionBatchTransitionOutcome = "updated"
	AtomicTransactionBatchAlreadyTransitioned     AtomicTransactionBatchTransitionOutcome = "already_transitioned"
	AtomicTransactionBatchTransitionMissing       AtomicTransactionBatchTransitionOutcome = "missing"
	AtomicTransactionBatchTransitionStaleOwner    AtomicTransactionBatchTransitionOutcome = "stale_owner"
	AtomicTransactionBatchTransitionStateConflict AtomicTransactionBatchTransitionOutcome = "state_conflict"
)

type AtomicTransactionBatchTransitionResult struct {
	Outcome AtomicTransactionBatchTransitionOutcome
	Record  AtomicTransactionBatchIdempotencyRecord
}

type AtomicTransactionBatchDeleteOutcome string

const (
	AtomicTransactionBatchDeleted          AtomicTransactionBatchDeleteOutcome = "deleted"
	AtomicTransactionBatchDeleteMissing    AtomicTransactionBatchDeleteOutcome = "missing"
	AtomicTransactionBatchDeleteStaleOwner AtomicTransactionBatchDeleteOutcome = "stale_owner"
	AtomicTransactionBatchDeleteProtected  AtomicTransactionBatchDeleteOutcome = "protected"
)

type AtomicTransactionBatchDeleteResult struct {
	Outcome AtomicTransactionBatchDeleteOutcome
	Record  AtomicTransactionBatchIdempotencyRecord
}

// TransitionAtomicTransactionBatch advances one owner-held record with an
// atomic compare-and-set. replayTTL is the existing seconds-count convention:
// it must be zero for prepared/applied and positive only for complete.
func (rr *RedisConsumerRepository) TransitionAtomicTransactionBatch(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey, ownerToken string,
	expectedState AtomicTransactionBatchIdempotencyState,
	next AtomicTransactionBatchIdempotencyRecord,
	replayTTL time.Duration,
) (*AtomicTransactionBatchTransitionResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	ctx, span := tracer.Start(ctx, "redis.transition_atomic_transaction_batch")
	defer span.End()

	if err := validateAtomicTransactionBatchTransition(expectedState, next, replayTTL); err != nil {
		return nil, err
	}
	if ownerToken == "" || ownerToken != next.OwnerToken {
		return nil, errors.New("atomic transaction batch transition owner token is invalid")
	}

	payload, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("marshal atomic transaction batch transition: %w", err)
	}

	raw, err := rr.runAtomicTransactionBatchScript(
		ctx,
		organizationID,
		ledgerID,
		effectiveKey,
		transitionAtomicTransactionBatchScript,
		ownerToken,
		string(expectedState),
		string(payload),
		strconv.FormatInt(int64(replayTTL), 10),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to transition atomic batch idempotency", err)
		logger.Log(ctx, libLog.LevelError, "Failed to transition atomic batch idempotency", libLog.Err(err))

		return nil, fmt.Errorf("transition atomic transaction batch idempotency: %w", err)
	}

	outcome, storedPayload, err := decodeAtomicTransactionBatchTransitionReply(raw)
	if err != nil {
		return nil, err
	}
	result, err := atomicTransactionBatchTransitionResult(outcome, storedPayload)
	if err != nil {
		return nil, err
	}

	switch outcome {
	case AtomicTransactionBatchTransitionUpdated, AtomicTransactionBatchAlreadyTransitioned:
		return result, nil
	case AtomicTransactionBatchTransitionMissing,
		AtomicTransactionBatchTransitionStaleOwner,
		AtomicTransactionBatchTransitionStateConflict:
		return result, atomicTransactionBatchIdempotencyConflictError()
	default:
		return nil, fmt.Errorf("unsupported atomic transaction batch transition outcome %q", outcome)
	}
}

// DeleteAtomicTransactionBatchPrePublication removes an owner-held claimed or
// prepared record only while no execution handoff exists.
func (rr *RedisConsumerRepository) DeleteAtomicTransactionBatchPrePublication(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey, ownerToken string,
) (*AtomicTransactionBatchDeleteResult, error) {
	return rr.deleteAtomicTransactionBatch(ctx, organizationID, ledgerID, effectiveKey, ownerToken, false)
}

// CleanupAbandonedAtomicTransactionBatch applies the same owner/state CAS used
// by request cleanup and additionally refuses deletion when an external engine
// evidence probe found evidence. Correct execution handoff always persists the
// execution ID before engine invocation; the evidence flag is a fail-closed
// defense for inconsistent or manually repaired state.
func (rr *RedisConsumerRepository) CleanupAbandonedAtomicTransactionBatch(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey, ownerToken string,
	engineEvidence bool,
) (*AtomicTransactionBatchDeleteResult, error) {
	return rr.deleteAtomicTransactionBatch(ctx, organizationID, ledgerID, effectiveKey, ownerToken, engineEvidence)
}

func (rr *RedisConsumerRepository) deleteAtomicTransactionBatch(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey, ownerToken string,
	engineEvidence bool,
) (*AtomicTransactionBatchDeleteResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	ctx, span := tracer.Start(ctx, "redis.delete_atomic_transaction_batch")
	defer span.End()

	if organizationID == uuid.Nil || ledgerID == uuid.Nil || effectiveKey == "" || ownerToken == "" {
		return nil, errors.New("atomic transaction batch delete identity is incomplete")
	}

	evidenceArg := "0"
	if engineEvidence {
		evidenceArg = "1"
	}
	raw, err := rr.runAtomicTransactionBatchScript(
		ctx,
		organizationID,
		ledgerID,
		effectiveKey,
		deleteAtomicTransactionBatchScript,
		ownerToken,
		evidenceArg,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to delete atomic batch idempotency", err)
		logger.Log(ctx, libLog.LevelError, "Failed to delete atomic batch idempotency", libLog.Err(err))

		return nil, fmt.Errorf("delete atomic transaction batch idempotency: %w", err)
	}

	outcome, storedPayload, err := decodeAtomicTransactionBatchDeleteReply(raw)
	if err != nil {
		return nil, err
	}
	result, err := atomicTransactionBatchDeleteResult(outcome, storedPayload)
	if err != nil {
		return nil, err
	}

	switch outcome {
	case AtomicTransactionBatchDeleted, AtomicTransactionBatchDeleteMissing:
		return result, nil
	case AtomicTransactionBatchDeleteStaleOwner, AtomicTransactionBatchDeleteProtected:
		return result, atomicTransactionBatchIdempotencyConflictError()
	default:
		return nil, fmt.Errorf("unsupported atomic transaction batch delete outcome %q", outcome)
	}
}

func (rr *RedisConsumerRepository) runAtomicTransactionBatchScript(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey string,
	script *redisclient.Script,
	args ...any,
) (any, error) {
	if organizationID == uuid.Nil || ledgerID == uuid.Nil {
		return nil, errors.New("atomic transaction batch idempotency scope is required")
	}
	if effectiveKey == "" {
		return nil, errors.New("atomic transaction batch effective idempotency key is required")
	}

	internalKey := utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, effectiveKey)
	redisKey, err := tenantKeyFromContextOrError(ctx, internalKey)
	if err != nil {
		return nil, err
	}
	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	return script.Run(ctx, rds, []string{redisKey}, args...).Result()
}

func validateAtomicTransactionBatchTransition(
	expectedState AtomicTransactionBatchIdempotencyState,
	next AtomicTransactionBatchIdempotencyRecord,
	replayTTL time.Duration,
) error {
	if err := validateAtomicTransactionBatchIdempotencyRecord(next); err != nil {
		return err
	}

	allowed := expectedState == AtomicTransactionBatchStateClaimed && next.State == AtomicTransactionBatchStatePrepared ||
		expectedState == AtomicTransactionBatchStatePrepared && next.State == AtomicTransactionBatchStateApplied ||
		expectedState == AtomicTransactionBatchStateApplied && next.State == AtomicTransactionBatchStateComplete
	if !allowed {
		return fmt.Errorf("invalid atomic transaction batch transition %q -> %q", expectedState, next.State)
	}

	if next.State == AtomicTransactionBatchStateComplete {
		if replayTTL <= 0 {
			return errors.New("complete atomic transaction batch transition requires a positive replay TTL")
		}

		return nil
	}
	if replayTTL != 0 {
		return errors.New("nonterminal atomic transaction batch transition cannot set a replay TTL")
	}

	return nil
}

func decodeAtomicTransactionBatchTransitionReply(raw any) (AtomicTransactionBatchTransitionOutcome, string, error) {
	outcome, payload, err := decodeAtomicTransactionBatchScriptReply(raw)
	if err != nil {
		return "", "", err
	}

	switch AtomicTransactionBatchTransitionOutcome(outcome) {
	case AtomicTransactionBatchTransitionUpdated,
		AtomicTransactionBatchAlreadyTransitioned,
		AtomicTransactionBatchTransitionMissing,
		AtomicTransactionBatchTransitionStaleOwner,
		AtomicTransactionBatchTransitionStateConflict:
		return AtomicTransactionBatchTransitionOutcome(outcome), payload, nil
	default:
		return "", "", fmt.Errorf("unknown atomic transaction batch transition outcome %q", outcome)
	}
}

func decodeAtomicTransactionBatchDeleteReply(raw any) (AtomicTransactionBatchDeleteOutcome, string, error) {
	outcome, payload, err := decodeAtomicTransactionBatchScriptReply(raw)
	if err != nil {
		return "", "", err
	}

	switch AtomicTransactionBatchDeleteOutcome(outcome) {
	case AtomicTransactionBatchDeleted,
		AtomicTransactionBatchDeleteMissing,
		AtomicTransactionBatchDeleteStaleOwner,
		AtomicTransactionBatchDeleteProtected:
		return AtomicTransactionBatchDeleteOutcome(outcome), payload, nil
	default:
		return "", "", fmt.Errorf("unknown atomic transaction batch delete outcome %q", outcome)
	}
}

func decodeAtomicTransactionBatchScriptReply(raw any) (string, string, error) {
	values, ok := raw.([]any)
	if !ok || len(values) != 2 {
		return "", "", fmt.Errorf("invalid atomic transaction batch script reply %T", raw)
	}

	outcome, ok := redisReplyString(values[0])
	if !ok || outcome == "" {
		return "", "", errors.New("atomic transaction batch script reply has invalid outcome")
	}
	payload, ok := redisReplyString(values[1])
	if !ok {
		return "", "", errors.New("atomic transaction batch script reply has invalid record")
	}

	return outcome, payload, nil
}

func atomicTransactionBatchTransitionResult(
	outcome AtomicTransactionBatchTransitionOutcome,
	payload string,
) (*AtomicTransactionBatchTransitionResult, error) {
	result := &AtomicTransactionBatchTransitionResult{Outcome: outcome}
	if payload == "" {
		return result, nil
	}
	if err := json.Unmarshal([]byte(payload), &result.Record); err != nil {
		return nil, fmt.Errorf("decode atomic transaction batch transition record: %w", err)
	}
	if err := validateAtomicTransactionBatchIdempotencyRecord(result.Record); err != nil {
		return nil, fmt.Errorf("invalid atomic transaction batch transition record: %w", err)
	}

	return result, nil
}

func atomicTransactionBatchDeleteResult(
	outcome AtomicTransactionBatchDeleteOutcome,
	payload string,
) (*AtomicTransactionBatchDeleteResult, error) {
	result := &AtomicTransactionBatchDeleteResult{Outcome: outcome}
	if payload == "" {
		return result, nil
	}
	if err := json.Unmarshal([]byte(payload), &result.Record); err != nil {
		return nil, fmt.Errorf("decode atomic transaction batch delete record: %w", err)
	}
	if err := validateAtomicTransactionBatchIdempotencyRecord(result.Record); err != nil {
		return nil, fmt.Errorf("invalid atomic transaction batch delete record: %w", err)
	}

	return result, nil
}

func atomicTransactionBatchIdempotencyConflictError() error {
	return pkg.ValidateBusinessError(
		constant.ErrIdempotencyKey,
		constant.EntityTransaction,
		"atomic transaction batch",
	)
}
