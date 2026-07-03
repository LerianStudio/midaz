// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// ErrNilLimitRepository is returned when a nil LimitRepository is passed to a limit command constructor.
var ErrNilLimitRepository = errors.New("nil LimitRepository passed to limit command constructor")

// ErrNilClock is returned when a nil Clock is passed to a command constructor.
var ErrNilClock = errors.New("nil Clock passed to command constructor")

// ErrNilCreateLimitAuditWriter is returned when a nil AuditWriter is passed to
// NewCreateLimitCommand. The audit writer is required because limit creation
// and audit recording are persisted atomically inside a single transaction.
var ErrNilCreateLimitAuditWriter = errors.New("nil AuditWriter passed to create limit command constructor")

// ErrNilCreateLimitTxBeginner is returned when a nil TxBeginner is passed to
// NewCreateLimitCommand. The tx beginner is required because limit creation
// and audit recording are persisted atomically inside a single transaction.
var ErrNilCreateLimitTxBeginner = errors.New("nil TxBeginner passed to create limit command constructor")

// CreateLimitInput defines input for creating a limit.
//
// ARCHITECTURE NOTE: This struct intentionally mirrors in.CreateLimitInput from the HTTP layer.
// The HTTP struct contains validation tags (validate:"...") for request validation,
// while this command struct is a pure DTO without framework dependencies.
// The HTTP layer converts its struct to this one via in.ToCreateLimitServiceInput()
// before calling command handlers. This separation ensures:
//   - HTTP adapter owns request validation (tags, format checks)
//   - Command layer remains framework-agnostic and testable
//   - Domain model (model.NewLimit) performs business validation
type CreateLimitInput struct {
	Name            string
	Description     *string
	LimitType       model.LimitType
	MaxAmount       decimal.Decimal
	Currency        string
	Scopes          []model.Scope
	ActiveTimeStart *model.TimeOfDay
	ActiveTimeEnd   *model.TimeOfDay
	CustomStartDate *string
	CustomEndDate   *string
}

// CreateLimitCommand handles limit creation.
//
// Persistence contract: the limit insert and the matching audit event are
// persisted atomically inside a single database transaction via executeInTx —
// either both land or neither does. Audit failure rolls the limit insert back,
// so a successful Create implies a successful audit record.
type CreateLimitCommand struct {
	repo        LimitRepository
	clock       clock.Clock
	auditWriter AuditWriter
	txBeginner  pgdb.TxBeginner

	// Streaming is the lib-streaming Emitter used to publish past-tense domain
	// events; nil disables emission and never fails the request. Set
	// post-construction at bootstrap.
	Streaming libStreaming.Emitter
}

// NewCreateLimitCommand creates a new CreateLimitCommand with dependencies.
// Returns an error if any of repo, clk, auditWriter, or txBeginner is nil to
// catch invalid dependency injection at construction time. The tx beginner is
// required because limit creation and audit recording are persisted atomically
// inside a single transaction.
func NewCreateLimitCommand(repo LimitRepository, clk clock.Clock, auditWriter AuditWriter, txBeginner pgdb.TxBeginner) (*CreateLimitCommand, error) {
	if repo == nil {
		return nil, ErrNilLimitRepository
	}

	if clk == nil {
		return nil, ErrNilClock
	}

	if auditWriter == nil {
		return nil, ErrNilCreateLimitAuditWriter
	}

	if txBeginner == nil {
		return nil, ErrNilCreateLimitTxBeginner
	}

	return &CreateLimitCommand{
		repo:        repo,
		clock:       clk,
		auditWriter: auditWriter,
		txBeginner:  txBeginner,
	}, nil
}

