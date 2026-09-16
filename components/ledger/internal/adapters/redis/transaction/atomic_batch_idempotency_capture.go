// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//nolint:wsl_v5 // capture phases are intentionally aligned with Redis CAS boundaries.
package redis

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

//go:embed scripts/capture_atomic_transaction_batch_initial_response.lua
var captureAtomicTransactionBatchInitialResponseLua string

var captureAtomicTransactionBatchInitialResponseScript = redisclient.NewScript(captureAtomicTransactionBatchInitialResponseLua)

// CaptureAtomicTransactionBatchInitialResponse freezes one member's creation
// representation before its recovery evidence is acknowledged. It is a CAS:
// the same bytes replay safely while divergent bytes can never overwrite the
// first accepted response.
//
//nolint:gocyclo // every branch protects a distinct immutable capture invariant.
func (rr *RedisConsumerRepository) CaptureAtomicTransactionBatchInitialResponse(
	ctx context.Context,
	organizationID, ledgerID, executionID uuid.UUID,
	ownerToken string,
	transactionID uuid.UUID,
	response json.RawMessage,
) (*AtomicTransactionBatchInitialResponseCaptureResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.capture_atomic_transaction_batch_initial_response")
	defer span.End()

	if organizationID == uuid.Nil || ledgerID == uuid.Nil || executionID == uuid.Nil || transactionID == uuid.Nil || strings.TrimSpace(ownerToken) == "" {
		return nil, errors.New("atomic transaction batch initial response identity is incomplete")
	}

	if !validAtomicTransactionBatchResponse(response) || !strings.HasPrefix(strings.TrimSpace(string(response)), "{") {
		return nil, errors.New("atomic transaction batch initial response is invalid")
	}

	if len(response) > atomicTransactionBatchInitialResponsesMaxBytes {
		return nil, errors.New("atomic transaction batch initial response exceeds byte budget")
	}

	recordKey, record, err := rr.getAtomicTransactionBatchByExecutionID(ctx, organizationID, ledgerID, executionID)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, atomicTransactionBatchIdempotencyConflictError()
	}

	indexKey, err := tenantKeyFromContextOrError(ctx, utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID))
	if err != nil {
		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	raw, err := captureAtomicTransactionBatchInitialResponseScript.Run(
		ctx,
		rds,
		[]string{recordKey, indexKey},
		ownerToken,
		executionID.String(),
		transactionID.String(),
		base64.StdEncoding.EncodeToString(response),
		strconv.Itoa(len(response)),
		strconv.Itoa(atomicTransactionBatchInitialResponsesMaxBytes),
	).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to capture atomic batch initial response", err)
		logger.Log(ctx, libLog.LevelError, "Failed to capture atomic batch initial response", libLog.Err(err))

		return nil, fmt.Errorf("capture atomic transaction batch initial response: %w", err)
	}

	outcome, payload, err := decodeAtomicTransactionBatchScriptReply(raw)
	if err != nil {
		return nil, err
	}

	result := &AtomicTransactionBatchInitialResponseCaptureResult{
		Outcome: AtomicTransactionBatchInitialResponseCaptureOutcome(outcome),
	}
	if payload != "" {
		if err := json.Unmarshal([]byte(payload), &result.Record); err != nil {
			return nil, fmt.Errorf("decode atomic transaction batch initial response record: %w", err)
		}
		if err := validateAtomicTransactionBatchIdempotencyRecord(result.Record); err != nil {
			return nil, fmt.Errorf("invalid atomic transaction batch initial response record: %w", err)
		}
	}

	switch result.Outcome {
	case AtomicTransactionBatchInitialResponseCaptured, AtomicTransactionBatchInitialResponseAlreadyCaptured:
		return result, nil
	case AtomicTransactionBatchInitialResponseMissing,
		AtomicTransactionBatchInitialResponseStaleOwner,
		AtomicTransactionBatchInitialResponseStateConflict,
		AtomicTransactionBatchInitialResponseIndexConflict,
		AtomicTransactionBatchInitialResponseConflict:
		return result, atomicTransactionBatchIdempotencyConflictError()
	default:
		return nil, fmt.Errorf("unsupported atomic transaction batch initial response capture outcome %q", outcome)
	}
}
