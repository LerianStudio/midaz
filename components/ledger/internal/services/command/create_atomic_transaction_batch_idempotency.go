// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
)

type atomicTransactionBatchPrePublicationError struct{ cause error }

func (err atomicTransactionBatchPrePublicationError) Error() string { return err.cause.Error() }
func (err atomicTransactionBatchPrePublicationError) Unwrap() error { return err.cause }

func markAtomicTransactionBatchPrePublication(err error) error {
	if err == nil {
		return nil
	}

	return atomicTransactionBatchPrePublicationError{cause: err}
}

func isAtomicTransactionBatchPrePublication(err error) bool {
	var target atomicTransactionBatchPrePublicationError

	return errors.As(err, &target)
}

// AtomicTransactionBatchIdempotencyRepository is the command-owned subset of
// the Redis batch state machine used before durable projection completion.
type AtomicTransactionBatchIdempotencyRepository interface {
	ClaimAtomicTransactionBatch(
		ctx context.Context,
		organizationID, ledgerID uuid.UUID,
		effectiveKey string,
		claim txRedis.AtomicTransactionBatchIdempotencyRecord,
	) (*txRedis.AtomicTransactionBatchClaimResult, error)
	TransitionAtomicTransactionBatch(
		ctx context.Context,
		organizationID, ledgerID uuid.UUID,
		effectiveKey, ownerToken string,
		expectedState txRedis.AtomicTransactionBatchIdempotencyState,
		next txRedis.AtomicTransactionBatchIdempotencyRecord,
		replayTTL time.Duration,
	) (*txRedis.AtomicTransactionBatchTransitionResult, error)
	HandoffAtomicTransactionBatchExecution(
		ctx context.Context,
		organizationID, ledgerID uuid.UUID,
		effectiveKey, ownerToken string,
		next txRedis.AtomicTransactionBatchIdempotencyRecord,
	) (*txRedis.AtomicTransactionBatchTransitionResult, error)
	FinalizeAtomicTransactionBatch(
		ctx context.Context,
		organizationID, ledgerID, executionID uuid.UUID,
		ownerToken string,
		transactions map[uuid.UUID]json.RawMessage,
		replayTTL time.Duration,
	) (*txRedis.AtomicTransactionBatchFinalizationResult, error)
	CaptureAtomicTransactionBatchInitialResponse(
		ctx context.Context,
		organizationID, ledgerID, executionID uuid.UUID,
		ownerToken string,
		transactionID uuid.UUID,
		response json.RawMessage,
	) (*txRedis.AtomicTransactionBatchInitialResponseCaptureResult, error)
	AbortAtomicTransactionBatchConfirmedRefusal(
		ctx context.Context,
		organizationID, ledgerID uuid.UUID,
		effectiveKey, ownerToken string,
		executionID uuid.UUID,
		transactionIDs []uuid.UUID,
	) (*txRedis.AtomicTransactionBatchRefusalAbortResult, error)
	DeleteAtomicTransactionBatchPrePublication(
		ctx context.Context,
		organizationID, ledgerID uuid.UUID,
		effectiveKey, ownerToken string,
	) (*txRedis.AtomicTransactionBatchDeleteResult, error)
}

