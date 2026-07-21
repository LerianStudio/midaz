// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// Sentinel errors for ActivateRuleService constructor validation.
var (
	ErrActivateNilRepository         = errors.New("repository is required")
	ErrActivateNilExpressionCompiler = errors.New("expressionCompiler is required")
	ErrActivateNilClock              = errors.New("clock is required")
)

// ActivateRuleService handles rule activation (DRAFT/INACTIVE → ACTIVE).
//
// Persistence contract: the rule status update and the matching audit event are
// persisted atomically inside a single database transaction via executeInTx —
// either both land or neither does. The in-memory rule cache is updated only
// after a successful commit; cache write failures are logged but do not fail
// the command (the DB remains the source of truth and the sync worker
// reconciles any cache drift).
type ActivateRuleService struct {
	repository         RuleRepository
	expressionCompiler ExpressionCompiler
	clock              clock.Clock
	auditWriter        AuditWriter
	cacheWriter        RuleCacheWriter
	txBeginner         pgdb.TxBeginner

	// Streaming is the lib-streaming Emitter used to publish past-tense domain
	// events; nil disables emission and never fails the request. Set
	// post-construction at bootstrap.
	Streaming libStreaming.Emitter
}

// NewActivateRuleService creates a new ActivateRuleService.
// The cacheWriter parameter is optional (nil-safe); when set, it synchronously
// updates the in-memory cache after a successful activation commit.
// txBeginner may be nil — in that case Execute will surface pgdb.ErrNilConnection
// when it attempts to start the persistence transaction.
func NewActivateRuleService(repository RuleRepository, expressionCompiler ExpressionCompiler, clk clock.Clock, auditWriter AuditWriter, cacheWriter RuleCacheWriter, txBeginner pgdb.TxBeginner) (*ActivateRuleService, error) {
	if repository == nil {
		return nil, ErrActivateNilRepository
	}

	if expressionCompiler == nil {
		return nil, ErrActivateNilExpressionCompiler
	}

	if clk == nil {
		return nil, ErrActivateNilClock
	}

	return &ActivateRuleService{
		repository:         repository,
		expressionCompiler: expressionCompiler,
		clock:              clk,
		auditWriter:        auditWriter,
		cacheWriter:        cacheWriter,
		txBeginner:         txBeginner,
	}, nil
}

