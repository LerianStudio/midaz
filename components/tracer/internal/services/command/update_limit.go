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

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// UpdateLimitInput defines input for updating a limit.
// Only non-nil fields are updated.
//
// ARCHITECTURE NOTE: This struct intentionally mirrors in.UpdateLimitInput from the HTTP layer.
// The HTTP struct contains validation tags (validate:"...") for request validation,
// while this command struct is a pure DTO without framework dependencies.
// The HTTP layer converts its struct to this one via in.ToUpdateLimitServiceInput()
// before calling command handlers. This separation ensures:
//   - HTTP adapter owns request validation (tags, format checks)
//   - Command layer remains framework-agnostic and testable
//   - Domain model (limit.Update) performs business validation
//
// Note: LimitType and Currency are immutable and cannot be updated.
type UpdateLimitInput struct {
	Name            *string          `json:"name,omitempty"`
	Description     *string          `json:"description,omitempty"`
	MaxAmount       *decimal.Decimal `json:"maxAmount,omitempty"`
	Scopes          *[]model.Scope   `json:"scopes,omitempty"`
	ActiveTimeStart *model.TimeOfDay `json:"activeTimeStart,omitempty"`
	ActiveTimeEnd   *model.TimeOfDay `json:"activeTimeEnd,omitempty"`
	CustomStartDate *string          `json:"customStartDate,omitempty"`
	CustomEndDate   *string          `json:"customEndDate,omitempty"`
}

// UpdateLimitCommand handles limit updates.
type UpdateLimitCommand struct {
	repo        LimitRepository
	clock       clock.Clock
	auditWriter AuditWriter
	txBeginner  pgdb.TxBeginner
}

// NewUpdateLimitCommand creates a new UpdateLimitCommand with dependencies.
// Returns an error if repo or clk is nil to catch invalid dependency injection at construction time.
// txBeginner may be nil — in that case Execute will surface pgdb.ErrNilConnection when
// it attempts to start the persistence transaction.
func NewUpdateLimitCommand(repo LimitRepository, clk clock.Clock, auditWriter AuditWriter, txBeginner pgdb.TxBeginner) (*UpdateLimitCommand, error) {
	if repo == nil {
		return nil, ErrNilLimitRepository
	}

	if clk == nil {
		return nil, ErrNilClock
	}

	return &UpdateLimitCommand{
		repo:        repo,
		clock:       clk,
		auditWriter: auditWriter,
		txBeginner:  txBeginner,
	}, nil
}

