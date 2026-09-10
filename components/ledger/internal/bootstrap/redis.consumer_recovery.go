// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type transactionCompleter interface {
	Complete(context.Context, *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error)
}

type recoveryRecordAcknowledger interface {
	CompareAndDeleteRecovery(context.Context, string, string) (int64, error)
}

type recoveryProtectionAcknowledger interface {
	CompareAndDeleteRecoveryWithProtection(context.Context, uuid.UUID, uuid.UUID, string, string, bool, time.Time) (int64, error)
}

type recoveryCleanupOwner interface {
	CleanupEngineRecovery(context.Context, time.Time, int) (txRedis.RecoveryCleanupResult, error)
}

const recoveryCleanupBatchSize = 100

// WithTransactionCompleter supplies durable SQL and metadata completion for
// version-two records. A missing completer leaves those records in the queue.
func (r *RedisQueueConsumer) WithTransactionCompleter(completer transactionCompleter) *RedisQueueConsumer {
	r.transactionCompleter = completer
	return r
}

// WithRecoveryClock injects the clock used to timestamp durable terminal
// completion. Production constructors use time.Now; deterministic tests supply
// a fixed clock. A nil clock leaves the existing clock unchanged.
func (r *RedisQueueConsumer) WithRecoveryClock(clock func() time.Time) *RedisQueueConsumer {
	if clock != nil {
		r.recoveryClock = clock
	}

	return r
}

func (r *RedisQueueConsumer) cleanupEngineRecovery(ctx context.Context) {
	owner, ok := r.queue.(recoveryCleanupOwner)
	if !ok {
		return
	}

	if r.recoveryClock == nil {
		r.Logger.Log(ctx, libLog.LevelWarn, "Engine recovery cleanup clock is not configured")
		return
	}

	result, err := owner.CleanupEngineRecovery(ctx, r.recoveryClock(), recoveryCleanupBatchSize)
	if err != nil {
		r.Logger.Log(ctx, libLog.LevelWarn, "Failed to clean protected engine recovery artifacts", libLog.Err(err))
		return
	}

	r.Logger.Log(ctx, libLog.LevelDebug, "Cleaned protected engine recovery artifacts",
		libLog.Int("scanned_count", result.Scanned),
		libLog.Int("cleaned_count", result.Cleaned),
		libLog.Int("stale_count", result.Stale),
		libLog.Int("rescheduled_count", result.Rescheduled))
}

// backupRecordVersion permits legacy decoding only when the discriminator is
// absent from a complete JSON object. Explicit but unsupported versions never
// inherit the legacy decoder's permissive zero-value behavior.
func backupRecordVersion(raw string) (int, error) {
	if !json.Valid([]byte(raw)) {
		return 0, errors.New("invalid backup JSON")
	}

	decoder := json.NewDecoder(strings.NewReader(raw))

	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return 0, errors.New("backup record must be an object")
	}

	found := false

	for decoder.More() {
		name, err := decoder.Token()
		if err != nil {
			return 0, fmt.Errorf("read backup discriminator: %w", err)
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return 0, fmt.Errorf("read backup field: %w", err)
		}

		field, ok := name.(string)
		if !ok {
			return 0, errors.New("invalid backup field")
		}

		if !strings.EqualFold(field, "formatVersion") {
			continue
		}

		if found || field != "formatVersion" || !bytes.Equal(bytes.TrimSpace(value), []byte("2")) {
			return 0, errors.New("unsupported or ambiguous backup version")
		}

		found = true
	}

	if found {
		return command.TransactionCompletionFormatVersion, nil
	}

	return 0, nil
}

func trustedLegacyBackupScope(ctx context.Context, field string) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	organizationID, ledgerID, transactionID, ok := parsePoisonKeyIDs(field)
	if !ok || organizationID == uuid.Nil || ledgerID == uuid.Nil || transactionID == uuid.Nil {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}

	canonical := fmt.Sprintf("transaction:{transactions}:%s:%s:%s", organizationID, ledgerID, transactionID)

	expected, err := tmvalkey.GetKeyContext(ctx, canonical)
	if err != nil || field != expected {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}

	return organizationID, ledgerID, transactionID, true
}

func (r *RedisQueueConsumer) handleInvalidBackupRecord(ctx context.Context, span trace.Span, field, raw string, cause error) {
	r.Logger.Log(ctx, libLog.LevelWarn, "Invalid backup discriminator", libLog.String("redis_key", field), libLog.Err(cause))

	if organizationID, ledgerID, transactionID, trusted := trustedLegacyBackupScope(ctx, field); trusted {
		r.quarantinePoisonRecord(ctx, span, r.Logger, field, organizationID, ledgerID, transactionID, []byte(raw), "unmarshal_failure")
	}
}

