// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmvalkey "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/valkey"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	transactionquarantine "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionquarantine"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type legacyRecoveryRecordAcknowledger interface {
	CompareAndDeleteRecovery(context.Context, string, string) (int64, error)
}

type recoveryRecordAcknowledger interface {
	CompareAndDeleteRecoveryFrom(context.Context, txRedis.RecoveryQueueSource, string, string) (int64, error)
}

type legacyRecoveryProtectionAcknowledger interface {
	CompareAndDeleteRecoveryWithProtection(context.Context, uuid.UUID, uuid.UUID, string, string, bool, time.Time) (int64, error)
}

type recoveryProtectionAcknowledger interface {
	CompareAndDeleteRecoveryWithProtectionFrom(context.Context, txRedis.RecoveryQueueSource, uuid.UUID, uuid.UUID, string, string, bool, time.Time) (int64, error)
}

type atomicTransactionBatchRecoveryAcknowledger interface {
	CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
		context.Context,
		txRedis.RecoveryQueueSource,
		uuid.UUID,
		uuid.UUID,
		string,
		string,
		bool,
		time.Time,
		string,
		map[uuid.UUID]json.RawMessage,
	) (int64, error)
}

type recoveryRecordReader interface {
	ReadRecoveryMessage(context.Context, txRedis.RecoveryQueueSource, string) (string, error)
}

type recoveryCleanupOwner interface {
	CleanupEngineRecovery(context.Context, time.Time, int) (txRedis.RecoveryCleanupResult, error)
}

const (
	recoveryCleanupBatchSize              = 100
	transactionWriteBehindRecoveryVersion = command.TransactionWriteBehindFormatVersion
	engineRecoveryAttemptLimit            = 3
	engineRecoveryInitialBackoff          = 10 * time.Millisecond
)

var errRecoveryRecordChanged = errors.New("recovery record changed during completion")

type recoveryAttemptTracker interface {
	IncrementRecoveryAttempt(context.Context, txRedis.RecoveryQueueSource, string) (int64, error)
}

// recoveryRecordCompleter is the only capability exposed to the engine
// recovery consumer. It can complete durable projections and acknowledge the
// exact recovery record; it cannot execute accounting mutations.
type recoveryRecordCompleter struct {
	logger         libLog.Logger
	queue          txRedis.RedisRepository
	completer      command.AppliedTransactionCompleter
	resolver       command.TransactionEvidenceResolver
	batchFinalizer command.AtomicTransactionBatchRecoveryFinalizer
	quarantineRepo transactionquarantine.Repository
	clock          func() time.Time
	metricsFactory *metrics.MetricsFactory
}

func (r *RedisQueueConsumer) newRecoveryRecordCompleter() *recoveryRecordCompleter {
	completion := &recoveryRecordCompleter{
		logger:         r.Logger,
		queue:          r.queue,
		completer:      r.appliedTransactionCompleter,
		quarantineRepo: r.quarantineRepo,
		clock:          r.recoveryClock,
		metricsFactory: r.metricsFactory,
	}
	if repository, ok := r.queue.(txRedis.EngineWriteBehindRepository); ok {
		completion.resolver = rabbitEngineEvidenceResolver{repository: repository}
	}

	if r.Command != nil && r.Command.AtomicTransactionBatchIdempotencyRepo != nil {
		completion.batchFinalizer = r.Command
	}

	return completion
}

// WithAppliedTransactionCompleter supplies durable SQL and metadata completion
// for version-two records. A missing completer leaves those records in the queue.
func (r *RedisQueueConsumer) WithAppliedTransactionCompleter(completer command.AppliedTransactionCompleter) *RedisQueueConsumer {
	r.appliedTransactionCompleter = completer
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
	r.newRecoveryRecordCompleter().cleanup(ctx)
}

func (r *recoveryRecordCompleter) cleanup(ctx context.Context) {
	owner, ok := r.queue.(recoveryCleanupOwner)
	if !ok {
		return
	}

	if r.clock == nil {
		r.logger.Log(ctx, libLog.LevelWarn, "Engine recovery cleanup clock is not configured")
		return
	}

	result, err := owner.CleanupEngineRecovery(ctx, r.clock(), recoveryCleanupBatchSize)
	if err != nil {
		r.logger.Log(ctx, libLog.LevelWarn, "Failed to clean protected engine recovery artifacts", libLog.Err(err))
		return
	}

	r.logger.Log(ctx, libLog.LevelDebug, "Cleaned protected engine recovery artifacts",
		libLog.Int("scanned_count", result.Scanned),
		libLog.Int("cleaned_count", result.Cleaned),
		libLog.Int("stale_count", result.Stale),
		libLog.Int("rescheduled_count", result.Rescheduled))
}