func (uc *UseCase) claimAtomicTransactionBatch(
	ctx context.Context,
	in CreateAtomicTransactionBatchV2Input,
	run *atomicTransactionBatchRun,
) (*CreateAtomicTransactionBatchV2Result, error) {
	if uc.AtomicTransactionBatchIdempotencyRepo == nil {
		return nil, nil
	}

	fingerprint, effectiveKey, err := atomicTransactionBatchRequestIdentity(in)
	if err != nil {
		return nil, err
	}

	ownerToken := uuid.NewString()
	claim := txRedis.AtomicTransactionBatchIdempotencyRecord{
		FormatVersion:      txRedis.AtomicTransactionBatchIdempotencyFormatVersion,
		State:              txRedis.AtomicTransactionBatchStateClaimed,
		RequestFingerprint: fingerprint,
		OwnerToken:         ownerToken,
		BatchID:            run.batchID,
		LifecycleAction:    run.idempotencyLifecycle,
	}

	result, err := uc.AtomicTransactionBatchIdempotencyRepo.ClaimAtomicTransactionBatch(
		ctx,
		run.coordinationOrganizationID,
		run.coordinationLedgerID,
		effectiveKey,
		claim,
	)
	if err != nil {
		return nil, err
	}

	if result == nil {
		return nil, fmt.Errorf("atomic transaction batch idempotency claim returned no result")
	}

	switch result.Outcome {
	case txRedis.AtomicTransactionBatchClaimed:
		run.idempotencyEffectiveKey = effectiveKey
		run.idempotencyFingerprint = fingerprint
		run.idempotencyOwnerToken = ownerToken
		run.idempotencyClaimed = true

		return nil, nil
	case txRedis.AtomicTransactionBatchReplayed:
		return decodeAtomicTransactionBatchReplay(result.Record.BatchID, result.Record.Response)
	default:
		return nil, fmt.Errorf("unexpected successful atomic transaction batch claim outcome %q", result.Outcome)
	}
}

func atomicTransactionBatchRequestIdentity(in CreateAtomicTransactionBatchV2Input) (string, string, error) {
	canonical := in.CanonicalRequest
	if len(canonical) == 0 {
		encoded, err := json.Marshal(in.Transactions)
		if err != nil {
			return "", "", fmt.Errorf("encode atomic transaction batch identity: %w", err)
		}

		canonical = encoded
	}

	fingerprint := in.RequestFingerprint
	if fingerprint == "" {
		digest := sha256.Sum256(canonical)
		fingerprint = hex.EncodeToString(digest[:])
	}

	effectiveKey := strings.TrimSpace(in.IdempotencyKey)
	if effectiveKey == "" {
		effectiveKey = fingerprint
	}

	return fingerprint, effectiveKey, nil
}

func decodeAtomicTransactionBatchReplay(batchID uuid.UUID, raw json.RawMessage) (*CreateAtomicTransactionBatchV2Result, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("atomic transaction batch replay has no terminal response")
	}

	var response struct {
		Transactions []*transaction.Transaction `json:"transactions"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode atomic transaction batch replay: %w", err)
	}

	if batchID == uuid.Nil || len(response.Transactions) == 0 {
		return nil, fmt.Errorf("atomic transaction batch replay response is incomplete")
	}

	return &CreateAtomicTransactionBatchV2Result{
		BatchID:      batchID,
		Transactions: response.Transactions,
		Replayed:     true,
	}, nil
}

func (uc *UseCase) abortAtomicTransactionBatchPrePublication(
	ctx context.Context,
	run *atomicTransactionBatchRun,
	primary error,
) error {
	if run == nil || !run.idempotencyClaimed || run.idempotencyHandedOff || uc.AtomicTransactionBatchIdempotencyRepo == nil {
		return markAtomicTransactionBatchPrePublication(primary)
	}

	result, err := uc.AtomicTransactionBatchIdempotencyRepo.DeleteAtomicTransactionBatchPrePublication(
		ctx,
		run.coordinationOrganizationID,
		run.coordinationLedgerID,
		run.idempotencyEffectiveKey,
		run.idempotencyOwnerToken,
	)
	if err != nil {
		return markAtomicTransactionBatchPrePublication(fmt.Errorf("clean up atomic transaction batch after pre-publication failure: %w", err))
	}

	if result == nil || (result.Outcome != txRedis.AtomicTransactionBatchDeleted && result.Outcome != txRedis.AtomicTransactionBatchDeleteMissing) {
		return markAtomicTransactionBatchPrePublication(fmt.Errorf("clean up atomic transaction batch after pre-publication failure: unexpected delete outcome"))
	}

	run.idempotencyClaimed = false

	return markAtomicTransactionBatchPrePublication(primary)
}
