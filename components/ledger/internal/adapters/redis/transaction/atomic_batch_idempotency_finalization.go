// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

//go:embed scripts/handoff_atomic_transaction_batch.lua
var handoffAtomicTransactionBatchLua string

//go:embed scripts/finalize_atomic_transaction_batch.lua
var finalizeAtomicTransactionBatchLua string

var (
	handoffAtomicTransactionBatchScript  = redisclient.NewScript(handoffAtomicTransactionBatchLua)
	finalizeAtomicTransactionBatchScript = redisclient.NewScript(finalizeAtomicTransactionBatchLua)
)

type AtomicTransactionBatchExecutionLookupResult struct {
	Record AtomicTransactionBatchIdempotencyRecord
}

// AtomicTransactionBatchFinalizationCandidateResult describes whether the
// current durable member can be the execution's finalization trigger. The raw
// receipt is an optimistic token: the ACK/finalization script must observe the
// same bytes before it may seal the response.
type AtomicTransactionBatchFinalizationCandidateResult struct {
	Record       AtomicTransactionBatchIdempotencyRecord
	Candidate    bool
	ReceiptToken string
}

type atomicTransactionBatchReceipt struct {
	FormatVersion     int                                     `json:"formatVersion"`
	OrganizationID    uuid.UUID                               `json:"organizationId"`
	LedgerID          uuid.UUID                               `json:"ledgerId"`
	ExecutionID       uuid.UUID                               `json:"executionId"`
	IntentFingerprint string                                  `json:"intentFingerprint"`
	Protection        atomicTransactionBatchReceiptProtection `json:"protection"`
}

type atomicTransactionBatchReceiptProtection struct {
	FormatVersion         int              `json:"formatVersion"`
	RetentionSeconds      int64            `json:"retentionSeconds"`
	Transactions          []uuid.UUID      `json:"transactions"`
	RecoveryFields        []string         `json:"recoveryFields"`
	Acknowledged          map[string]bool  `json:"acknowledged"`
	TerminalCompletedAtMS map[string]int64 `json:"terminalCompletedAtMs"`
}

type AtomicTransactionBatchFinalizationOutcome string

const (
	AtomicTransactionBatchFinalized         AtomicTransactionBatchFinalizationOutcome = "completed"
	AtomicTransactionBatchAlreadyComplete   AtomicTransactionBatchFinalizationOutcome = "already_complete"
	AtomicTransactionBatchFinalizeMissing   AtomicTransactionBatchFinalizationOutcome = "missing"
	AtomicTransactionBatchFinalizeStale     AtomicTransactionBatchFinalizationOutcome = "stale_owner"
	AtomicTransactionBatchFinalizeConflict  AtomicTransactionBatchFinalizationOutcome = "state_conflict"
	AtomicTransactionBatchIndexConflict     AtomicTransactionBatchFinalizationOutcome = "index_conflict"
	AtomicTransactionBatchExecutionConflict AtomicTransactionBatchFinalizationOutcome = "execution_conflict"
)

type AtomicTransactionBatchFinalizationResult struct {
	Outcome  AtomicTransactionBatchFinalizationOutcome
	Record   AtomicTransactionBatchIdempotencyRecord
	Response json.RawMessage
}

type atomicTransactionBatchPublicResponse struct {
	BatchID      uuid.UUID         `json:"batchId"`
	Transactions []json.RawMessage `json:"transactions"`
}

// HandoffAtomicTransactionBatchExecution atomically installs the execution-ID
// index and advances prepared -> applied before the accounting engine is
// invoked. Neither key has a TTL until terminal finalization.
func (rr *RedisConsumerRepository) HandoffAtomicTransactionBatchExecution(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey, ownerToken string,
	next AtomicTransactionBatchIdempotencyRecord,
) (*AtomicTransactionBatchTransitionResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.handoff_atomic_transaction_batch_execution")
	defer span.End()

	if err := validateAtomicTransactionBatchHandoff(ownerToken, next); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("marshal atomic transaction batch handoff: %w", err)
	}

	recordKey, indexKey, rds, err := rr.atomicTransactionBatchExecutionKeys(
		ctx, organizationID, ledgerID, effectiveKey, *next.ExecutionID,
	)
	if err != nil {
		return nil, err
	}

	raw, err := handoffAtomicTransactionBatchScript.Run(
		ctx,
		rds,
		[]string{recordKey, indexKey},
		ownerToken,
		string(payload),
		next.ExecutionID.String(),
	).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to hand off atomic batch execution", err)
		logger.Log(ctx, libLog.LevelError, "Failed to hand off atomic batch execution", libLog.Err(err))

		return nil, fmt.Errorf("hand off atomic transaction batch execution: %w", err)
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
		AtomicTransactionBatchTransitionStateConflict,
		AtomicTransactionBatchTransitionIndexConflict:
		return result, atomicTransactionBatchIdempotencyConflictError()
	default:
		return nil, fmt.Errorf("unsupported atomic transaction batch handoff outcome %q", outcome)
	}
}

