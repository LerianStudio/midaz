// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

//go:generate mockgen -source=validation_service.go -destination=mocks/validation_service_mock.go -package=mocks

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/metrics"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	trcConstant "github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/sanitize"
)

// validationPersistTimeout is the maximum duration for transaction validation record persistence.
// 5 seconds is sufficient for DB operations in normal conditions.
const validationPersistTimeout = 5 * time.Second

// validationTxTimeout bounds the entire transaction lifecycle (BeginTx through Commit/Rollback).
// Prevents connection pool exhaustion from stalled queries or deadlocks.
const validationTxTimeout = 10 * time.Second

// Sentinel errors for ValidationService constructor validation.
var (
	ErrNilRuleEvaluator                  = errors.New("rule evaluator cannot be nil")
	ErrNilLimitChecker                   = errors.New("limit checker cannot be nil")
	ErrNilTransactionValidationRepo      = errors.New("transaction validation repository cannot be nil")
	ErrNilTransactionValidationQueryRepo = errors.New("transaction validation query repository cannot be nil")
	ErrNilAuditWriter                    = errors.New("auditWriter cannot be nil")
	ErrNilConn                           = errors.New("database connection cannot be nil")
)

// ValidateResult is the result of transaction validation including idempotency information.
// IsDuplicate is true when the same request_id was already processed - the handler uses
// this to return HTTP 200 (duplicate) vs HTTP 201 (new).
type ValidateResult struct {
	Response    *model.ValidationResponse
	IsDuplicate bool
}

// RuleEvaluator evaluates transaction rules.
type RuleEvaluator interface {
	Execute(ctx context.Context, req *model.ValidationRequest) (*model.EvaluationResult, error)
}

// LimitChecker checks transaction limits.
type LimitChecker interface {
	CheckLimits(ctx context.Context, db pgdb.DB, input *model.CheckLimitsInput) (*model.CheckLimitsOutput, error)
}

// ValidationService orchestrates transaction validation.
type ValidationService struct {
	conn                           pgdb.TxBeginner
	ruleEvaluator                  RuleEvaluator
	limitChecker                   LimitChecker
	transactionValidationRepo      command.TransactionValidationRepository
	transactionValidationQueryRepo query.TransactionValidationRepository
	auditWriter                    AuditWriter
	clock                          clock.Clock
	// mtMetrics emits the canonical tenant_messages_processed_total counter
	// with one label combination per (tenant_id, module, result). Optional —
	// bootstrap wires a no-op when MULTI_TENANT_ENABLED=false, so existing
	// tests do not need to pass an instance.
	mtMetrics metrics.MultiTenantMetrics
}

// NewValidationService creates a new ValidationService with dependency validation.
func NewValidationService(
	conn pgdb.TxBeginner,
	ruleEval RuleEvaluator,
	limitCheck LimitChecker,
	transactionValidationRepo command.TransactionValidationRepository,
	transactionValidationQueryRepo query.TransactionValidationRepository,
	auditWriter AuditWriter,
	clk clock.Clock,
) (*ValidationService, error) {
	if conn == nil {
		return nil, ErrNilConn
	}

	if ruleEval == nil {
		return nil, ErrNilRuleEvaluator
	}

	if limitCheck == nil {
		return nil, ErrNilLimitChecker
	}

	if transactionValidationRepo == nil {
		return nil, ErrNilTransactionValidationRepo
	}

	if transactionValidationQueryRepo == nil {
		return nil, ErrNilTransactionValidationQueryRepo
	}

	if auditWriter == nil {
		return nil, ErrNilAuditWriter
	}

	if clk == nil {
		clk = clock.RealClock{}
	}

	return &ValidationService{
		conn:                           conn,
		ruleEvaluator:                  ruleEval,
		limitChecker:                   limitCheck,
		transactionValidationRepo:      transactionValidationRepo,
		transactionValidationQueryRepo: transactionValidationQueryRepo,
		auditWriter:                    auditWriter,
		clock:                          clk,
		// Default to the zero-cost no-op; bootstrap swaps this via
		// SetMultiTenantMetrics when MULTI_TENANT_ENABLED=true so existing
		// callers are not forced to pass an instance.
		mtMetrics: metrics.NewMultiTenantMetrics(false, nil, nil),
	}, nil
}