func decodeRecoveryRecord(ctx context.Context, field, raw string) (*command.TransactionCompletionRecord, time.Time, error) {
	envelope, err := command.DecodeTransactionCompletionRecord([]byte(raw))
	if err != nil {
		return nil, time.Time{}, err
	}

	if envelope.TenantID != tmcore.GetTenantIDContext(ctx) {
		return nil, time.Time{}, errors.New("backup tenant differs from authenticated scope")
	}

	if field != envelope.TransactionID.String()+":"+envelope.ExecutionID.String() {
		return nil, time.Time{}, errors.New("backup field differs from frozen execution identity")
	}

	payload, err := command.DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	if err != nil {
		return nil, time.Time{}, err
	}

	return envelope, payload.TTL, nil
}

func backupRecordEligible(ttl, now time.Time) bool {
	return ttl.Unix() <= now.Add(-MessageTimeOfLife*time.Minute).Unix()
}

func (r *RedisQueueConsumer) processRecoveryRecord(ctx context.Context, field, raw string, envelope *command.TransactionCompletionRecord) {
	if err := r.finalizeRecoveryRecord(ctx, field, raw, envelope); err != nil {
		r.Logger.Log(ctx, libLog.LevelError, "Version-two backup retained after recovery failure", libLog.String("redis_key", field), libLog.Err(err))
	}
}

func (r *RedisQueueConsumer) finalizeRecoveryRecord(ctx context.Context, field, raw string, envelope *command.TransactionCompletionRecord) error {
	startedAt := time.Now()
	outcome := recoveryMetricOutcomeCompleted

	metricsCtx := ctx
	defer func() {
		r.emitRecoveryMetrics(metricsCtx, outcome, time.Since(startedAt))
	}()

	if err := ctx.Err(); err != nil {
		outcome = recoveryMetricOutcomeContextCanceled
		return err
	}

	if r.transactionCompleter == nil {
		outcome = recoveryMetricOutcomeNotConfigured
		return errors.New("durable transaction completer is not configured")
	}

	acknowledger, ok := r.queue.(recoveryRecordAcknowledger)
	if !ok {
		outcome = recoveryMetricOutcomeNotConfigured
		return errors.New("conditional balance recovery acknowledgment is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	completion, err := r.transactionCompleter.Complete(ctx, envelope)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			outcome = recoveryMetricOutcomeContextCanceled
		} else {
			outcome = recoveryMetricOutcomeFinalizationFailed
		}

		return fmt.Errorf("complete recovered transaction: %w", err)
	}

	if err := ctx.Err(); err != nil {
		outcome = recoveryMetricOutcomeContextCanceled
		return err
	}

	var status int64

	if protected, ok := r.queue.(recoveryProtectionAcknowledger); ok {
		terminal, validStatus := durableRecoveryTerminal(completion.Outcome.TransactionStatus)
		if !validStatus {
			outcome = recoveryMetricOutcomeFinalizationFailed
			return fmt.Errorf("complete recovered transaction: durable SQL reported unsupported status %q", completion.Outcome.TransactionStatus)
		}

		if r.recoveryClock == nil {
			outcome = recoveryMetricOutcomeNotConfigured
			return errors.New("durable balance recovery completion clock is not configured")
		}

		completedAt := r.recoveryClock()

		status, err = protected.CompareAndDeleteRecoveryWithProtection(
			ctx, envelope.OrganizationID, envelope.LedgerID, field, raw, terminal, completedAt,
		)
	} else {
		status, err = acknowledger.CompareAndDeleteRecovery(ctx, field, raw)
	}

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			outcome = recoveryMetricOutcomeContextCanceled
		} else {
			outcome = recoveryMetricOutcomeAckFailed
		}

		return fmt.Errorf("acknowledge balance recovery: %w", err)
	}

	switch status {
	case 0, 1:
		return nil
	case 2:
		outcome = recoveryMetricOutcomeRecordChanged
		return errors.New("backup changed during recovery; replacement retained")
	default:
		outcome = recoveryMetricOutcomeInvalidAck
		return errors.New("invalid conditional recovery acknowledgment result")
	}
}

func durableRecoveryTerminal(status string) (terminal, valid bool) {
	switch status {
	case constant.APPROVED, constant.CANCELED:
		return true, true
	case constant.PENDING:
		return false, true
	default:
		return false, false
	}
}