// recoveryRecordVersion permits legacy decoding only when the discriminator is
// absent from a complete JSON object. Explicit but unsupported versions never
// inherit the legacy decoder's permissive zero-value behavior.
func recoveryRecordVersion(raw string) (int, error) {
	if !json.Valid([]byte(raw)) {
		return 0, errors.New("invalid recovery JSON")
	}

	decoder := json.NewDecoder(strings.NewReader(raw))

	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return 0, errors.New("recovery record must be an object")
	}

	found := false
	version := 0
	hasRecord := false

	for decoder.More() {
		name, err := decoder.Token()
		if err != nil {
			return 0, fmt.Errorf("read recovery discriminator: %w", err)
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return 0, fmt.Errorf("read recovery field: %w", err)
		}

		field, ok := name.(string)
		if !ok {
			return 0, errors.New("invalid recovery field")
		}

		if field == "record" {
			hasRecord = true
		}

		if !strings.EqualFold(field, "formatVersion") {
			continue
		}

		if found || field != "formatVersion" {
			return 0, errors.New("unsupported or ambiguous backup version")
		}

		switch {
		case bytes.Equal(bytes.TrimSpace(value), []byte("1")):
			version = transactionWriteBehindRecoveryVersion
		case bytes.Equal(bytes.TrimSpace(value), []byte("2")):
			version = command.TransactionCompletionFormatVersion
		default:
			return 0, errors.New("unsupported or ambiguous backup version")
		}

		found = true
	}

	if found {
		if version == transactionWriteBehindRecoveryVersion && !hasRecord {
			return 0, errors.New("unsupported or ambiguous backup version")
		}

		return version, nil
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
	writeBehind, ttl, err := decodeRecoveryEnvelope(ctx, field, raw)
	if err != nil {
		return nil, time.Time{}, err
	}

	return &writeBehind.Record, ttl, nil
}

func decodeRecoveryEnvelope(ctx context.Context, field, raw string) (*command.TransactionWriteBehindEnvelope, time.Time, error) {
	writeBehind, err := command.DecodeTransactionWriteBehindEnvelope([]byte(raw))
	if err != nil {
		return nil, time.Time{}, err
	}

	envelope := &writeBehind.Record

	if envelope.TenantID != tmcore.GetTenantIDContext(ctx) {
		return nil, time.Time{}, errors.New("recovery tenant differs from authenticated scope")
	}

	if field != envelope.TransactionID.String()+":"+envelope.ExecutionID.String() {
		return nil, time.Time{}, errors.New("recovery field differs from recorded execution identity")
	}

	payload, err := command.DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	if err != nil {
		return nil, time.Time{}, err
	}

	return writeBehind, payload.TTL, nil
}

func recoveryRecordEligible(ttl, now time.Time) bool {
	return ttl.Unix() <= now.Add(-MessageTimeOfLife*time.Minute).Unix()
}

func (r *recoveryRecordCompleter) process(ctx context.Context, source txRedis.RecoveryQueueSource, field, raw string, envelope *command.TransactionCompletionRecord) {
	if err := r.complete(ctx, source, field, raw, envelope); err != nil {
		r.logger.Log(ctx, libLog.LevelError, "Version-two record retained after recovery failure", libLog.String("source", string(source)), libLog.String("redis_key", field), libLog.Err(err))
	}
}