// GetAtomicTransactionBatchByExecutionID resolves the recovery-facing index.
// A missing index means the execution is not an atomic batch. Stored pointers
// are strictly scoped before they are dereferenced.
func (rr *RedisConsumerRepository) GetAtomicTransactionBatchByExecutionID(
	ctx context.Context,
	organizationID, ledgerID, executionID uuid.UUID,
) (*AtomicTransactionBatchExecutionLookupResult, error) {
	_, record, err := rr.getAtomicTransactionBatchByExecutionID(ctx, organizationID, ledgerID, executionID)
	if err != nil || record == nil {
		return nil, err
	}

	return &AtomicTransactionBatchExecutionLookupResult{Record: *record}, nil
}

// GetAtomicTransactionBatchFinalizationCandidate resolves the execution index
// and checks the frozen receipt proof before the caller performs the one
// bounded durable-projection read. A false candidate still identifies a batch:
// its current member may be acknowledged, while the atomic ACK script protects
// the race in which that member becomes the last outstanding trigger.
func (rr *RedisConsumerRepository) GetAtomicTransactionBatchFinalizationCandidate(
	ctx context.Context,
	organizationID, ledgerID, executionID, transactionID uuid.UUID,
) (*AtomicTransactionBatchFinalizationCandidateResult, error) {
	_, record, err := rr.getAtomicTransactionBatchByExecutionID(ctx, organizationID, ledgerID, executionID)
	if err != nil || record == nil {
		return nil, err
	}

	result := &AtomicTransactionBatchFinalizationCandidateResult{Record: *record}
	if record.State == AtomicTransactionBatchStateComplete {
		return result, nil
	}

	receiptKey, err := tenantKeyFromContextOrError(
		ctx,
		atomicTransactionBatchEngineReceiptInternalKey(organizationID, ledgerID),
	)
	if err != nil {
		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	receiptRaw, err := rds.HGet(ctx, receiptKey, executionID.String()).Result()
	if errors.Is(err, redisclient.Nil) {
		return nil, errors.New("atomic transaction batch execution receipt is missing")
	}

	if err != nil {
		return nil, fmt.Errorf("get atomic transaction batch execution receipt: %w", err)
	}

	var receipt atomicTransactionBatchReceipt
	if err := json.Unmarshal([]byte(receiptRaw), &receipt); err != nil {
		return nil, fmt.Errorf("decode atomic transaction batch execution receipt: %w", err)
	}

	if err := validateAtomicTransactionBatchReceipt(
		receipt,
		*record,
		organizationID,
		ledgerID,
		executionID,
		transactionID,
	); err != nil {
		return nil, err
	}

	result.Candidate = true

	for _, memberID := range receipt.Protection.Transactions {
		if memberID == transactionID {
			continue
		}

		member := memberID.String()
		if !receipt.Protection.Acknowledged[member] {
			result.Candidate = false
			break
		}
	}

	if result.Candidate {
		result.ReceiptToken = receiptRaw
	}

	return result, nil
}

//nolint:gocyclo // receipt identity, bounds, and ordered membership are one fail-closed proof
func validateAtomicTransactionBatchReceipt(
	receipt atomicTransactionBatchReceipt,
	record AtomicTransactionBatchIdempotencyRecord,
	organizationID, ledgerID, executionID, transactionID uuid.UUID,
) error {
	protection := receipt.Protection
	if receipt.FormatVersion != 1 || receipt.OrganizationID != organizationID ||
		receipt.LedgerID != ledgerID || receipt.ExecutionID != executionID ||
		receipt.IntentFingerprint == "" ||
		protection.FormatVersion != 1 || protection.RetentionSeconds < 1 ||
		protection.RetentionSeconds > 604800 || len(protection.Transactions) == 0 ||
		len(protection.Transactions) != len(record.TransactionIDs) ||
		len(protection.RecoveryFields) != len(record.TransactionIDs) ||
		protection.Acknowledged == nil || protection.TerminalCompletedAtMS == nil {
		return errors.New("atomic transaction batch execution receipt is invalid")
	}

	foundCurrent := false

	for index, memberID := range record.TransactionIDs {
		if protection.Transactions[index] != memberID ||
			protection.RecoveryFields[index] != memberID.String()+":"+executionID.String() {
			return errors.New("atomic transaction batch execution receipt membership differs")
		}

		if memberID == transactionID {
			foundCurrent = true
		}
	}

	if !foundCurrent {
		return errors.New("atomic transaction batch recovery member is not indexed")
	}

	return nil
}

// FinalizeAtomicTransactionBatch reconstructs the public response by walking
// the stored transaction ID slice. Map iteration can therefore never change
// response order. Record and execution index receive the replay TTL together.
func (rr *RedisConsumerRepository) FinalizeAtomicTransactionBatch(
	ctx context.Context,
	organizationID, ledgerID, executionID uuid.UUID,
	ownerToken string,
	transactions map[uuid.UUID]json.RawMessage,
	replayTTL time.Duration,
) (*AtomicTransactionBatchFinalizationResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "redis.finalize_atomic_transaction_batch")
	defer span.End()

	if strings.TrimSpace(ownerToken) == "" {
		return nil, errors.New("atomic transaction batch finalization owner token is required")
	}

	if replayTTL < 0 {
		return nil, errors.New("atomic transaction batch finalization replay TTL cannot be negative")
	}

	recordKey, record, err := rr.getAtomicTransactionBatchByExecutionID(
		ctx, organizationID, ledgerID, executionID,
	)
	if err != nil {
		return nil, err
	}

	if record == nil {
		result := &AtomicTransactionBatchFinalizationResult{Outcome: AtomicTransactionBatchFinalizeMissing}

		return result, atomicTransactionBatchIdempotencyConflictError()
	}

	next := *record
	if record.State != AtomicTransactionBatchStateComplete {
		response, err := buildAtomicTransactionBatchResponse(*record, transactions)
		if err != nil {
			return nil, err
		}

		next.State = AtomicTransactionBatchStateComplete
		next.Response = response
	}

	if err := validateAtomicTransactionBatchIdempotencyRecord(next); err != nil {
		return nil, err
	}

	payload, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("marshal atomic transaction batch finalization: %w", err)
	}

	indexKey, err := tenantKeyFromContextOrError(
		ctx,
		utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID),
	)
	if err != nil {
		return nil, err
	}

	receiptKey, err := tenantKeyFromContextOrError(
		ctx,
		atomicTransactionBatchEngineReceiptInternalKey(organizationID, ledgerID),
	)
	if err != nil {
		return nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return nil, err
	}

	raw, err := finalizeAtomicTransactionBatchScript.Run(
		ctx,
		rds,
		[]string{recordKey, indexKey, receiptKey},
		ownerToken,
		executionID.String(),
		string(payload),
		strconv.FormatInt(int64(replayTTL), 10),
		organizationID.String(),
		ledgerID.String(),
	).Result()
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to finalize atomic batch", err)
		logger.Log(ctx, libLog.LevelError, "Failed to finalize atomic batch", libLog.Err(err))

		return nil, fmt.Errorf("finalize atomic transaction batch: %w", err)
	}

	outcome, storedPayload, err := decodeAtomicTransactionBatchFinalizationReply(raw)
	if err != nil {
		return nil, err
	}

	result, err := atomicTransactionBatchFinalizationResult(outcome, storedPayload)
	if err != nil {
		return nil, err
	}

	switch outcome {
	case AtomicTransactionBatchFinalized, AtomicTransactionBatchAlreadyComplete:
		return result, nil
	case AtomicTransactionBatchFinalizeMissing,
		AtomicTransactionBatchFinalizeStale,
		AtomicTransactionBatchFinalizeConflict,
		AtomicTransactionBatchIndexConflict,
		AtomicTransactionBatchExecutionConflict:
		return result, atomicTransactionBatchIdempotencyConflictError()
	default:
		return nil, fmt.Errorf("unsupported atomic transaction batch finalization outcome %q", outcome)
	}
}

