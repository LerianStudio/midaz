// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// ErrNilRuleRepository is returned when a nil RuleRepository is passed to a rule command constructor.
var ErrNilRuleRepository = errors.New("nil RuleRepository passed to rule command constructor")

// DraftRuleService handles rule draft transition (INACTIVE → DRAFT).
//
// Persistence contract: the rule status update and the matching audit event are
// persisted atomically inside a single database transaction via executeInTx —
// either both land or neither does. The in-memory rule cache is not touched:
// only ACTIVE rules are cached, and INACTIVE → DRAFT transitions a rule that
// is already outside the cache.
type DraftRuleService struct {
	repository  RuleRepository
	clock       clock.Clock
	auditWriter AuditWriter
	txBeginner  pgdb.TxBeginner
}

// NewDraftRuleService creates a new DraftRuleService with dependencies.
// Returns an error if repository or clk is nil to catch invalid dependency injection at construction time.
// txBeginner may be nil — in that case Execute will surface pgdb.ErrNilConnection
// when it attempts to start the persistence transaction.
func NewDraftRuleService(repository RuleRepository, clk clock.Clock, auditWriter AuditWriter, txBeginner pgdb.TxBeginner) (*DraftRuleService, error) {
	if repository == nil {
		return nil, ErrNilRuleRepository
	}

	if clk == nil {
		return nil, ErrNilClock
	}

	return &DraftRuleService{
		repository:  repository,
		clock:       clk,
		auditWriter: auditWriter,
		txBeginner:  txBeginner,
	}, nil
}

// Execute transitions a rule to DRAFT status.
// Idempotent: if already DRAFT, returns the rule without error.
// Returns the updated rule for atomic draft-and-return pattern.
func (s *DraftRuleService) Execute(ctx context.Context, ruleID uuid.UUID) (*model.Rule, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.rule.draft")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if err := libOpentelemetry.SetSpanAttributesFromValue(span, "draft_input", map[string]any{
		"rule_id":   ruleID.String(),
		"operation": "draft",
	}, nil); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set span attributes", err)
	}

	logger.With(
		libLog.String("operation", "service.rule.draft"),
		libLog.String("rule.id", ruleID.String()),
	).Log(ctx, libLog.LevelInfo, "Transitioning rule to draft")

	rule, err := s.repository.GetByID(ctx, ruleID)
	if err != nil {
		if errors.Is(err, constant.ErrRuleNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", err)
			logger.With(
				libLog.String("operation", "service.rule.draft"),
				libLog.String("rule.id", ruleID.String()),
			).Log(ctx, libLog.LevelWarn, "Rule not found")

			return nil, libCommons.ValidateBusinessError(constant.ErrRuleNotFound, "Rule")
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get rule from repository", err)
		logger.With(
			libLog.String("operation", "service.rule.draft"),
			libLog.String("rule.id", ruleID.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to get rule")

		return nil, fmt.Errorf("failed to get rule: %w", err)
	}

	// Defensive check: treat nil rule as not found (guards against repo returning nil, nil)
	if rule == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", constant.ErrRuleNotFound)
		logger.With(
			libLog.String("operation", "service.rule.draft"),
			libLog.String("rule.id", ruleID.String()),
		).Log(ctx, libLog.LevelWarn, "Rule not found")

		return nil, libCommons.ValidateBusinessError(constant.ErrRuleNotFound, "Rule")
	}

	// Idempotency: if already draft, return the rule (no-op)
	// Check before audit capture to avoid unnecessary state snapshots
	if rule.Status == model.RuleStatusDraft {
		logger.With(
			libLog.String("operation", "service.rule.draft"),
			libLog.String("rule.id", ruleID.String()),
		).Log(ctx, libLog.LevelInfo, "Rule already in draft (idempotent no-op)")

		return rule, nil
	}

	// Capture "before" state for audit (after idempotency check, before mutation)
	beforeState := RuleToMap(rule)

	// Use domain model method for status transition (validates and maintains invariants)
	if err := rule.SetStatus(model.RuleStatusDraft, s.clock.Now()); err != nil {
		// Check for invalid transition (business error)
		var transitionErr *model.InvalidTransitionError
		if errors.As(err, &transitionErr) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid state transition", transitionErr)
			logger.With(
				libLog.String("operation", "service.rule.draft"),
				libLog.String("rule.id", ruleID.String()),
				libLog.String("rule.status_from", string(transitionErr.From)),
				libLog.String("rule.status_to", string(transitionErr.To)),
			).Log(ctx, libLog.LevelWarn, "Invalid transition")

			return nil, transitionErr
		}

		// Technical error (invalid status value or other)
		libOpentelemetry.HandleSpanError(span, "Failed to set rule status", err)
		logger.With(
			libLog.String("operation", "service.rule.draft"),
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
				libLog.String("operation", "service.rule.draft"),
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
			model.AuditEventRuleDrafted,
			model.AuditActionDraft,
			rule.ID,
			beforeState,
			afterState,
			"Rule transitioned to draft via API",
		); err != nil {
			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", err)
			logger.With(
				libLog.String("operation", "service.rule.draft.audit"),
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
			libOpentelemetry.HandleSpanError(span, "Failed to transition rule to draft transaction", txErr)
			logger.With(
				libLog.String("operation", "service.rule.draft"),
				libLog.String("rule.id", ruleID.String()),
				libLog.String("error.message", txErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to transition rule to draft")
		}

		return nil, fmt.Errorf("failed to transition rule to draft: %w", txErr)
	}

	logger.With(
		libLog.String("operation", "service.rule.draft"),
		libLog.String("rule.id", rule.ID.String()),
	).Log(ctx, libLog.LevelInfo, "Rule transitioned to draft successfully")

	return rule, nil
}