func (r *recoveryRecordCompleter) processEngine(ctx context.Context, field, raw string, envelope *command.TransactionWriteBehindEnvelope) {
	var err error
	for attempt := 1; attempt <= engineRecoveryAttemptLimit; attempt++ {
		err = r.completeWriteBehind(ctx, txRedis.RecoveryQueueSourceEngineRecover, field, raw, envelope)
		if err == nil || errors.Is(err, errRecoveryRecordChanged) {
			return
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			break
		}

		if attempt < engineRecoveryAttemptLimit {
			if waitErr := waitRecoveryBackoff(ctx, engineRecoveryInitialBackoff*time.Duration(1<<(attempt-1))); waitErr != nil {
				err = waitErr
				break
			}
		}
	}

	r.logger.Log(ctx, libLog.LevelError, "Engine recovery record retained after bounded retries",
		libLog.String("redis_key", field), libLog.Int("attempts", engineRecoveryAttemptLimit), libLog.Err(err))

	if ctx.Err() == nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		r.quarantineEngineRecovery(ctx, field, raw, envelope, err)
	}
}

func waitRecoveryBackoff(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (r *recoveryRecordCompleter) quarantineEngineRecovery(
	ctx context.Context,
	field, raw string,
	envelope *command.TransactionWriteBehindEnvelope,
	cause error,
) {
	tracker, ok := r.queue.(recoveryAttemptTracker)
	if !ok {
		r.logger.Log(ctx, libLog.LevelError, "Engine recovery attempt tracking is not configured; record retained",
			libLog.String("redis_key", field))

		return
	}

	attempts, err := tracker.IncrementRecoveryAttempt(ctx, txRedis.RecoveryQueueSourceEngineRecover, field)
	if err != nil {
		r.logger.Log(ctx, libLog.LevelError, "Failed to increment engine recovery attempt; record retained",
			libLog.String("redis_key", field), libLog.Err(err))

		return
	}

	if attempts < QuarantineThreshold {
		return
	}

	if r.quarantineRepo == nil {
		r.logger.Log(ctx, libLog.LevelError, "Quarantine repository not configured; engine recovery record retained",
			libLog.String("redis_key", field), libLog.Int("attempts", int(attempts)))

		return
	}

	if envelope == nil {
		return
	}

	payloadDigest := sha256.Sum256([]byte(raw))

	record := &transactionquarantine.QuarantineRecord{
		OrganizationID: envelope.Record.OrganizationID,
		LedgerID:       envelope.Record.LedgerID,
		TransactionID:  envelope.Record.TransactionID,
		RedisKey:       fmt.Sprintf("engine_recover:%s:%x", field, payloadDigest),
		Payload:        []byte(raw),
		FailureReason:  "engine_projection_failure",
		Attempts:       int(attempts),
		FirstFailedAt:  time.Now(),
		QuarantinedAt:  time.Now(),
	}
	if err := r.quarantineRepo.Insert(ctx, record); err != nil {
		r.logger.Log(ctx, libLog.LevelError, "Failed to persist engine recovery quarantine; record retained",
			libLog.String("redis_key", field), libLog.Err(err))

		return
	}

	acknowledger, ok := r.queue.(recoveryRecordAcknowledger)
	if !ok {
		r.logger.Log(ctx, libLog.LevelError, "Engine recovery quarantine persisted but exact removal is not configured; record retained",
			libLog.String("redis_key", field))

		return
	}

	status, err := acknowledger.CompareAndDeleteRecoveryFrom(ctx, txRedis.RecoveryQueueSourceEngineRecover, field, raw)
	if err != nil {
		r.logger.Log(ctx, libLog.LevelError, "Engine recovery quarantine persisted but exact removal failed; record retained",
			libLog.String("redis_key", field), libLog.Err(err))

		return
	}

	if status == txRedis.RecoveryAckReplaced {
		r.logger.Log(ctx, libLog.LevelWarn, "Engine recovery record changed during quarantine; replacement retained",
			libLog.String("redis_key", field))

		return
	}

	r.logger.Log(ctx, libLog.LevelError, "Engine recovery record quarantined after bounded retries",
		libLog.String("redis_key", field), libLog.String("failure_reason", record.FailureReason),
		libLog.Int("attempts", int(attempts)), libLog.Err(cause))
}

// completeWriteBehind is the shared engine projection path used by RabbitMQ
// and recovery. It resolves predecessor/origin evidence before projection and
// intentionally has no accounting-engine capability.
func (r *recoveryRecordCompleter) completeWriteBehind(
	ctx context.Context,
	source txRedis.RecoveryQueueSource,
	field, raw string,
	envelope *command.TransactionWriteBehindEnvelope,
) error {
	startedAt := time.Now()
	outcome := recoveryMetricOutcomeCompleted

	metricsCtx := ctx
	defer func() { r.emitRecoveryMetrics(metricsCtx, source, outcome, time.Since(startedAt)) }()

	if err := ctx.Err(); err != nil {
		outcome = recoveryMetricOutcomeContextCanceled
		return err
	}

	if r.completer == nil {
		outcome = recoveryMetricOutcomeNotConfigured
		return errors.New("applied transaction completer is not configured")
	}

	if !supportsRecoveryAcknowledgment(r.queue, source) {
		outcome = recoveryMetricOutcomeNotConfigured
		return errors.New("conditional balance recovery acknowledgment is not configured")
	}

	completionCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	completed, err := command.CompleteTransactionWriteBehind(completionCtx, envelope, r.resolver, r.completer)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			outcome = recoveryMetricOutcomeContextCanceled
		} else {
			outcome = recoveryMetricOutcomeFinalizationFailed
		}

		return fmt.Errorf("complete recovered transaction write-behind: %w", err)
	}

	if err := completionCtx.Err(); err != nil {
		outcome = recoveryMetricOutcomeContextCanceled
		return err
	}

	var batchFinalization *command.AtomicTransactionBatchRecoveryFinalization
	if source == txRedis.RecoveryQueueSourceEngineRecover && r.batchFinalizer != nil {
		batchFinalization, err = r.batchFinalizer.PrepareAtomicTransactionBatchRecoveryFinalization(
			completionCtx, &envelope.Record, completed.Current,
		)
		if err != nil {
			outcome = recoveryMetricOutcomeFinalizationFailed
			return fmt.Errorf("prepare atomic transaction batch recovery finalization: %w", err)
		}
	}

	status, acknowledgmentOutcome, err := r.acknowledgeCompletion(
		completionCtx, source, field, raw, &envelope.Record, completed.Current, batchFinalization,
	)
	if acknowledgmentOutcome != "" {
		outcome = acknowledgmentOutcome
	}

	if err != nil {
		if acknowledgmentOutcome == "" {
			outcome = recoveryMetricOutcomeAckFailed
		}

		return fmt.Errorf("acknowledge engine recovery: %w", err)
	}

	switch status {
	case txRedis.RecoveryAckMissing, txRedis.RecoveryAckDeleted:
		return nil
	case txRedis.RecoveryAckReplaced:
		outcome = recoveryMetricOutcomeRecordChanged
		return fmt.Errorf("%w; replacement retained", errRecoveryRecordChanged)
	case txRedis.RecoveryAckFinalizationRequired:
		outcome = recoveryMetricOutcomeFinalizationFailed
		return errors.New("atomic transaction batch finalization response is not ready; recovery record retained")
	case txRedis.RecoveryAckReceiptChanged:
		outcome = recoveryMetricOutcomeRecordChanged
		return fmt.Errorf("%w; atomic transaction batch receipt changed", errRecoveryRecordChanged)
	default:
		outcome = recoveryMetricOutcomeInvalidAck
		return errors.New("invalid conditional recovery acknowledgment result")
	}
}