func (rr *RedisConsumerRepository) getAtomicTransactionBatchByExecutionID(
	ctx context.Context,
	organizationID, ledgerID, executionID uuid.UUID,
) (string, *AtomicTransactionBatchIdempotencyRecord, error) {
	if organizationID == uuid.Nil || ledgerID == uuid.Nil || executionID == uuid.Nil {
		return "", nil, errors.New("atomic transaction batch execution lookup identity is incomplete")
	}

	indexKey, err := tenantKeyFromContextOrError(
		ctx,
		utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID),
	)
	if err != nil {
		return "", nil, err
	}

	recordPrefix, err := tenantKeyFromContextOrError(
		ctx,
		utils.AtomicTransactionBatchIdempotencyInternalKeyPrefix(organizationID, ledgerID),
	)
	if err != nil {
		return "", nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return "", nil, err
	}

	recordKey, err := rds.Get(ctx, indexKey).Result()
	if errors.Is(err, redisclient.Nil) {
		return "", nil, nil
	}

	if err != nil {
		return "", nil, fmt.Errorf("get atomic transaction batch execution index: %w", err)
	}

	if !validAtomicTransactionBatchRecordPointer(recordPrefix, recordKey) {
		return "", nil, errors.New("atomic transaction batch execution index contains an invalid record pointer")
	}

	payload, err := rds.Get(ctx, recordKey).Bytes()
	if errors.Is(err, redisclient.Nil) {
		return "", nil, errors.New("atomic transaction batch execution index points to a missing record")
	}

	if err != nil {
		return "", nil, fmt.Errorf("get atomic transaction batch idempotency record: %w", err)
	}

	var record AtomicTransactionBatchIdempotencyRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return "", nil, fmt.Errorf("decode indexed atomic transaction batch record: %w", err)
	}

	if err := validateAtomicTransactionBatchIdempotencyRecord(record); err != nil {
		return "", nil, fmt.Errorf("invalid indexed atomic transaction batch record: %w", err)
	}

	if record.ExecutionID == nil || *record.ExecutionID != executionID ||
		(record.State != AtomicTransactionBatchStateApplied && record.State != AtomicTransactionBatchStateComplete) {
		return "", nil, errors.New("atomic transaction batch execution index points to a mismatched record")
	}

	return recordKey, &record, nil
}