// Execute activates a rule by validating its expression and updating status to ACTIVE.
// Idempotent: if already ACTIVE, returns the rule without error.
// Returns the updated rule for atomic activate-and-return pattern.
func (s *ActivateRuleService) Execute(ctx context.Context, ruleID uuid.UUID) (_ *model.Rule, retErr error) {
	logger, tracer, _, factory := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.rule.activate")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, factory, logger, "tracer", "rule_activate", start, retErr)
	}()

	logger = logging.WithTrace(ctx, logger)

	span.SetAttributes(
		attribute.String("app.request.rule_id", ruleID.String()),
		attribute.String("app.request.operation", "activate"),
	)

	rule, err := s.repository.GetByID(ctx, ruleID)
	if err != nil {
		if errors.Is(err, constant.ErrRuleNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", err)
			logger.With(
				libLog.String("operation", "service.rule.activate"),
				libLog.String("rule.id", ruleID.String()),
			).Log(ctx, libLog.LevelWarn, "Rule not found")

			return nil, pkg.ValidateBusinessError(constant.ErrRuleNotFound, constant.EntityRule)
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get rule from repository", err)
		logger.With(
			libLog.String("operation", "service.rule.activate"),
			libLog.String("rule.id", ruleID.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to get rule")

		return nil, fmt.Errorf("failed to get rule: %w", err)
	}

	// Defensive check: treat nil rule as not found (guards against repo returning nil, nil)
	if rule == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", constant.ErrRuleNotFound)
		logger.With(
			libLog.String("operation", "service.rule.activate"),
			libLog.String("rule.id", ruleID.String()),
		).Log(ctx, libLog.LevelWarn, "Rule not found")

		return nil, pkg.ValidateBusinessError(constant.ErrRuleNotFound, constant.EntityRule)
	}

	if rule.Expression == "" {
		err := pkg.ValidateBusinessError(constant.ErrRuleExpressionRequired, constant.EntityRule)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty expression", err)
		logger.With(
			libLog.String("operation", "service.rule.activate"),
			libLog.String("rule.id", ruleID.String()),
		).Log(ctx, libLog.LevelWarn, "Cannot activate rule with empty expression")

		return nil, err
	}

	// Idempotency: if already active, return the rule (no-op)
	// Check before audit capture to avoid unnecessary state snapshots
	if rule.Status == model.RuleStatusActive {
		logger.With(
			libLog.String("operation", "service.rule.activate"),
			libLog.String("rule.id", ruleID.String()),
		).Log(ctx, libLog.LevelDebug, "Rule already active (idempotent no-op)")

		return rule, nil
	}

	logger.With(
		libLog.String("operation", "service.rule.activate"),
		libLog.String("rule.id", ruleID.String()),
	).Log(ctx, libLog.LevelDebug, "Validating expression for rule")

	// Compile the expression BEFORE opening the transaction: compilation is a
	// pure, CPU-bound operation with no database side-effects, so holding row
	// locks for its duration would be wasteful. The compiled program flows into
	// the post-commit cache update below.
	program, err := s.expressionCompiler.Compile(ctx, rule.Expression)
	if err != nil {
		// Propagate the raw compiler sentinel (ErrExpressionSyntax /
		// ErrExpressionType) unchanged, mirroring create_rule.go. Collapsing it
		// into a single fixed sentinel here loses the syntax-vs-type distinction
		// (0340 vs 0341) that classifyServiceError maps to 400.
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression compilation failed", err)
		logger.With(
			libLog.String("operation", "service.rule.activate"),
			libLog.String("rule.id", ruleID.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Expression validation failed")

		return nil, err
	}

	// Capture "before" state for audit AFTER idempotency / expression checks
	// but BEFORE SetStatus mutates the domain object.
	beforeState := RuleToMap(rule)

	// Use domain model method for status transition (validates and maintains invariants)
	if err := rule.SetStatus(model.RuleStatusActive, s.clock.Now()); err != nil {
		// Check for invalid transition (business error)
		var transitionErr *model.InvalidTransitionError
		if errors.As(err, &transitionErr) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid state transition", transitionErr)
			logger.With(
				libLog.String("operation", "service.rule.activate"),
				libLog.String("rule.id", ruleID.String()),
				libLog.String("rule.status_from", string(transitionErr.From)),
				libLog.String("rule.status_to", string(transitionErr.To)),
			).Log(ctx, libLog.LevelWarn, "Invalid transition")

			return nil, transitionErr
		}

		// Technical error (invalid status value or other)
		libOpentelemetry.HandleSpanError(span, "Failed to set rule status", err)
		logger.With(
			libLog.String("operation", "service.rule.activate"),
			libLog.String("rule.id", ruleID.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to set rule status")

		return nil, fmt.Errorf("failed to set rule status: %w", err)
	}

	// Capture "after" state post-mutation for audit.
	afterState := RuleToMap(rule)

	// Persist status change + audit event atomically. The reportedInCallback flag
	// ensures the outer HandleSpanError fires only for transaction-lifecycle
	// failures (BeginTx, Commit, panic recovery) that the inner branches cannot
	// instrument; in-callback errors are already recorded with their specific
	// context and must not be double-reported.
	reportedInCallback := false

	txErr := executeInTx(ctx, s.txBeginner, func(db pgdb.DB) error {
		if err := s.repository.UpdateWithTx(ctx, db, rule); err != nil {
			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to update rule", err)
			logger.With(
				libLog.String("operation", "service.rule.activate"),
				libLog.String("rule.id", ruleID.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to update rule")

			return fmt.Errorf("failed to update rule: %w", err)
		}

		if s.auditWriter == nil {
			return nil
		}

		if err := s.auditWriter.RecordRuleEventWithTx(
			ctx,
			db,
			model.AuditEventRuleActivated,
			model.AuditActionActivate,
			rule.ID,
			beforeState,
			afterState,
			"Rule activated via API",
		); err != nil {
			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", err)
			logger.With(
				libLog.String("operation", "service.rule.activate.audit"),
				libLog.String("rule.id", rule.ID.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to record audit event")

			return fmt.Errorf("failed to record audit event: %w", err)
		}

		return nil
	})
	if txErr != nil {
		// Only mark the span and log at the outer level if the callback did not
		// already do so. In-callback failures (UpdateWithTx, RecordRuleEventWithTx)
		// are fully instrumented at their specific error site. The outer path
		// covers transaction-lifecycle failures (BeginTx, Commit, panic recovery)
		// that cannot be recorded from inside the callback.
		if !reportedInCallback {
			libOpentelemetry.HandleSpanError(span, "Failed to activate rule transaction", txErr)
			logger.With(
				libLog.String("operation", "service.rule.activate"),
				libLog.String("rule.id", ruleID.String()),
				libLog.String("error.message", txErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to activate rule")
		}

		return nil, fmt.Errorf("failed to activate rule: %w", txErr)
	}

	// POST-COMMIT: update the in-memory rule cache. Reached only when the
	// transaction above committed successfully. The RuleCacheWriter interface
	// (see rule_cache_writer.go) is fire-and-forget: the DB is the source of
	// truth and rule_sync_worker reconciles any drift on its next poll. The
	// cache call is wrapped in a recover to keep a cache-layer panic from
	// turning a successfully-committed activation into a 5xx response.
	if s.cacheWriter != nil {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.With(
						libLog.String("operation", "service.rule.activate.cache"),
						libLog.String("rule.id", rule.ID.String()),
						libLog.String("error.message", fmt.Sprint(recovered)),
					).Log(ctx, libLog.LevelError, "Panic recovered from rule cache update")
				}
			}()

			s.cacheWriter.UpsertRule(ctx, rule, program)
			// Backstop: mark the per-tenant cache bucket ready so the first
			// activation on a freshly spawned tenant is usable without waiting
			// for the sync worker's next poll cycle. In single-tenant mode this
			// is also a no-op after WarmUp already marked the default bucket
			// ready, but remains idempotent. See CacheAdapter.GetActiveRules —
			// the IsReady gate returns ErrRuleCacheNotReady until
			// MarkReady is called for the tenant context.
			s.cacheWriter.MarkReady(ctx)
		}()
	}

	s.emitRuleActivatedEvent(ctx, span, logger, rule)

	return rule, nil
}

// emitRuleActivatedEvent publishes the rule.activated event post-commit.
// IMPORTANT posture: emit failures never fail the request.
func (s *ActivateRuleService) emitRuleActivatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, rule *model.Rule) {
	pkgStreaming.EmitImportant(ctx, span, logger, s.Streaming, events.RuleActivatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewRuleActivated(rule).ToEmitRequest(tenantID, rule.UpdatedAt)
		})
}
