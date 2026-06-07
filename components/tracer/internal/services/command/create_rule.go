// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

//go:generate mockgen -source=create_rule.go -destination=expression_compiler_mock.go -package=command

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

var multipleSpaces = regexp.MustCompile(`\s+`)

// Sentinel errors for nil dependencies passed to NewCreateRuleCommand.
// The atomicity fix made the constructor return (*Cmd, error) so DI
// failures surface at boot rather than at first request.
var (
	// ErrNilCreateRuleRepository is returned when a nil RuleRepository is
	// passed to NewCreateRuleCommand.
	ErrNilCreateRuleRepository = errors.New("create rule repository is nil")
	// ErrNilCreateRuleCEL is returned when a nil ExpressionCompiler is passed
	// to NewCreateRuleCommand.
	ErrNilCreateRuleCEL = errors.New("create rule CEL compiler is nil")
	// ErrNilCreateRuleClock is returned when a nil clock is passed to
	// NewCreateRuleCommand.
	ErrNilCreateRuleClock = errors.New("create rule clock is nil")
	// ErrNilCreateRuleAuditWriter is returned when a nil AuditWriter is passed
	// to NewCreateRuleCommand. The audit writer is required because rule
	// creation and audit recording are persisted atomically inside a single
	// transaction.
	ErrNilCreateRuleAuditWriter = errors.New("create rule audit writer is nil")
	// ErrNilCreateRuleTxBeginner is returned when a nil TxBeginner is passed
	// to NewCreateRuleCommand. The tx beginner is required because rule
	// creation and audit recording are persisted atomically inside a single
	// transaction.
	ErrNilCreateRuleTxBeginner = errors.New("create rule tx beginner is nil")
)

// NormalizeName normalizes a rule name for uniqueness comparison.
// Applies: lowercase + trim whitespace + collapse multiple spaces.
// Example: "  mInha    REGRA  xpto " -> "minha regra xpto"
func NormalizeName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	return multipleSpaces.ReplaceAllString(name, " ")
}

// ExpressionCompiler validates CEL expressions.
// Interface defined locally per Ring pattern.
type ExpressionCompiler interface {
	Compile(ctx context.Context, expression string) (any, error)
}

// CreateRuleInput represents the input for creating a new rule.
type CreateRuleInput struct {
	Name        string
	Description string
	Expression  string
	Action      model.Decision
	Scopes      []model.Scope
}

// CreateRuleCommand handles the creation of new rules.
//
// Persistence contract: the rule insert and the matching audit event are
// persisted atomically inside a single database transaction via executeInTx —
// either both land or neither does. Audit failure rolls the rule insert back,
// so a successful Create implies a successful audit record.
type CreateRuleCommand struct {
	repo        RuleRepository
	cel         ExpressionCompiler
	clock       clock.Clock
	auditWriter AuditWriter
	txBeginner  pgdb.TxBeginner
}

// NewCreateRuleCommand creates a new CreateRuleCommand instance.
// Returns an error if any of repo, cel, clk, auditWriter, or txBeginner is nil
// to catch invalid dependency injection at construction time. The tx beginner
// is required because rule creation and audit recording are persisted
// atomically inside a single transaction.
func NewCreateRuleCommand(repo RuleRepository, cel ExpressionCompiler, clk clock.Clock, auditWriter AuditWriter, txBeginner pgdb.TxBeginner) (*CreateRuleCommand, error) {
	if repo == nil {
		return nil, ErrNilCreateRuleRepository
	}

	if cel == nil {
		return nil, ErrNilCreateRuleCEL
	}

	if clk == nil {
		return nil, ErrNilCreateRuleClock
	}

	if auditWriter == nil {
		return nil, ErrNilCreateRuleAuditWriter
	}

	if txBeginner == nil {
		return nil, ErrNilCreateRuleTxBeginner
	}

	return &CreateRuleCommand{
		repo:        repo,
		cel:         cel,
		clock:       clk,
		auditWriter: auditWriter,
		txBeginner:  txBeginner,
	}, nil
}

