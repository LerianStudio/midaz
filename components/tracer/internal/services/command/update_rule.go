// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// Sentinel errors for nil dependencies passed to NewUpdateRuleCommand.
// The atomicity fix made the constructor return (*Cmd, error) so DI
// failures surface at boot rather than at first request.
var (
	// ErrNilUpdateRuleRepository is returned when a nil RuleRepository is
	// passed to NewUpdateRuleCommand.
	ErrNilUpdateRuleRepository = errors.New("update rule repository is nil")
	// ErrNilUpdateRuleCEL is returned when a nil ExpressionCompiler is passed
	// to NewUpdateRuleCommand.
	ErrNilUpdateRuleCEL = errors.New("update rule CEL compiler is nil")
	// ErrNilUpdateRuleClock is returned when a nil clock is passed to
	// NewUpdateRuleCommand.
	ErrNilUpdateRuleClock = errors.New("update rule clock is nil")
	// ErrNilUpdateRuleAuditWriter is returned when a nil AuditWriter is passed
	// to NewUpdateRuleCommand. The audit writer is required because rule
	// updates and audit recording are persisted atomically inside a single
	// transaction.
	ErrNilUpdateRuleAuditWriter = errors.New("update rule audit writer is nil")
	// ErrNilUpdateRuleTxBeginner is returned when a nil TxBeginner is passed
	// to NewUpdateRuleCommand. The tx beginner is required because rule
	// updates and audit recording are persisted atomically inside a single
	// transaction.
	ErrNilUpdateRuleTxBeginner = errors.New("update rule tx beginner is nil")
)

// UpdateRuleInput represents the input for updating an existing rule.
// All fields are optional (pointers) to support partial updates.
type UpdateRuleInput struct {
	Name        *string
	Description *string
	Expression  *string
	Action      *model.Decision
	Scopes      *[]model.Scope
}

// UpdateRuleCommand handles the update of existing rules.
//
// Persistence contract: the rule update and the matching audit event are
// persisted atomically inside a single database transaction via executeInTx —
// either both land or neither does. Audit failure rolls the rule update back,
// so a successful Update implies a successful audit record.
type UpdateRuleCommand struct {
	repo        RuleRepository
	cel         ExpressionCompiler
	clock       clock.Clock
	auditWriter AuditWriter
	txBeginner  pgdb.TxBeginner
}

// NewUpdateRuleCommand creates a new UpdateRuleCommand instance.
// Returns an error if any of repo, cel, clk, auditWriter, or txBeginner is nil
// to catch invalid dependency injection at construction time. The tx beginner
// is required because rule updates and audit recording are persisted
// atomically inside a single transaction.
func NewUpdateRuleCommand(repo RuleRepository, cel ExpressionCompiler, clk clock.Clock, auditWriter AuditWriter, txBeginner pgdb.TxBeginner) (*UpdateRuleCommand, error) {
	if repo == nil {
		return nil, ErrNilUpdateRuleRepository
	}

	if cel == nil {
		return nil, ErrNilUpdateRuleCEL
	}

	if clk == nil {
		return nil, ErrNilUpdateRuleClock
	}

	if auditWriter == nil {
		return nil, ErrNilUpdateRuleAuditWriter
	}

	if txBeginner == nil {
		return nil, ErrNilUpdateRuleTxBeginner
	}

	return &UpdateRuleCommand{
		repo:        repo,
		cel:         cel,
		clock:       clk,
		auditWriter: auditWriter,
		txBeginner:  txBeginner,
	}, nil
}