// SetMultiTenantMetrics installs a MultiTenantMetrics sink on the service.
// Safe to call once after construction; passing nil restores the no-op.
// Introduced as a setter rather than a constructor parameter to avoid
// breaking every existing test that builds a ValidationService.
func (s *ValidationService) SetMultiTenantMetrics(m metrics.MultiTenantMetrics) {
	if m == nil {
		s.mtMetrics = metrics.NewMultiTenantMetrics(false, nil, nil)
		return
	}

	s.mtMetrics = m
}

// Validate orchestrates the transaction validation flow with idempotency support.
// Returns ValidateResult with IsDuplicate=true for duplicate requests (DD-3: Stripe model).
// Decision precedence: DENY > Limit Exceeded > REVIEW > ALLOW > Default.
//
// # Transactional Flow
//
// The validation uses database transactions to ensure atomicity:
//   - Dedup check and rule evaluation happen OUTSIDE a transaction
//   - DENY-by-rule: persists validation+audit in separate transaction (no counters involved)
//   - Limit checks, validation persistence, and audit recording happen INSIDE a transaction
//   - If limit exceeded or REVIEW: tx.Rollback() atomically undoes counter increments
//   - If ALLOW: COMMIT saves counters, validation record, and audit event atomically
//
// This eliminates the need for compensating rollbacks and their associated failure modes.
func (s *ValidationService) Validate(ctx context.Context, req *model.ValidationRequest) (result *ValidateResult, retErr error) {
	// Emit tenant_messages_processed_total on every exit path. The result
	// label drives SLO dashboards ("% ERROR by tenant") and is derived from
	// the final Decision on the response, or "ERROR" when the service
	// returned a non-nil error before producing a decision.
	//
	// Registered BEFORE the early-return guards (ctx cancelled, nil req) so
	// those error paths still emit the counter — emitMessageProcessed maps
	// retErr != nil and result == nil to "ERROR", so it is safe to fire
	// before the request has even been inspected.
	defer func() {
		s.emitMessageProcessed(ctx, result, retErr)
	}()

	// Check context cancellation FIRST
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if req == nil {
		return nil, errors.New("validation request cannot be nil")
	}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.validation.orchestrate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Step 0: Check for duplicate request (DD-3: Stripe model deduplication)
	// This is done BEFORE any processing to avoid double-counting limits.
	existingValidation, err := s.transactionValidationQueryRepo.FindByRequestID(ctx, req.RequestID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "failed to check for duplicate request", err)

		return nil, fmt.Errorf("failed to check for duplicate request: %w", err)
	}

	if existingValidation != nil {
		// Duplicate detected - return cached response without processing
		logger.With(
			libLog.String("operation", "service.validation.orchestrate"),
			libLog.Any("request.id", req.RequestID),
			libLog.Any("existing.validation.id", existingValidation.ID),
		).Log(ctx, libLog.LevelDebug, "Duplicate request detected - returning cached response")

		span.AddEvent("duplicate_request_detected")

		return &ValidateResult{
			Response:    existingValidation.ToValidationResponse(),
			IsDuplicate: true,
		}, nil
	}

	startTime := time.Now() // Wall clock for latency measurement only
	evaluatedAt := s.clock.Now().UTC()

	// Generate validationId for audit record (used in both response and persistence)
	validationID := uuid.New()

	// Build response
	response := model.NewValidationResponse(validationID, req.RequestID, model.DecisionAllow, evaluatedAt)

	// Step 1: Evaluate rules (OUTSIDE transaction)
	evalResult, err := s.ruleEvaluator.Execute(ctx, req)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "rule evaluation failed", err)

		return nil, fmt.Errorf("rule evaluation failed: %w", err)
	}

	if evalResult == nil {
		libOpentelemetry.HandleSpanError(span, "rule evaluation returned nil", nil)

		return nil, fmt.Errorf("rule evaluation returned nil result")
	}

	// Copy evaluation result to response
	response.EvaluationResult = *evalResult
	response.Decision = evalResult.Decision

	// If DENY by rule, return immediately (don't check limits)
	// Persist using non-transactional helpers since no counters are involved
	if evalResult.Decision == model.DecisionDeny {
		response.ProcessingTimeMs = float64(time.Since(startTime).Nanoseconds()) / 1e6

		return s.finalizeDenyByRule(ctx, req, response, logger), nil
	}

	// Step 2: Begin transaction for limit checks + persistence
	// This ensures atomicity: counter increments, validation record, and audit event
	// are either ALL committed (ALLOW) or ALL rolled back (DENY/REVIEW).
	// txCtx bounds the entire transaction lifecycle to prevent connection pool exhaustion.
	txCtx, txCancel := context.WithTimeout(ctx, validationTxTimeout)
	defer txCancel()

	tx, err := s.conn.BeginTx(txCtx, nil) // nil = default isolation level (typically READ COMMITTED)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "failed to begin transaction", err)

		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Ensure tx is rolled back on panic or early return
	// The defer will be a no-op if we explicitly commit
	defer func() {
		if tx != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				logger.With(
					libLog.String("operation", "service.validation.orchestrate"),
					libLog.String("error", rollbackErr.Error()),
				).Log(ctx, libLog.LevelWarn, "Failed to rollback transaction in defer cleanup")
			}
		}
	}()

	// Step 3: Check limits (INSIDE transaction)
	limitInput := req.ToCheckLimitsInput()

	limitOutput, err := s.limitChecker.CheckLimits(txCtx, tx, limitInput)
	if err != nil {
		// tx.Rollback() will be called by defer
		libOpentelemetry.HandleSpanError(span, "limit check failed", err)

		return nil, fmt.Errorf("limit check failed: %w", err)
	}

	if limitOutput == nil {
		// tx.Rollback() will be called by defer
		libOpentelemetry.HandleSpanError(span, "limit check returned nil", nil)

		return nil, fmt.Errorf("limit check returned nil result")
	}

	response.LimitUsageDetails = limitOutput.LimitUsageDetails
	response.EvaluatedAt = limitOutput.EvaluatedAt

	// If limit exceeded, rollback counters and persist DENY outside tx
	if !limitOutput.Allowed {
		response.Decision = model.DecisionDeny
		response.Reason = "limit_exceeded"
		response.ProcessingTimeMs = float64(time.Since(startTime).Nanoseconds()) / 1e6

		out := s.finalizeNonAllow(ctx, tx, req, response, logger, "limit exceeded")
		tx = nil // ownership transferred — defer must not roll back

		return out, nil
	}

	// Step 4: If rules returned REVIEW, rollback counters and persist outside tx
	// REVIEW means "manual review required" - don't count transaction against limits
	if evalResult.Decision == model.DecisionReview {
		response.ProcessingTimeMs = float64(time.Since(startTime).Nanoseconds()) / 1e6

		out := s.finalizeNonAllow(ctx, tx, req, response, logger, "REVIEW decision")
		tx = nil // ownership transferred — defer must not roll back

		return out, nil
	}

	// Step 5: ALLOW path - persist validation and audit inside transaction, then COMMIT
	response.ProcessingTimeMs = float64(time.Since(startTime).Nanoseconds()) / 1e6

	// The persist-validation/persist-audit/COMMIT sequence is extracted into
	// commitAllowPath to keep Validate under the gocyclo budget. A non-nil dup
	// signals a concurrent-duplicate short-circuit (tx must NOT be detached so
	// the defer rolls back); committed=true means COMMIT succeeded and the
	// caller must detach tx to prevent the defer from rolling it back.
	dup, committed, err := s.commitAllowPath(ctx, txCtx, tx, req, response, span, logger)
	if err != nil {
		return nil, err
	}

	if dup != nil {
		return dup, nil
	}

	if committed {
		tx = nil // Prevent defer from rolling back after successful commit
	}

	return &ValidateResult{
		Response:    response,
		IsDuplicate: false,
	}, nil
}