func (rr *RedisConsumerRepository) atomicTransactionBatchExecutionKeys(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey string,
	executionID uuid.UUID,
) (string, string, redisclient.UniversalClient, error) {
	if organizationID == uuid.Nil || ledgerID == uuid.Nil || executionID == uuid.Nil {
		return "", "", nil, errors.New("atomic transaction batch execution identity is incomplete")
	}

	if strings.TrimSpace(effectiveKey) == "" {
		return "", "", nil, errors.New("atomic transaction batch effective idempotency key is required")
	}

	keys, err := tenantKeysFromContext(ctx, []string{
		utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, effectiveKey),
		utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID),
	})
	if err != nil {
		return "", "", nil, err
	}

	rds, err := rr.conn.GetClient(ctx)
	if err != nil {
		return "", "", nil, err
	}

	return keys[0], keys[1], rds, nil
}

func validateAtomicTransactionBatchHandoff(
	ownerToken string,
	next AtomicTransactionBatchIdempotencyRecord,
) error {
	if next.State != AtomicTransactionBatchStateApplied {
		return errors.New("atomic transaction batch handoff must enter applied state")
	}

	if err := validateAtomicTransactionBatchIdempotencyRecord(next); err != nil {
		return err
	}

	if strings.TrimSpace(ownerToken) == "" || ownerToken != next.OwnerToken {
		return errors.New("atomic transaction batch handoff owner token is invalid")
	}

	return nil
}