// Execute updates an existing rule with validation.
//
// The rule update and the corresponding audit event are persisted atomically
// inside a single transaction. If either step fails, the transaction is rolled
// back and the caller observes the original error — the rule update is never
// committed without an audit trail.
func (c *UpdateRuleCommand) Execute(ctx context.Context, id uuid.UUID, input *UpdateRuleInput) (*model.Rule, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.rule.update")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	logger.With(
		libLog.String("operation", "service.rule.update"),
		libLog.String("rule.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Updating rule")

	if input == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Nil input provided", constant.ErrRuleNilInput)

		return nil, constant.ErrRuleNilInput
	}

	rule, err := c.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, constant.ErrRuleNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule not found", err)
			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get rule", err)

		return nil, fmt.Errorf("failed to get rule: %w", err)
	}

	beforeState := RuleToMap(rule)

	// Validate expression update (only for DRAFT rules)
	if input.Expression != nil {
		if rule.Status != model.RuleStatusDraft {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Expression cannot be modified for non-DRAFT rules", constant.ErrExpressionNotModifiable)
			return nil, constant.ErrExpressionNotModifiable
		}

		_, err := c.cel.Compile(ctx, *input.Expression)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid CEL expression", err)
			return nil, err
		}
	}

	// Prepare normalized name if provided
	// Name uniqueness is enforced at the database level with a partial unique index
	// on (context_id, name) WHERE status != 'DELETED'. The repository will return
	// ErrRuleNameAlreadyExistsInCtx if a duplicate name is detected within the same context.
	var normalizedName *string

	if input.Name != nil {
		normalized := NormalizeName(*input.Name)
		normalizedName = &normalized
	}

	// Hoist a single Now() call so SetAction and Update share one timestamp.
	// With clock.Real this also keeps the persisted UpdatedAt aligned with the
	// in-memory state in the (unlikely) case action+other-field are updated
	// together, since both calls then mutate UpdatedAt to the same instant.
	now := c.clock.Now()

	// Validate action FIRST (before any mutations) to ensure atomicity
	if input.Action != nil {
		if err := rule.SetAction(*input.Action, now); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid decision value", err)
			return nil, err
		}
	}

	// Use domain model Update method with normalized name (validates all before mutating any)
	if err := rule.Update(normalizedName, input.Expression, input.Description, input.Scopes, now); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update rule", err)
		return nil, err
	}

	// Persist rule update + audit event atomically. Audit failures roll the
	// rule update back so a successful Execute always implies a successful
	// audit record.
	//
	// Inner callback attributes span errors via HandleSpanError /
	// HandleSpanBusinessErrorEvent and sets reportedInCallback = true. The outer
	// txErr branch only emits HandleSpanError when the failure originated from
	// executeInTx itself (BeginTx, Commit, panic-recovery) — i.e., when no inner
	// branch reported it. This mirrors the pattern in delete_rule.go and avoids
	// double-reporting business errors as technical failures on the span.
	reportedInCallback := false

	txErr := executeInTx(ctx, c.txBeginner, func(db pgdb.DB) error {
		if updateErr := c.repo.UpdateWithTx(ctx, db, rule); updateErr != nil {
			if errors.Is(updateErr, constant.ErrRuleNameAlreadyExistsInCtx) {
				reportedInCallback = true

				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule name already exists in this context", updateErr)

				return updateErr
			}

			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to persist rule update", updateErr)
			logger.With(
				libLog.String("operation", "service.rule.update"),
				libLog.String("rule.id", id.String()),
				libLog.String("error.message", updateErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to update rule")

			return fmt.Errorf("failed to persist rule update: %w", updateErr)
		}

		afterState := RuleToMap(rule)

		if auditErr := c.auditWriter.RecordRuleEventWithTx(
			ctx,
			db,
			model.AuditEventRuleUpdated,
			model.AuditActionUpdate,
			rule.ID,
			beforeState,
			afterState,
			"Rule updated via API",
		); auditErr != nil {
			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", auditErr)
			logger.With(
				libLog.String("operation", "service.rule.update"),
				libLog.String("rule.id", rule.ID.String()),
				libLog.String("error.message", auditErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to update rule")

			return fmt.Errorf("failed to record audit event: %w", auditErr)
		}

		return nil
	})
	if txErr != nil {
		if !reportedInCallback {
			libOpentelemetry.HandleSpanError(span, "Failed to update rule transactionally", txErr)
			logger.With(
				libLog.String("operation", "service.rule.update"),
				libLog.String("rule.id", id.String()),
				libLog.String("error.message", txErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to update rule")
		}

		return nil, fmt.Errorf("failed to update rule: %w", txErr)
	}

	logger.With(
		libLog.String("operation", "service.rule.update"),
		libLog.String("rule.id", rule.ID.String()),
		libLog.String("rule.name", rule.Name),
	).Log(ctx, libLog.LevelInfo, "Rule updated successfully")

	return rule, nil
}