// Execute creates a new rule with validation.
//
// The rule insert and the corresponding audit event are persisted atomically
// inside a single transaction. If either step fails, the transaction is rolled
// back and the caller observes the original error — the rule insert is never
// committed without an audit trail.
func (c *CreateRuleCommand) Execute(ctx context.Context, input *CreateRuleInput) (_ *model.Rule, retErr error) {
	logger, tracer, _, factory := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.rule.create")
	defer span.End()

	start := time.Now()
	defer func() {
		utils.RecordDomainOperation(ctx, factory, logger, "tracer", "rule_create", start, retErr)
	}()

	logger = logging.WithTrace(ctx, logger)

	// Handle nil input
	if input == nil {
		err := constant.ErrRuleNilInput
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Nil input provided", err)
		logger.With(
			libLog.String("operation", "service.rule.create"),
		).Log(ctx, libLog.LevelWarn, "Nil input provided")

		return nil, err
	}

	// Normalize name for storage and uniqueness check
	normalizedName := NormalizeName(input.Name)

	// 1. Validate CEL expression syntax
	_, err := c.cel.Compile(ctx, input.Expression)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid CEL expression", err)
		return nil, err
	}

	// 2. Build rule entity using validating constructor (store normalized name)
	// model.NewRule normalizes nil scopes to empty slice for proper JSON serialization
	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	now := c.clock.Now()

	rule, err := model.NewRule(normalizedName, input.Expression, input.Action, input.Scopes, description, now)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid rule input", err)
		logger.With(
			libLog.String("operation", "service.rule.create"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Invalid rule input")

		return nil, err
	}

	// 3. Persist rule insert + audit event atomically. Audit failures roll the
	// rule insert back so a successful Execute always implies a successful
	// audit record.
	var result *model.Rule

	// Inner callback attributes span errors via HandleSpanError /
	// HandleSpanBusinessErrorEvent and sets reportedInCallback = true. The outer
	// txErr branch only emits HandleSpanError when the failure originated from
	// executeInTx itself (BeginTx, Commit, panic-recovery) — i.e., when no inner
	// branch reported it. This mirrors the pattern in delete_rule.go and avoids
	// double-reporting business errors as technical failures on the span.
	reportedInCallback := false

	txErr := executeInTx(ctx, c.txBeginner, func(db pgdb.DB) error {
		created, repoErr := c.repo.CreateWithTx(ctx, db, rule)
		if repoErr != nil {
			if errors.Is(repoErr, constant.ErrRuleNameAlreadyExistsInCtx) {
				reportedInCallback = true

				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Rule name already exists in this context", repoErr)

				return repoErr
			}

			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to insert rule", repoErr)
			logger.With(
				libLog.String("operation", "service.rule.create"),
				libLog.String("error.message", repoErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to create rule")

			return fmt.Errorf("failed to insert rule: %w", repoErr)
		}

		afterState := RuleToMap(created)

		if auditErr := c.auditWriter.RecordRuleEventWithTx(
			ctx,
			db,
			model.AuditEventRuleCreated,
			model.AuditActionCreate,
			created.ID,
			nil,        // no "before" state for create
			afterState, // "after" state is the created rule
			"Rule created via API",
		); auditErr != nil {
			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", auditErr)
			logger.With(
				libLog.String("operation", "service.rule.create"),
				libLog.String("rule.id", created.ID.String()),
				libLog.String("error.message", auditErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to create rule")

			return fmt.Errorf("failed to record audit event: %w", auditErr)
		}

		result = created

		return nil
	})
	if txErr != nil {
		if !reportedInCallback {
			libOpentelemetry.HandleSpanError(span, "Failed to create rule transactionally", txErr)
			logger.With(
				libLog.String("operation", "service.rule.create"),
				libLog.String("error.message", txErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to create rule")
		}

		return nil, fmt.Errorf("failed to create rule: %w", txErr)
	}

	return result, nil
}