// commitAllowPath persists the transaction validation and audit event inside
// tx and commits. Extracted from Validate to keep it under the gocyclo budget;
// control flow and side effects are identical to the inlined version.
//
// Return contract:
//   - (dup, false, nil): a concurrent duplicate was detected during persist;
//     the caller returns dup and leaves tx attached so the defer rolls it back.
//   - (nil, true, nil): persist + COMMIT succeeded; the caller detaches tx.
//   - (nil, false, err): a persist or commit error; tx stays attached so the
//     defer rolls it back.
func (s *ValidationService) commitAllowPath(
	ctx, txCtx context.Context,
	tx pgdb.Tx,
	req *model.ValidationRequest,
	response *model.ValidationResponse,
	span trace.Span,
	logger libLog.Logger,
) (*ValidateResult, bool, error) {
	// Persist transaction validation inside tx
	if err := s.persistTransactionValidationWithTx(txCtx, tx, req, response, logger); err != nil {
		if dup := s.handleConcurrentDuplicate(ctx, err, req, logger); dup != nil {
			return dup, false, nil
		}

		// tx.Rollback() will be called by defer
		libOpentelemetry.HandleSpanError(span, "failed to persist transaction validation", err)

		return nil, false, fmt.Errorf("failed to persist transaction validation: %w", err)
	}

	// Persist audit event inside tx
	if err := s.persistAuditEventWithTx(txCtx, tx, req, response, logger); err != nil {
		// tx.Rollback() will be called by defer
		libOpentelemetry.HandleSpanError(span, "failed to persist audit event", err)

		return nil, false, fmt.Errorf("failed to persist audit event: %w", err)
	}

	// COMMIT the transaction
	if err := tx.Commit(); err != nil {
		libOpentelemetry.HandleSpanError(span, "failed to commit transaction", err)

		return nil, false, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil, true, nil
}

// tenantIDFromContext returns the tenant identifier from ctx, or "" in
// single-tenant mode. Kept as a thin wrapper so the intent is obvious at
// the call site and so a future tenancy change (e.g., header-based
// fallback) has a single spot to update.
func tenantIDFromContext(ctx context.Context) string {
	return tmcore.GetTenantIDContext(ctx)
}

// emitMessageProcessed increments the canonical
// tenant_messages_processed_total counter. Called via defer from Validate,
// so it always runs regardless of the return path. Result mapping:
//
//   - retErr != nil          → result="ERROR"
//   - Decision == DecisionDeny → result="DENY"
//   - Decision == DecisionReview → result="REVIEW"
//   - otherwise             → result="ALLOW"
//
// tenantID is pulled from the request context, which is empty in
// single-tenant mode — matches the no-op metrics pass-through contract.
func (s *ValidationService) emitMessageProcessed(ctx context.Context, result *ValidateResult, retErr error) {
	if s.mtMetrics == nil {
		return
	}

	tenantID := tenantIDFromContext(ctx)

	var decisionLabel string

	switch {
	case retErr != nil:
		decisionLabel = "ERROR"
	case result == nil || result.Response == nil:
		decisionLabel = "ERROR"
	case result.Response.Decision == model.DecisionDeny:
		decisionLabel = "DENY"
	case result.Response.Decision == model.DecisionReview:
		decisionLabel = "REVIEW"
	default:
		decisionLabel = "ALLOW"
	}

	s.mtMetrics.IncMessagesProcessed(ctx, tenantID, trcConstant.ModuleName, decisionLabel)
}

// finalizeDenyByRule handles the DENY-by-rule terminal branch: persist the
// validation record and audit event outside of any transaction (no counters
// are involved on this path) and return the canonical ValidateResult.
//
// Preserves the H2 idempotency contract: when persistTransactionValidation
// surfaces command.ErrDuplicateValidation, the loser of a concurrent
// persistence race must return the existing record rather than its locally
// built response — otherwise GET /v1/validations/{id} would 404.
func (s *ValidationService) finalizeDenyByRule(ctx context.Context, req *model.ValidationRequest, resp *model.ValidationResponse, logger libLog.Logger) *ValidateResult {
	if persistErr := s.persistTransactionValidation(ctx, req, resp, logger); persistErr != nil {
		if dup := s.handleConcurrentDuplicate(ctx, persistErr, req, logger); dup != nil {
			return dup
		}
	}

	s.persistAuditEvent(ctx, req, resp, logger)

	return &ValidateResult{
		Response:    resp,
		IsDuplicate: false,
	}
}

// finalizeNonAllow handles the DENY-by-limit and REVIEW terminal branches:
// roll back the in-flight transaction (undoing counter increments), persist
// the validation/audit records outside the transaction, and return the
// canonical ValidateResult. Reuses rollbackAndPersist so the H2 duplicate
// resolution path stays identical to the original code.
//
// NOTE: callers MUST set tx = nil in their own scope after this returns —
// Go can't reassign the caller's local variable from here, and the outer
// deferred Rollback() must become a no-op once the transaction has been
// rolled back inside this helper.
func (s *ValidationService) finalizeNonAllow(ctx context.Context, tx pgdb.Tx, req *model.ValidationRequest, resp *model.ValidationResponse, logger libLog.Logger, reason string) *ValidateResult {
	if dup := s.rollbackAndPersist(ctx, tx, req, resp, logger, reason); dup != nil {
		return dup
	}

	return &ValidateResult{Response: resp, IsDuplicate: false}
}

// rollbackAndPersist rolls back the transaction to undo counter increments,
// then persists validation and audit records outside the transaction.
// Used by DENY-by-limit and REVIEW paths.
//
// Returns a non-nil *ValidateResult when persistTransactionValidation surfaces
// command.ErrDuplicateValidation — a concurrent request with the same
// RequestID won the persistence race and the loser must return that
// canonical record (H2). Mirrors the ALLOW-path behavior of
// handleConcurrentDuplicate. nil means "no duplicate detected; the caller
// should keep its locally-built response".
func (s *ValidationService) rollbackAndPersist(ctx context.Context, tx pgdb.Tx, req *model.ValidationRequest, resp *model.ValidationResponse, logger libLog.Logger, reason string) *ValidateResult {
	if tx != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			logger.With(
				libLog.String("operation", "service.validation.orchestrate"),
				libLog.Any("request.id", req.RequestID),
				libLog.String("error", rollbackErr.Error()),
			).Log(ctx, libLog.LevelWarn, "Failed to rollback transaction for "+reason)
		}
	}

	if persistErr := s.persistTransactionValidation(ctx, req, resp, logger); persistErr != nil {
		// Only ErrDuplicateValidation propagates here — every other path
		// stays best-effort and returns nil. handleConcurrentDuplicate is
		// the canonical pattern (used by the ALLOW path); reusing it keeps
		// the duplicate-resolution shape consistent across all decision
		// branches.
		if dup := s.handleConcurrentDuplicate(ctx, persistErr, req, logger); dup != nil {
			return dup
		}
	}

	s.persistAuditEvent(ctx, req, resp, logger)

	return nil
}

