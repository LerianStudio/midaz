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
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// DeleteRuleService handles rule deletion (DRAFT/INACTIVE → DELETED).
//
// Persistence contract: the soft-delete and the matching audit event are
// persisted atomically inside a single database transaction via executeInTx —
// either both land or neither does. The in-memory rule cache is not touched:
// only ACTIVE rules are cached, and DRAFT/INACTIVE → DELETED transitions a
// rule that is already outside the cache.
type DeleteRuleService struct {
	repository  RuleRepository
	auditWriter AuditWriter
	txBeginner  pgdb.TxBeginner
}

var ErrNilDeleteRuleRepository = errors.New("delete rule repository is nil")
var ErrNilAuditWriter = errors.New("audit writer is nil")

// NewDeleteRuleService creates a new DeleteRuleService.
// txBeginner may be nil — in that case Execute will surface pgdb.ErrNilConnection
// when it attempts to start the persistence transaction.
func NewDeleteRuleService(repository RuleRepository, auditWriter AuditWriter, txBeginner pgdb.TxBeginner) (*DeleteRuleService, error) {
	if repository == nil {
		return nil, ErrNilDeleteRuleRepository
	}

	if auditWriter == nil {
		return nil, ErrNilAuditWriter
	}

	return &DeleteRuleService{
		repository:  repository,
		auditWriter: auditWriter,
		txBeginner:  txBeginner,
	}, nil
}

// Execute soft-deletes a rule by updating status to DELETED.
// Only DRAFT/INACTIVE rules can be deleted per state machine.
// Idempotent: if already DELETED, returns success without error.
func (s *DeleteRuleService) Execute(ctx context.Context, ruleID uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.rule.delete")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if err := libOpentelemetry.SetSpanAttributesFromValue(span, "delete_input", map[string]any{
		"rule_id":   ruleID.String(),
		"operation": "delete",
	}, nil); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set span attributes", err)
	}

	logger.With(
		libLog.String("operation", "service.rule.delete"),
		libLog.String("rule.id", ruleID.String()),
	).Log(ctx, libLog.LevelInfo, "Deleting rule")

	rule, err := s.repository.GetByID(ctx, ruleID)
	if err != nil {
		if errors.Is(err, constant.ErrRuleNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", err)
			logger.With(
				libLog.String("operation", "service.rule.delete"),
				libLog.String("rule.id", ruleID.String()),
			).Log(ctx, libLog.LevelWarn, "Rule not found")

			return libCommons.ValidateBusinessError(constant.ErrRuleNotFound, "Rule")
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get rule from repository", err)
		logger.With(
			libLog.String("operation", "service.rule.delete"),
			libLog.String("rule.id", ruleID.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to get rule")

		return fmt.Errorf("failed to get rule: %w", err)
	}

	// Defensive check: treat nil rule as not found (guards against repo returning nil, nil)
	if rule == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", constant.ErrRuleNotFound)
		logger.With(
			libLog.String("operation", "service.rule.delete"),
			libLog.String("rule.id", ruleID.String()),
		).Log(ctx, libLog.LevelWarn, "Rule not found")

		return libCommons.ValidateBusinessError(constant.ErrRuleNotFound, "Rule")
	}

	// Idempotency: if already deleted, return success (no-op)
	// Check before audit capture to avoid unnecessary state snapshots
	if rule.Status == model.RuleStatusDeleted {
		logger.With(
			libLog.String("operation", "service.rule.delete"),
			libLog.String("rule.id", ruleID.String()),
		).Log(ctx, libLog.LevelInfo, "Rule already deleted (idempotent no-op)")

		return nil
	}

	// Check if transition is valid (only DRAFT/INACTIVE → DELETED allowed)
	if !rule.Status.CanTransitionTo(model.RuleStatusDeleted) {
		err := model.NewInvalidTransitionError(rule.Status, model.RuleStatusDeleted)
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid state transition", err)
		logger.With(
			libLog.String("operation", "service.rule.delete"),
			libLog.String("rule.id", ruleID.String()),
			libLog.String("rule.status_from", string(rule.Status)),
			libLog.String("rule.status_to", "DELETED"),
		).Log(ctx, libLog.LevelWarn, "Invalid transition")

		return err
	}

	// Capture "before" state for audit (after idempotency + transition checks,
	// before the soft-delete mutates the rule row).
	beforeState := RuleToMap(rule)

	// Persist deletion + audit event atomically. The reportedInCallback flag
	// ensures the outer HandleSpanError fires only for transaction-lifecycle
	// failures (BeginTx, Commit, panic recovery) that the inner branches cannot
	// instrument; in-callback errors are already recorded with their specific
	// context and must not be double-reported.
	reportedInCallback := false

	txErr := executeInTx(ctx, s.txBeginner, func(db pgdb.DB) error {
		if err := s.repository.DeleteWithTx(ctx, db, ruleID); err != nil {
			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to delete rule", err)
			logger.With(
				libLog.String("operation", "service.rule.delete"),
				libLog.String("rule.id", ruleID.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to delete rule")

			return fmt.Errorf("failed to delete rule: %w", err)
		}

		if err := s.auditWriter.RecordRuleEventWithTx(
			ctx,
			db,
			model.AuditEventRuleDeleted,
			model.AuditActionDelete,
			ruleID,
			beforeState,
			nil, // no "after" state for delete
			"Rule deleted via API",
		); err != nil {
			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", err)
			logger.With(
				libLog.String("operation", "service.rule.delete.audit"),
				libLog.String("rule.id", ruleID.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to record audit event")

			return fmt.Errorf("failed to record audit event: %w", err)
		}

		return nil
	})
	if txErr != nil {
		// Only mark the span and log at the outer level if the callback did not
		// already do so. In-callback failures (DeleteWithTx, RecordRuleEventWithTx)
		// are fully instrumented at their specific error site. The outer path
		// covers transaction-lifecycle failures (BeginTx, Commit, panic recovery)
		// that cannot be recorded from inside the callback.
		if !reportedInCallback {
			libOpentelemetry.HandleSpanError(span, "Failed to delete rule transaction", txErr)
			logger.With(
				libLog.String("operation", "service.rule.delete"),
				libLog.String("rule.id", ruleID.String()),
				libLog.String("error.message", txErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to delete rule")
		}

		return fmt.Errorf("failed to delete rule: %w", txErr)
	}

	logger.With(
		libLog.String("operation", "service.rule.delete"),
		libLog.String("rule.id", ruleID.String()),
	).Log(ctx, libLog.LevelInfo, "Rule deleted successfully")

	return nil
}