func buildAtomicTransactionBatchResponse(
	record AtomicTransactionBatchIdempotencyRecord,
	transactions map[uuid.UUID]json.RawMessage,
) (json.RawMessage, error) {
	if record.FormatVersion == AtomicTransactionBatchIdempotencyFormatVersion {
		captured, err := atomicTransactionBatchCapturedResponses(record)
		if err != nil {
			return nil, err
		}

		transactions = captured
	}

	if len(transactions) != len(record.TransactionIDs) {
		return nil, fmt.Errorf(
			"atomic transaction batch finalization requires %d transaction responses, got %d",
			len(record.TransactionIDs),
			len(transactions),
		)
	}

	ordered := make([]json.RawMessage, len(record.TransactionIDs))
	for index, transactionID := range record.TransactionIDs {
		transaction, found := transactions[transactionID]

		trimmed := strings.TrimSpace(string(transaction))
		if !found || !json.Valid(transaction) || !strings.HasPrefix(trimmed, "{") {
			return nil, fmt.Errorf("invalid response for transaction %s", transactionID)
		}

		ordered[index] = transaction
	}

	response, err := json.Marshal(atomicTransactionBatchPublicResponse{
		BatchID:      record.BatchID,
		Transactions: ordered,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal atomic transaction batch response: %w", err)
	}

	return response, nil
}

func atomicTransactionBatchCapturedResponses(
	record AtomicTransactionBatchIdempotencyRecord,
) (map[uuid.UUID]json.RawMessage, error) {
	if record.FormatVersion != AtomicTransactionBatchIdempotencyFormatVersion {
		return nil, errors.New("atomic transaction batch record has no captured initial responses")
	}

	if len(record.InitialResponses) != len(record.TransactionIDs) {
		return nil, fmt.Errorf(
			"atomic transaction batch requires %d captured initial responses, got %d",
			len(record.TransactionIDs),
			len(record.InitialResponses),
		)
	}

	responses := make(map[uuid.UUID]json.RawMessage, len(record.TransactionIDs))
	for _, transactionID := range record.TransactionIDs {
		encoded, found := record.InitialResponses[transactionID.String()]
		if !found {
			return nil, fmt.Errorf("atomic transaction batch initial response for transaction %s is missing", transactionID)
		}

		response, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || !validAtomicTransactionBatchResponse(response) || !strings.HasPrefix(strings.TrimSpace(string(response)), "{") {
			return nil, fmt.Errorf("atomic transaction batch initial response for transaction %s is invalid", transactionID)
		}

		responses[transactionID] = json.RawMessage(response)
	}

	return responses, nil
}

func validAtomicTransactionBatchRecordPointer(prefix, recordKey string) bool {
	if !strings.HasPrefix(recordKey, prefix) {
		return false
	}

	digest := strings.TrimPrefix(recordKey, prefix)
	if len(digest) != sha256.Size*2 || strings.ToLower(digest) != digest {
		return false
	}

	decoded, err := hex.DecodeString(digest)

	return err == nil && len(decoded) == sha256.Size
}

func decodeAtomicTransactionBatchFinalizationReply(
	raw any,
) (AtomicTransactionBatchFinalizationOutcome, string, error) {
	outcome, payload, err := decodeAtomicTransactionBatchScriptReply(raw)
	if err != nil {
		return "", "", err
	}

	switch AtomicTransactionBatchFinalizationOutcome(outcome) {
	case AtomicTransactionBatchFinalized,
		AtomicTransactionBatchAlreadyComplete,
		AtomicTransactionBatchFinalizeMissing,
		AtomicTransactionBatchFinalizeStale,
		AtomicTransactionBatchFinalizeConflict,
		AtomicTransactionBatchIndexConflict,
		AtomicTransactionBatchExecutionConflict:
		return AtomicTransactionBatchFinalizationOutcome(outcome), payload, nil
	default:
		return "", "", fmt.Errorf("unknown atomic transaction batch finalization outcome %q", outcome)
	}
}

func atomicTransactionBatchFinalizationResult(
	outcome AtomicTransactionBatchFinalizationOutcome,
	payload string,
) (*AtomicTransactionBatchFinalizationResult, error) {
	result := &AtomicTransactionBatchFinalizationResult{Outcome: outcome}
	if payload == "" {
		return result, nil
	}

	if err := json.Unmarshal([]byte(payload), &result.Record); err != nil {
		return nil, fmt.Errorf("decode atomic transaction batch finalization record: %w", err)
	}

	if err := validateAtomicTransactionBatchIdempotencyRecord(result.Record); err != nil {
		return nil, fmt.Errorf("invalid atomic transaction batch finalization record: %w", err)
	}

	result.Response = append(json.RawMessage(nil), result.Record.Response...)

	return result, nil
}