// handleConcurrentDuplicate checks if a persist error is a concurrent duplicate (TOCTOU race)
// and returns the cached response if so. Returns nil if the error is not a duplicate.
func (s *ValidationService) handleConcurrentDuplicate(ctx context.Context, err error, req *model.ValidationRequest, logger libLog.Logger) *ValidateResult {
	if !errors.Is(err, command.ErrDuplicateValidation) {
		return nil
	}

	logger.With(
		libLog.String("operation", "service.validation.orchestrate"),
		libLog.Any("request.id", req.RequestID),
	).Log(ctx, libLog.LevelDebug, "Concurrent duplicate detected - fetching cached response")

	existing, findErr := s.transactionValidationQueryRepo.FindByRequestID(ctx, req.RequestID)
	if findErr != nil {
		logger.With(
			libLog.String("operation", "service.validation.orchestrate"),
			libLog.Any("request.id", req.RequestID),
			libLog.String("error", findErr.Error()),
		).Log(ctx, libLog.LevelWarn, "Failed to fetch cached response for concurrent duplicate")

		return nil
	}

	if existing != nil {
		return &ValidateResult{
			Response:    existing.ToValidationResponse(),
			IsDuplicate: true,
		}
	}

	return nil
}

// persistTransactionValidation persists a transaction validation record synchronously.
//
// # Error Contract: BEST-EFFORT WITH DUPLICATE DETECTION
//
// This method is used outside transactions (DENY-by-rule, DENY-by-limit, REVIEW paths).
// Most errors are logged and metrics emitted; the validation response is still
// delivered to the client. Two cases differ:
//
//  1. command.ErrDuplicateValidation — returned to caller. The non-tx paths
//     (DENY-by-rule, REVIEW, DENY-by-limit) all generate a fresh validationID,
//     so a unique-constraint violation here means a concurrent request with the
//     same RequestID won the persistence race. Without this signal the loser
//     would return a ValidateResult whose validationID has no DB record, and
//     GET /v1/validations/{id} would 404 — a broken idempotency contract (H2).
//     Callers swap the response with handleConcurrentDuplicate.
//
//  2. All other errors are logged but suppressed (returns nil) so the rest of
//     the response flow is unaffected. This preserves the historical
//     "compliance write best-effort, never block the client" behavior on
//     transient DB errors.
//
// This differs from persistTransactionValidationWithTx, which returns ALL
// errors so the caller can let the deferred tx.Rollback() unwind atomically.
//
// # Design Decision: Synchronous Persistence
//
// Persistence is synchronous to ensure that validation records are immediately
// available for retrieval via GET /v1/validations/{id}. This design prioritizes:
//
//  1. Consistency: Validation records are available immediately after POST returns
//  2. Compliance: SOX/GLBA audit trail is guaranteed before response is sent
//  3. Simplicity: No race conditions between validation response and queries
//
// Trade-offs accepted:
//   - Slightly higher latency (~5-10ms for DB write)
//   - Validation response blocked on DB availability
//
// The timeout (validationPersistTimeout) bounds the maximum wait time.
func (s *ValidationService) persistTransactionValidation(ctx context.Context, req *model.ValidationRequest, resp *model.ValidationResponse, logger libLog.Logger) error {
	tv, err := buildTransactionValidation(req, resp, s.clock.Now().UTC())
	if err != nil {
		logger.With(
			libLog.Any("request.id", resp.RequestID),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "failed to build transaction validation record")

		return nil
	}

	// Extract tracer and metricsFactory from context for observability
	_, tracer, _, metricsFactory := libObservability.NewTrackingFromContext(ctx)

	// Create context with timeout for DB operation.
	// WithoutCancel preserves trace/values from ctx but detaches cancellation,
	// ensuring persistence has full timeout budget even if request context is cancelled.
	// This ensures SOX/GLBA compliance persistence completes regardless of client-side cancellation.
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), validationPersistTimeout)
	defer cancel()

	// Create span for tracing
	persistCtx, span := tracer.Start(persistCtx, "transaction-validation.persist")
	defer span.End()

	logger = logging.WithTrace(persistCtx, logger)

	if err := s.transactionValidationRepo.Insert(persistCtx, tv); err != nil {
		// Concurrent-duplicate races (H2): bubble up so the caller can swap
		// the response with the existing record via handleConcurrentDuplicate.
		// All other persistence errors stay best-effort to preserve the
		// historical "never block the client on a transient DB error" contract.
		if errors.Is(err, command.ErrDuplicateValidation) {
			span.AddEvent("duplicate_request_id_detected_outside_tx")

			return err
		}

		libOpentelemetry.HandleSpanError(span, "failed to persist transaction validation record", err)

		// Emit metric for alerting (compliance risk: audit trail gap)
		if metricsFactory != nil {
			if counter, cErr := metricsFactory.Counter(MetricAuditPersistFailures); cErr == nil && counter != nil {
				_ = counter.Add(persistCtx, 1)
			}
		}

		logger.With(
			libLog.Any("request.id", resp.RequestID),
			libLog.String("error.message", err.Error()),
		).Log(persistCtx, libLog.LevelError, "failed to persist transaction validation record")
	}

	return nil
}

