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
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// Sentinel errors for DeactivateRuleService constructor validation.
var (
	ErrDeactivateNilRepository = errors.New("repository is required")
	ErrDeactivateNilClock      = errors.New("clock is required")
)

// DeactivateRuleService handles rule deactivation (ACTIVE → INACTIVE).
//
// Persistence contract: the rule status update and the matching audit event are
// persisted atomically inside a single database transaction via executeInTx —
// either both land or neither does. The in-memory rule cache is updated only
// after a successful commit; cache-removal failures are logged but do not fail
// the command (the DB remains the source of truth and the sync worker
// reconciles any cache drift).
type DeactivateRuleService struct {
	repository  RuleRepository
	clock       clock.Clock
	auditWriter AuditWriter
	cacheWriter RuleCacheWriter
	txBeginner  pgdb.TxBeginner
}

// NewDeactivateRuleService creates a new DeactivateRuleService.
// The cacheWriter parameter is optional (nil-safe); when set, it synchronously
// removes the rule from the in-memory cache after a successful deactivation
// commit.
// txBeginner may be nil — in that case Execute will surface pgdb.ErrNilConnection
// when it attempts to start the persistence transaction.
func NewDeactivateRuleService(repository RuleRepository, clk clock.Clock, auditWriter AuditWriter, cacheWriter RuleCacheWriter, txBeginner pgdb.TxBeginner) (*DeactivateRuleService, error) {
	if repository == nil {
		return nil, ErrDeactivateNilRepository
	}

	if clk == nil {
		return nil, ErrDeactivateNilClock
	}

	return &DeactivateRuleService{
		repository:  repository,
		clock:       clk,
		auditWriter: auditWriter,
		cacheWriter: cacheWriter,
		txBeginner:  txBeginner,
	}, nil
}

// Execute deactivates a rule by updating status to INACTIVE.
// Idempotent: if already INACTIVE, returns the rule without error.
// Returns the updated rule for atomic deactivate-and-return pattern.
func (s *DeactivateRuleService) Execute(ctx context.Context, ruleID uuid.UUID) (_ *model.Rule, retErr error) {
	logger, tracer, _, factory := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.rule.deactivate")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, factory, logger, "tracer", "rule_deactivate", start, retErr)
	}()

	logger = logging.WithTrace(ctx, logger)

	span.SetAttributes(
		attribute.String("app.request.rule_id", ruleID.String()),
		attribute.String("app.request.operation", "deactivate"),
	)

	rule, err := s.repository.GetByID(ctx, ruleID)
	if err != nil {
		if errors.Is(err, constant.ErrRuleNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", err)
			logger.With(
				libLog.String("operation", "service.rule.deactivate"),
				libLog.String("rule.id", ruleID.String()),
			).Log(ctx, libLog.LevelWarn, "Rule not found")

			return nil, pkg.ValidateBusinessError(constant.ErrRuleNotFound, constant.EntityRule)
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get rule from repository", err)
		logger.With(
			libLog.String("operation", "service.rule.deactivate"),
			libLog.String("rule.id", ruleID.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to get rule")

		return nil, fmt.Errorf("failed to get rule: %w", err)
	}

	// Defensive check: treat nil rule as not found (guards against repo returning nil, nil)
	if rule == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", constant.ErrRuleNotFound)
		logger.With(
			libLog.String("operation", "service.rule.deactivate"),
			libLog.String("rule.id", ruleID.String()),
		).Log(ctx, libLog.LevelWarn, "Rule not found")

		return nil, pkg.ValidateBusinessError(constant.ErrRuleNotFound, constant.EntityRule)
	}

	// Idempotency: if already inactive, return the rule (no-op)
	// Check before audit capture to avoid unnecessary state snapshots
	if rule.Status == model.RuleStatusInactive {
		logger.With(
			libLog.String("operation", "service.rule.deactivate"),
			libLog.String("rule.id", ruleID.String()),
		).Log(ctx, libLog.LevelDebug, "Rule already inactive (idempotent no-op)")

		return rule, nil
	}

	// Capture "before" state for audit (after idempotency check, before mutation)
	beforeState := RuleToMap(rule)

	// Use domain model method for status transition (validates and maintains invariants)
	if err := rule.SetStatus(model.RuleStatusInactive, s.clock.Now()); err != nil {
		// Check for invalid transition (business error)
		var transitionErr *model.InvalidTransitionError
		if errors.As(err, &transitionErr) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid state transition", transitionErr)
			logger.With(
				libLog.String("operation", "service.rule.deactivate"),
				libLog.String("rule.id", ruleID.String()),
				libLog.String("rule.status_from", string(transitionErr.From)),
				libLog.String("rule.status_to", string(transitionErr.To)),
			).Log(ctx, libLog.LevelWarn, "Invalid transition")

			return nil, transitionErr
		}

		// Technical error (invalid status value or other)
		libOpentelemetry.HandleSpanError(span, "Failed to set rule status", err)
		logger.With(
			libLog.String("operation", "service.rule.deactivate"),
			libLog.String("rule.id", ruleID.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to set rule status")

		return nil, fmt.Errorf("failed to set rule status: %w", err)
	}

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
				libLog.String("operation", "service.rule.deactivate"),
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
			model.AuditEventRuleDeactivated,
			model.AuditActionDeactivate,
			rule.ID,
			beforeState,
			afterState,
			"Rule deactivated via API",
		); err != nil {
			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", err)
			logger.With(
				libLog.String("operation", "service.rule.deactivate.audit"),
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
			libOpentelemetry.HandleSpanError(span, "Failed to deactivate rule transaction", txErr)
			logger.With(
				libLog.String("operation", "service.rule.deactivate"),
				libLog.String("rule.id", ruleID.String()),
				libLog.String("error.message", txErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to deactivate rule")
		}

		return nil, fmt.Errorf("failed to deactivate rule: %w", txErr)
	}

	// POST-COMMIT: remove the rule from the in-memory cache. Reached only when
	// the transaction above committed successfully. The RuleCacheWriter
	// interface is fire-and-forget: the DB remains the source of truth and
	// rule_sync_worker reconciles any cache drift on its next poll. The cache
	// call is wrapped in a recover to keep a cache-layer panic from turning a
	// successfully-committed deactivation into a 5xx response.
	if s.cacheWriter != nil {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.With(
						libLog.String("operation", "service.rule.deactivate.cache"),
						libLog.String("rule.id", rule.ID.String()),
						libLog.String("error.message", fmt.Sprint(recovered)),
					).Log(ctx, libLog.LevelError, "Panic recovered from rule cache removal")
				}
			}()

			s.cacheWriter.RemoveRule(ctx, rule.ID)
		}()
	}

	return rule, nil
}