//nolint:gocyclo // recovery outcome classification must remain adjacent to each pipeline transition
func (r *recoveryRecordCompleter) complete(ctx context.Context, source txRedis.RecoveryQueueSource, field, raw string, envelope *command.TransactionCompletionRecord) error {
	startedAt := time.Now()
	outcome := recoveryMetricOutcomeCompleted

	metricsCtx := ctx
	defer func() {
		r.emitRecoveryMetrics(metricsCtx, source, outcome, time.Since(startedAt))
	}()

	if err := ctx.Err(); err != nil {
		outcome = recoveryMetricOutcomeContextCanceled
		return err
	}

	if r.completer == nil {
		outcome = recoveryMetricOutcomeNotConfigured
		return errors.New("applied transaction completer is not configured")
	}

	if !supportsRecoveryAcknowledgment(r.queue, source) {
		outcome = recoveryMetricOutcomeNotConfigured
		return errors.New("conditional balance recovery acknowledgment is not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ctx, err := r.resolveRecoveryCompletionContext(ctx, envelope)
	if err != nil {
		outcome = recoveryMetricOutcomeFinalizationFailed
		return err
	}

	completion, err := r.completer.Complete(ctx, envelope)
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

	var batchFinalization *command.AtomicTransactionBatchRecoveryFinalization
	if source == txRedis.RecoveryQueueSourceEngineRecover && r.batchFinalizer != nil {
		batchFinalization, err = r.batchFinalizer.PrepareAtomicTransactionBatchRecoveryFinalization(
			ctx,
			envelope,
			completion,
		)
		if err != nil {
			outcome = recoveryMetricOutcomeFinalizationFailed
			return fmt.Errorf("prepare atomic transaction batch recovery finalization: %w", err)
		}
	}

	status, acknowledgmentOutcome, err := r.acknowledgeCompletion(
		ctx,
		source,
		field,
		raw,
		envelope,
		completion,
		batchFinalization,
	)
	if acknowledgmentOutcome != "" {
		outcome = acknowledgmentOutcome
	}

	if err != nil {
		if acknowledgmentOutcome != "" {
			return err
		}

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
		return errors.New("recovery record changed during completion; replacement retained")
	case txRedis.RecoveryAckFinalizationRequired:
		if batchFinalization != nil {
			outcome = recoveryMetricOutcomeFinalizationFailed
			return errors.New("atomic transaction batch finalization response is not ready; recovery record retained")
		}
	case txRedis.RecoveryAckReceiptChanged:
		if batchFinalization != nil {
			outcome = recoveryMetricOutcomeRecordChanged
			return errors.New("atomic transaction batch receipt changed during finalization; recovery record retained")
		}
	}

	outcome = recoveryMetricOutcomeInvalidAck

	return errors.New("invalid conditional recovery acknowledgment result")
}

// AcknowledgeEngineRecovery removes the exact engine recovery record after the
// normal request path has completed its durable projections. Missing records
// are already acknowledged; changed records are retained for inspection and
// asynchronous recovery.
func (r *recoveryRecordCompleter) AcknowledgeEngineRecovery(
	ctx context.Context,
	envelope *command.TransactionCompletionRecord,
	completion command.TransactionCompletionResult,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if envelope == nil {
		return errors.New("engine recovery acknowledgment requires a completion record")
	}

	reader, ok := r.queue.(recoveryRecordReader)
	if !ok {
		return errors.New("engine recovery acknowledgment is not configured")
	}

	if _, ok := r.queue.(recoveryProtectionAcknowledger); !ok {
		return errors.New("protected engine recovery acknowledgment is not configured")
	}

	field := envelope.TransactionID.String() + ":" + envelope.ExecutionID.String()

	raw, err := reader.ReadRecoveryMessage(ctx, txRedis.RecoveryQueueSourceEngineRecover, field)
	if err != nil {
		return fmt.Errorf("read engine recovery record: %w", err)
	}

	if raw == "" {
		return nil
	}

	writeBehind, err := command.DecodeTransactionWriteBehindEnvelope([]byte(raw))
	if err != nil {
		return fmt.Errorf("decode engine recovery record: %w", err)
	}

	stored := &writeBehind.Record

	if err := sameRecoveryCompletionRecord(envelope, stored); err != nil {
		return err
	}

	status, _, err := r.acknowledgeCompletion(
		ctx,
		txRedis.RecoveryQueueSourceEngineRecover,
		field,
		raw,
		envelope,
		completion,
		nil,
	)
	if err != nil {
		return fmt.Errorf("acknowledge engine recovery record: %w", err)
	}

	switch status {
	case txRedis.RecoveryAckMissing, txRedis.RecoveryAckDeleted:
		return nil
	case txRedis.RecoveryAckReplaced:
		return errors.New("engine recovery record changed before acknowledgment; replacement retained")
	default:
		return errors.New("invalid engine recovery acknowledgment result")
	}
}

func sameRecoveryCompletionRecord(expected, actual *command.TransactionCompletionRecord) error {
	if expected == nil || actual == nil {
		return errors.New("engine recovery record does not match completed execution")
	}

	expectedRaw, err := command.EncodeTransactionCompletionRecord(*expected)
	if err != nil {
		return fmt.Errorf("encode completed engine record: %w", err)
	}

	actualRaw, err := command.EncodeTransactionCompletionRecord(*actual)
	if err != nil {
		return fmt.Errorf("encode stored engine recovery record: %w", err)
	}

	if !bytes.Equal(expectedRaw, actualRaw) {
		return errors.New("engine recovery record does not match completed execution")
	}

	return nil
}

func supportsRecoveryAcknowledgment(queue txRedis.RedisRepository, source txRedis.RecoveryQueueSource) bool {
	if _, ok := queue.(recoveryRecordAcknowledger); ok {
		return true
	}

	if source != txRedis.RecoveryQueueSourceLegacyBackup {
		return false
	}

	_, ok := queue.(legacyRecoveryRecordAcknowledger)

	return ok
}

func (r *recoveryRecordCompleter) acknowledgeCompletion(
	ctx context.Context,
	source txRedis.RecoveryQueueSource,
	field, raw string,
	envelope *command.TransactionCompletionRecord,
	completion command.TransactionCompletionResult,
	batchFinalization *command.AtomicTransactionBatchRecoveryFinalization,
) (int64, string, error) {
	if batchFinalization != nil {
		acknowledger, ok := r.queue.(atomicTransactionBatchRecoveryAcknowledger)
		if !ok {
			return 0, recoveryMetricOutcomeNotConfigured,
				errors.New("atomic transaction batch recovery acknowledgment is not configured")
		}

		terminal, completedAt, outcome, err := r.completionEvidence(completion.Outcome.TransactionStatus)
		if err != nil {
			return 0, outcome, err
		}

		status, err := acknowledger.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
			ctx,
			source,
			envelope.OrganizationID,
			envelope.LedgerID,
			field,
			raw,
			terminal,
			completedAt,
			batchFinalization.ReceiptToken,
			batchFinalization.Transactions,
		)

		return status, "", err
	}

	if protected, ok := r.queue.(recoveryProtectionAcknowledger); ok {
		terminal, completedAt, outcome, err := r.completionEvidence(completion.Outcome.TransactionStatus)
		if err != nil {
			return 0, outcome, err
		}

		status, err := protected.CompareAndDeleteRecoveryWithProtectionFrom(
			ctx, source, envelope.OrganizationID, envelope.LedgerID, field, raw, terminal, completedAt,
		)

		return status, "", err
	}

	if source != txRedis.RecoveryQueueSourceLegacyBackup {
		acknowledger := r.queue.(recoveryRecordAcknowledger)
		status, err := acknowledger.CompareAndDeleteRecoveryFrom(ctx, source, field, raw)

		return status, "", err
	}

	if protected, ok := r.queue.(legacyRecoveryProtectionAcknowledger); ok {
		terminal, completedAt, outcome, err := r.completionEvidence(completion.Outcome.TransactionStatus)
		if err != nil {
			return 0, outcome, err
		}

		status, err := protected.CompareAndDeleteRecoveryWithProtection(
			ctx, envelope.OrganizationID, envelope.LedgerID, field, raw, terminal, completedAt,
		)

		return status, "", err
	}

	if acknowledger, ok := r.queue.(recoveryRecordAcknowledger); ok {
		status, err := acknowledger.CompareAndDeleteRecoveryFrom(ctx, source, field, raw)

		return status, "", err
	}

	acknowledger := r.queue.(legacyRecoveryRecordAcknowledger)
	status, err := acknowledger.CompareAndDeleteRecovery(ctx, field, raw)

	return status, "", err
}

func (r *recoveryRecordCompleter) resolveRecoveryCompletionContext(
	ctx context.Context,
	record *command.TransactionCompletionRecord,
) (context.Context, error) {
	resolver, ok := r.completer.(interface {
		resolveContext(context.Context, *command.TransactionCompletionRecord) (context.Context, error)
	})
	if !ok {
		return ctx, nil
	}

	return resolver.resolveContext(ctx, record)
}

func (r *recoveryRecordCompleter) completionEvidence(status string) (bool, time.Time, string, error) {
	terminal, validStatus := durableRecoveryTerminal(status)
	if !validStatus {
		return false, time.Time{}, recoveryMetricOutcomeFinalizationFailed,
			fmt.Errorf("complete recovered transaction: durable SQL reported unsupported status %q", status)
	}

	if r.clock == nil {
		return false, time.Time{}, recoveryMetricOutcomeNotConfigured,
			errors.New("durable balance recovery completion clock is not configured")
	}

	return terminal, r.clock(), "", nil
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