// buildTransactionValidation creates and populates a TransactionValidation record from the request and response.
// Populates all compliance fields (SOX/GLBA) and validates the record before returning.
// createdAt should come from the injected clock for testability.
func buildTransactionValidation(req *model.ValidationRequest, resp *model.ValidationResponse, createdAt time.Time) (*model.TransactionValidation, error) {
	tv, err := model.NewTransactionValidation(resp.ValidationID, resp.Decision, createdAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction validation record: %w", err)
	}

	tv.RequestID = req.RequestID
	tv.TransactionType = req.TransactionType
	tv.SubType = req.SubType
	tv.Amount = req.Amount
	tv.Currency = req.Currency
	tv.TransactionTimestamp = req.TransactionTimestamp
	tv.Account = req.Account
	tv.Segment = req.Segment
	tv.Portfolio = req.Portfolio
	tv.Merchant = req.Merchant
	tv.Metadata = sanitize.SanitizeMetadata(req.Metadata)
	tv.EvaluationResult = resp.EvaluationResult
	tv.LimitUsageDetails = resp.LimitUsageDetails
	tv.ProcessingTimeMs = resp.ProcessingTimeMs

	if err := validateTransactionValidation(tv); err != nil {
		return nil, fmt.Errorf("transaction validation record validation failed: %w", err)
	}

	return tv, nil
}