// Execute updates an existing limit.
// Returns constant.ErrLimitNilInput for nil input.
// Returns constant.ErrLimitInvalidID for nil UUID.
// Returns constant.ErrLimitAlreadyDeleted if attempting to update a deleted limit.
// Only fields with non-nil values in input are updated.
//
// The persistence of the update and the audit event happens atomically inside a
// single database transaction via executeInTx: either both land or neither does.
func (c *UpdateLimitCommand) Execute(ctx context.Context, id uuid.UUID, input *UpdateLimitInput) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.limit.update")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	if err := c.validateInput(span, id, input); err != nil {
		return nil, err
	}

	logger.With(
		libLog.String("operation", "service.limit.update"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Updating limit")

	normalizedInput := c.normalizeInput(input)

	if err := libOpentelemetry.SetSpanAttributesFromValue(span, "update_limit_input", map[string]any{
		"limit_id":        id.String(),
		"has_name":        normalizedInput.Name != nil,
		"has_max_amount":  normalizedInput.MaxAmount != nil,
		"has_description": normalizedInput.Description != nil,
		"has_scopes":      normalizedInput.Scopes != nil,
	}, nil); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set span attributes", err)
	}

	if ctx.Err() != nil {
		libOpentelemetry.HandleSpanError(span, "Context cancelled", ctx.Err())
		logger.With(
			libLog.String("operation", "service.limit.update"),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelWarn, "Context cancelled before fetching limit")

		return nil, ctx.Err()
	}

	limit, err := c.fetchLimit(ctx, span, logger, id)
	if err != nil {
		return nil, err
	}

	if limit.Status == model.LimitStatusDeleted {
		err := constant.ErrLimitAlreadyDeleted
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Cannot update deleted limit", err)
		logger.With(
			libLog.String("operation", "service.limit.update"),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelWarn, "Attempted to update deleted limit")

		return nil, err
	}

	if !c.hasChanges(normalizedInput) {
		logger.With(
			libLog.String("operation", "service.limit.update"),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelInfo, "No changes requested, returning existing limit")

		return limit, nil
	}

	// Capture "before" state for audit (after terminal-state guard and no-op check, before mutation)
	beforeState := LimitToMap(limit)

	// Parse custom period dates if provided
	var customStartDate, customEndDate *time.Time

	if normalizedInput.CustomStartDate != nil {
		parsed, parseErr := time.Parse(time.RFC3339, *normalizedInput.CustomStartDate)
		if parseErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse customStartDate", constant.ErrLimitInvalidCustomStartFormat)
			return nil, fmt.Errorf("%w: %w", constant.ErrLimitInvalidCustomStartFormat, parseErr)
		}

		customStartDate = &parsed
	}

	if normalizedInput.CustomEndDate != nil {
		parsed, parseErr := time.Parse(time.RFC3339, *normalizedInput.CustomEndDate)
		if parseErr != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse customEndDate", constant.ErrLimitInvalidCustomEndFormat)
			return nil, fmt.Errorf("%w: %w", constant.ErrLimitInvalidCustomEndFormat, parseErr)
		}

		customEndDate = &parsed
	}

	if err := limit.Update(
		normalizedInput.Name,
		normalizedInput.MaxAmount,
		normalizedInput.Description,
		normalizedInput.Scopes,
		normalizedInput.ActiveTimeStart,
		normalizedInput.ActiveTimeEnd,
		customStartDate,
		customEndDate,
		c.clock.Now(),
	); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Update validation failed", err)
		logger.With(
			libLog.String("operation", "service.limit.update"),
			libLog.String("limit.id", id.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelWarn, "Failed to update limit entity")

		return nil, err
	}

	if ctx.Err() != nil {
		libOpentelemetry.HandleSpanError(span, "Context cancelled", ctx.Err())
		logger.With(
			libLog.String("operation", "service.limit.update"),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelWarn, "Context cancelled")

		return nil, ctx.Err()
	}

	txErr := executeInTx(ctx, c.txBeginner, func(db pgdb.DB) error {
		if err := c.repo.UpdateWithTx(ctx, db, limit); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to persist limit update", err)
			logger.With(
				libLog.String("operation", "service.limit.update"),
				libLog.String("limit.id", id.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to persist limit update")

			return fmt.Errorf("failed to persist limit update: %w", err)
		}

		if c.auditWriter == nil {
			return nil
		}

		afterState := LimitToMap(limit)
		if err := c.auditWriter.RecordLimitEventWithTx(
			ctx,
			db,
			model.AuditEventLimitUpdated,
			model.AuditActionUpdate,
			limit.ID,
			beforeState,
			afterState,
			"Limit updated via API",
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", err)
			logger.With(
				libLog.String("operation", "service.limit.update.audit"),
				libLog.String("limit.id", limit.ID.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to record audit event")

			return fmt.Errorf("failed to record audit event: %w", err)
		}

		return nil
	})
	if txErr != nil {
		return nil, fmt.Errorf("failed to update limit: %w", txErr)
	}

	logger.With(
		libLog.String("operation", "service.limit.update"),
		libLog.String("limit.id", limit.ID.String()),
		libLog.String("limit.name", limit.Name),
	).Log(ctx, libLog.LevelInfo, "Limit updated successfully")

	return limit, nil
}

func (c *UpdateLimitCommand) validateInput(span trace.Span, id uuid.UUID, input *UpdateLimitInput) error {
	if input == nil {
		err := constant.ErrLimitNilInput
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Nil input provided", err)

		return err
	}

	if id == uuid.Nil {
		err := constant.ErrLimitInvalidID
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Nil limit ID provided", err)

		return err
	}

	return nil
}

func (c *UpdateLimitCommand) normalizeInput(input *UpdateLimitInput) *UpdateLimitInput {
	normalizedInput := &UpdateLimitInput{
		MaxAmount:       input.MaxAmount,
		Scopes:          input.Scopes,
		ActiveTimeStart: input.ActiveTimeStart,
		ActiveTimeEnd:   input.ActiveTimeEnd,
		CustomStartDate: input.CustomStartDate,
		CustomEndDate:   input.CustomEndDate,
	}

	if input.Name != nil {
		trimmed := strings.TrimSpace(*input.Name)
		normalizedInput.Name = &trimmed
	}

	if input.Description != nil {
		trimmed := strings.TrimSpace(*input.Description)
		normalizedInput.Description = &trimmed
	}

	return normalizedInput
}

func (c *UpdateLimitCommand) fetchLimit(ctx context.Context, span trace.Span, logger libLog.Logger, id uuid.UUID) (*model.Limit, error) {
	limit, err := c.repo.GetByID(ctx, id)
	if err != nil {
		c.handleFetchError(ctx, span, logger, id, err)
		return nil, err
	}

	if limit == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", constant.ErrLimitNotFound)
		logger.With(
			libLog.String("operation", "service.limit.update"),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelWarn, "Limit not found")

		return nil, constant.ErrLimitNotFound
	}

	return limit, nil
}

func (c *UpdateLimitCommand) handleFetchError(ctx context.Context, span trace.Span, logger libLog.Logger, id uuid.UUID, err error) {
	if errors.Is(err, constant.ErrLimitNotFound) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", err)
		logger.With(
			libLog.String("operation", "service.limit.update"),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelWarn, "Limit not found")
	} else {
		libOpentelemetry.HandleSpanError(span, "Failed to fetch limit", err)
		logger.With(
			libLog.String("operation", "service.limit.update"),
			libLog.String("limit.id", id.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to fetch limit")
	}
}

func (c *UpdateLimitCommand) hasChanges(input *UpdateLimitInput) bool {
	return input.Name != nil ||
		input.MaxAmount != nil ||
		input.Description != nil ||
		input.Scopes != nil ||
		input.ActiveTimeStart != nil ||
		input.ActiveTimeEnd != nil ||
		input.CustomStartDate != nil ||
		input.CustomEndDate != nil
}