// Execute creates a new limit with validation, tracing, and logging.
func (c *CreateLimitCommand) Execute(ctx context.Context, input *CreateLimitInput) (_ *model.Limit, retErr error) {
	logger, tracer, _, factory := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.limit.create")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, factory, logger, "tracer", "limit_create", start, retErr)
	}()

	logger = logging.WithTrace(ctx, logger)

	// Handle nil input
	if input == nil {
		err := constant.ErrLimitNilInput
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Nil input provided", err)

		return nil, err
	}

	// Create normalized copy to avoid mutating caller's input
	normalizedInput := *input
	normalizedInput.Name = strings.TrimSpace(normalizedInput.Name)
	normalizedInput.Currency = strings.ToUpper(strings.TrimSpace(normalizedInput.Currency))

	span.SetAttributes(
		attribute.String("app.request.limit_name", normalizedInput.Name),
		attribute.String("app.request.limit_type", string(normalizedInput.LimitType)),
		attribute.String("app.request.currency", normalizedInput.Currency),
	)

	// Create domain entity via appropriate NewLimit* function
	now := c.clock.Now()

	var (
		limit *model.Limit
		err   error
	)

	// Determine which constructor to use based on provided fields
	hasTimeWindow := normalizedInput.ActiveTimeStart != nil && normalizedInput.ActiveTimeEnd != nil
	hasCustomPeriod := normalizedInput.CustomStartDate != nil && normalizedInput.CustomEndDate != nil

	hasPartialTimeWindow := (normalizedInput.ActiveTimeStart != nil) != (normalizedInput.ActiveTimeEnd != nil)
	if hasPartialTimeWindow {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Partial time window", constant.ErrLimitTimeWindowMismatch)
		return nil, constant.ErrLimitTimeWindowMismatch
	}

	hasPartialCustomPeriod := (normalizedInput.CustomStartDate != nil) != (normalizedInput.CustomEndDate != nil)
	if hasPartialCustomPeriod {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Partial custom period", constant.ErrLimitCustomDatesRequired)
		return nil, constant.ErrLimitCustomDatesRequired
	}

	if hasCustomPeriod {
		// Parse custom period dates
		customStart, parseErr := time.Parse(time.RFC3339, *normalizedInput.CustomStartDate)
		if parseErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse customStartDate", constant.ErrLimitInvalidCustomStartFormat)
			return nil, fmt.Errorf("%w: %w", constant.ErrLimitInvalidCustomStartFormat, parseErr)
		}

		customEnd, parseErr := time.Parse(time.RFC3339, *normalizedInput.CustomEndDate)
		if parseErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse customEndDate", constant.ErrLimitInvalidCustomEndFormat)
			return nil, fmt.Errorf("%w: %w", constant.ErrLimitInvalidCustomEndFormat, parseErr)
		}

		if hasTimeWindow {
			// AC-09: CUSTOM period combined with time window
			limit, err = model.NewLimitWithCustomPeriodAndTimeWindow(
				normalizedInput.Name,
				normalizedInput.LimitType,
				normalizedInput.MaxAmount,
				normalizedInput.Currency,
				normalizedInput.Scopes,
				normalizedInput.Description,
				customStart,
				customEnd,
				normalizedInput.ActiveTimeStart.String(),
				normalizedInput.ActiveTimeEnd.String(),
				now,
			)
		} else {
			limit, err = model.NewLimitWithCustomPeriod(
				normalizedInput.Name,
				normalizedInput.LimitType,
				normalizedInput.MaxAmount,
				normalizedInput.Currency,
				normalizedInput.Scopes,
				normalizedInput.Description,
				customStart,
				customEnd,
				now,
			)
		}
	} else if hasTimeWindow {
		// Create limit with time window
		limit, err = model.NewLimitWithTimeWindow(
			normalizedInput.Name,
			normalizedInput.LimitType,
			normalizedInput.MaxAmount,
			normalizedInput.Currency,
			normalizedInput.Scopes,
			normalizedInput.Description,
			normalizedInput.ActiveTimeStart.String(),
			normalizedInput.ActiveTimeEnd.String(),
			now,
		)
	} else {
		// Standard limit (no time window, no custom period)
		limit, err = model.NewLimit(
			normalizedInput.Name,
			normalizedInput.LimitType,
			normalizedInput.MaxAmount,
			normalizedInput.Currency,
			normalizedInput.Scopes,
			normalizedInput.Description,
			now,
		)
	}

	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create limit entity", err)
		logger.With(
			libLog.String("operation", "service.limit.create"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Failed to create limit entity")

		return nil, err
	}

	// Check for context cancellation before repository call
	if err := ctx.Err(); err != nil {
		libOpentelemetry.HandleSpanError(span, "Context canceled before persist", err)
		logger.With(
			libLog.String("operation", "service.limit.create"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Context canceled before persisting limit")

		return nil, err
	}

	// Persist limit insert + audit event atomically. Audit failures roll the
	// limit insert back so a successful Execute always implies a successful
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
		if createErr := c.repo.CreateWithTx(ctx, db, limit); createErr != nil {
			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to persist limit", createErr)
			logger.With(
				libLog.String("operation", "service.limit.create"),
				libLog.String("error.message", createErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to create limit")

			return fmt.Errorf("failed to persist limit: %w", createErr)
		}

		afterState := LimitToMap(limit)

		if auditErr := c.auditWriter.RecordLimitEventWithTx(
			ctx,
			db,
			model.AuditEventLimitCreated,
			model.AuditActionCreate,
			limit.ID,
			nil, // no "before" state for create
			afterState,
			"Limit created via API",
		); auditErr != nil {
			reportedInCallback = true

			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", auditErr)
			logger.With(
				libLog.String("operation", "service.limit.create"),
				libLog.String("limit.id", limit.ID.String()),
				libLog.String("error.message", auditErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to create limit")

			return fmt.Errorf("failed to record audit event: %w", auditErr)
		}

		return nil
	})
	if txErr != nil {
		if !reportedInCallback {
			libOpentelemetry.HandleSpanError(span, "Failed to create limit transactionally", txErr)
			logger.With(
				libLog.String("operation", "service.limit.create"),
				libLog.String("error.message", txErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to create limit")
		}

		return nil, fmt.Errorf("failed to create limit: %w", txErr)
	}

	return limit, nil
}