// validateTransactionValidation ensures compliance-critical fields are present before persistence.
// Returns an error if any required field for SOX/GLBA compliance is missing or invalid.
// This is a defensive check - these fields should always be populated by the validation flow.
func validateTransactionValidation(tv *model.TransactionValidation) error {
	// ID must be valid (not nil UUID)
	if tv.ID == (uuid.UUID{}) {
		return errors.New("transaction validation ID is nil")
	}

	// Decision must be valid
	if !tv.Decision.IsValid() {
		return fmt.Errorf("invalid decision: %s", tv.Decision)
	}

	// RequestID must be valid
	if tv.RequestID == (uuid.UUID{}) {
		return errors.New("transaction validation request ID is nil")
	}

	// TransactionType must be valid
	if !tv.TransactionType.IsValid() {
		return fmt.Errorf("invalid transaction type: %s", tv.TransactionType)
	}

	// Amount must be positive
	if tv.Amount.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("invalid amount: %s", tv.Amount.String())
	}

	// Currency must not be empty
	if tv.Currency == "" {
		return errors.New("currency is empty")
	}

	// TransactionTimestamp must be valid for compliance audit trail
	if tv.TransactionTimestamp.IsZero() {
		return errors.New("transaction timestamp is zero")
	}

	// Account ID must be valid
	if tv.Account.ID == (uuid.UUID{}) {
		return errors.New("account ID is nil")
	}

	return nil
}

// buildRequestSnapshot creates the request snapshot map used for audit event persistence.
func buildRequestSnapshot(req *model.ValidationRequest) map[string]any {
	requestSnapshot := map[string]any{
		"requestId":       req.RequestID.String(),
		"transactionType": req.TransactionType,
		"subType":         req.SubType,
		"amount":          req.Amount,
		"currency":        req.Currency,
		"timestamp":       req.TransactionTimestamp,
		"account": map[string]any{
			"id":       req.Account.ID.String(),
			"type":     req.Account.Type,
			"status":   req.Account.Status,
			"metadata": req.Account.Metadata,
		},
		"metadata": req.Metadata,
	}

	if req.Segment != nil {
		requestSnapshot["account"].(map[string]any)["segmentId"] = req.Segment.ID.String()
		requestSnapshot["segment"] = map[string]any{
			"segmentId": req.Segment.ID.String(),
			"name":      req.Segment.Name,
			"metadata":  req.Segment.Metadata,
		}
	}

	if req.Portfolio != nil {
		requestSnapshot["account"].(map[string]any)["portfolioId"] = req.Portfolio.ID.String()
		requestSnapshot["portfolio"] = map[string]any{
			"portfolioId": req.Portfolio.ID.String(),
			"name":        req.Portfolio.Name,
			"metadata":    req.Portfolio.Metadata,
		}
	}

	if req.Merchant != nil {
		requestSnapshot["merchant"] = map[string]any{
			"merchantId": req.Merchant.ID,
			"name":       req.Merchant.Name,
			"category":   req.Merchant.Category,
			"country":    req.Merchant.Country,
			"metadata":   req.Merchant.Metadata,
		}
	}

	return requestSnapshot
}

// persistAuditEvent persists an audit event for the transaction validation.
//
// # Error Contract: BEST-EFFORT (errors logged, NOT returned)
//
// See persistTransactionValidation for rationale.
// Note: actor identity (Principal) and clientIP are resolved from ctx inside
// the audit writer — see RecordAuditEventCommand.resolveActor.
func (s *ValidationService) persistAuditEvent(ctx context.Context, req *model.ValidationRequest, resp *model.ValidationResponse, logger libLog.Logger) {
	requestSnapshot := buildRequestSnapshot(req)

	responseContext := model.ValidationResponseContext{
		ProcessingTimeMs:  resp.ProcessingTimeMs,
		LimitUsageDetails: resp.LimitUsageDetails,
	}

	// Extract tracer and metricsFactory from context for observability
	_, tracer, _, metricsFactory := libObservability.NewTrackingFromContext(ctx)

	// Create context with timeout for audit persistence.
	// WithoutCancel preserves trace/values from ctx but detaches cancellation,
	// ensuring persistence has full timeout budget even if request context is cancelled.
	// This ensures SOX/GLBA compliance persistence completes regardless of client-side cancellation.
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), validationPersistTimeout)
	defer cancel()

	// Create span for tracing
	persistCtx, span := tracer.Start(persistCtx, "service.validation.audit.persist")
	defer span.End()

	logger = logging.WithTrace(persistCtx, logger)

	if err := s.auditWriter.RecordValidationEvent(
		persistCtx,
		resp.ValidationID, // validationID (used as resource_id)
		requestSnapshot,
		resp.EvaluationResult,
		responseContext,
	); err != nil {
		libOpentelemetry.HandleSpanError(span, "failed to persist audit event", err)

		// Emit metric for alerting (compliance risk: audit trail gap)
		if metricsFactory != nil {
			if counter, cErr := metricsFactory.Counter(MetricAuditPersistFailures); cErr == nil && counter != nil {
				_ = counter.Add(persistCtx, 1)
			}
		}

		logger.With(
			libLog.Any("request.id", req.RequestID),
			libLog.String("error.message", err.Error()),
		).Log(persistCtx, libLog.LevelError, "failed to persist audit event")
	}
}

// persistTransactionValidationWithTx persists a transaction validation record using the provided transaction.
//
// # Error Contract: STRICT (errors returned to caller)
//
// This method is used inside transactions (ALLOW path). Errors are returned so the caller
// can let the deferred tx.Rollback() undo all changes atomically (counters + validation + audit).
// This differs from persistTransactionValidation which uses best-effort semantics.
func (s *ValidationService) persistTransactionValidationWithTx(ctx context.Context, tx pgdb.DB, req *model.ValidationRequest, resp *model.ValidationResponse, logger libLog.Logger) error {
	tv, err := buildTransactionValidation(req, resp, s.clock.Now().UTC())
	if err != nil {
		logger.With(
			libLog.Any("request.id", resp.RequestID),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "failed to build transaction validation record")

		return fmt.Errorf("failed to build transaction validation record: %w", err)
	}

	if err := s.transactionValidationRepo.InsertWithTx(ctx, tx, tv); err != nil {
		logger.With(
			libLog.Any("request.id", resp.RequestID),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "failed to persist transaction validation record")

		return fmt.Errorf("failed to persist transaction validation record: %w", err)
	}

	return nil
}

// persistAuditEventWithTx persists an audit event using the provided transaction.
//
// # Error Contract: STRICT (errors returned to caller)
//
// See persistTransactionValidationWithTx for rationale.
func (s *ValidationService) persistAuditEventWithTx(ctx context.Context, tx pgdb.DB, req *model.ValidationRequest, resp *model.ValidationResponse, logger libLog.Logger) error {
	requestSnapshot := buildRequestSnapshot(req)

	responseContext := model.ValidationResponseContext{
		ProcessingTimeMs:  resp.ProcessingTimeMs,
		LimitUsageDetails: resp.LimitUsageDetails,
	}

	// Extract metricsFactory for observability on error path
	_, _, _, metricsFactory := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled // only metricsFactory needed

	if err := s.auditWriter.RecordValidationEventWithTx(
		ctx,
		tx,
		resp.ValidationID, // validationID (used as resource_id)
		requestSnapshot,
		resp.EvaluationResult,
		responseContext,
	); err != nil {
		// Use detached context for metric/log so they survive even if ctx is cancelled
		telemetryCtx := context.WithoutCancel(ctx)

		// Emit metric for alerting (compliance risk: audit trail gap on ALLOW path)
		if metricsFactory != nil {
			if counter, cErr := metricsFactory.Counter(MetricAuditPersistFailures); cErr == nil && counter != nil {
				_ = counter.Add(telemetryCtx, 1)
			}
		}

		logger.With(
			libLog.Any("request.id", req.RequestID),
			libLog.String("error.message", err.Error()),
		).Log(telemetryCtx, libLog.LevelError, "failed to persist audit event")

		return fmt.Errorf("failed to persist audit event: %w", err)
	}

	return nil
}
