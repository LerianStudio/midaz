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

	pgdb "tracer/internal/adapters/postgres/db"
	"tracer/pkg/clock"
	"tracer/pkg/constant"
	"tracer/pkg/logging"
	"tracer/pkg/model"
)

// DeactivateLimitCommand handles limit deactivation (ACTIVE → INACTIVE).
type DeactivateLimitCommand struct {
	repo        LimitRepository
	clock       clock.Clock
	auditWriter AuditWriter
	txBeginner  pgdb.TxBeginner
}

// NewDeactivateLimitCommand creates a new DeactivateLimitCommand with dependencies.
// Returns an error if repo or clk is nil to catch invalid dependency injection at construction time.
// txBeginner may be nil — in that case Execute will surface pgdb.ErrNilConnection when
// it attempts to start the persistence transaction.
func NewDeactivateLimitCommand(repo LimitRepository, clk clock.Clock, auditWriter AuditWriter, txBeginner pgdb.TxBeginner) (*DeactivateLimitCommand, error) {
	if repo == nil {
		return nil, ErrNilLimitRepository
	}

	if clk == nil {
		return nil, ErrNilClock
	}

	return &DeactivateLimitCommand{
		repo:        repo,
		clock:       clk,
		auditWriter: auditWriter,
		txBeginner:  txBeginner,
	}, nil
}

// Execute deactivates an active limit.
// Idempotent: if already INACTIVE, returns the limit without error.
// Returns error if limit is not found or transition is invalid (e.g., from DELETED).
//
// The status update and the audit event are persisted atomically inside a single
// database transaction via executeInTx: either both land or neither does.
func (c *DeactivateLimitCommand) Execute(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.limit.deactivate")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Check context cancellation first
	if ctx.Err() != nil {
		libOpentelemetry.HandleSpanError(span, "Context cancelled", ctx.Err())
		logger.With(
			libLog.String("operation", "service.limit.deactivate"),
		).Log(ctx, libLog.LevelWarn, "Context cancelled")

		return nil, ctx.Err()
	}

	// Validate input
	if id == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid input: nil UUID", constant.ErrLimitInvalidID)
		logger.With(
			libLog.String("operation", "service.limit.deactivate"),
		).Log(ctx, libLog.LevelWarn, "Invalid input: nil UUID")

		return nil, constant.ErrLimitInvalidID
	}

	if err := libOpentelemetry.SetSpanAttributesFromValue(span, "deactivate_input", map[string]any{
		"limit_id":  id.String(),
		"operation": "deactivate",
	}, nil); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set span attributes", err)
	}

	logger.With(
		libLog.String("operation", "service.limit.deactivate"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Deactivating limit")

	limit, err := c.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, constant.ErrLimitNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", err)
			logger.With(
				libLog.String("operation", "service.limit.deactivate"),
				libLog.String("limit.id", id.String()),
			).Log(ctx, libLog.LevelWarn, "Limit not found")

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get limit from repository", err)
		logger.With(
			libLog.String("operation", "service.limit.deactivate"),
			libLog.String("limit.id", id.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to get limit")

		return nil, fmt.Errorf("failed to get limit: %w", err)
	}

	// Defensive check: treat nil limit as not found (guards against repo returning nil, nil)
	if limit == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", constant.ErrLimitNotFound)
		logger.With(
			libLog.String("operation", "service.limit.deactivate"),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelWarn, "Limit not found")

		return nil, constant.ErrLimitNotFound
	}

	// Idempotency: if already inactive, return the limit (no-op)
	if limit.Status == model.LimitStatusInactive {
		logger.With(
			libLog.String("operation", "service.limit.deactivate"),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelInfo, "Limit already inactive (idempotent no-op)")

		return limit, nil
	}

	// Capture "before" state for audit (after idempotency check, before mutation)
	beforeState := LimitToMap(limit)

	// Capture original status before mutation for accurate logging
	originalStatus := limit.Status

	// Use domain model's SetStatus for transition validation
	if err := limit.SetStatus(model.LimitStatusInactive, c.clock.Now()); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid state transition", err)
		logger.With(
			libLog.String("operation", "service.limit.deactivate"),
			libLog.String("limit.id", id.String()),
			libLog.String("limit.status_from", string(originalStatus)),
			libLog.String("limit.status_to", "INACTIVE"),
		).Log(ctx, libLog.LevelWarn, "Invalid transition")

		return nil, libCommons.ValidateBusinessError(constant.ErrLimitInvalidStatusChange, err.Error())
	}

	afterState := LimitToMap(limit)

	// Persist status change + audit event atomically.
	txErr := executeInTx(ctx, c.txBeginner, func(db pgdb.DB) error {
		if err := c.repo.UpdateStatusWithTx(ctx, db, id, model.LimitStatusInactive, limit.UpdatedAt); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to update limit status", err)
			logger.With(
				libLog.String("operation", "service.limit.deactivate"),
				libLog.String("limit.id", id.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to update limit status")

			return fmt.Errorf("failed to update limit status: %w", err)
		}

		if c.auditWriter == nil {
			return nil
		}

		if err := c.auditWriter.RecordLimitEventWithTx(
			ctx,
			db,
			model.AuditEventLimitDeactivated,
			model.AuditActionDeactivate,
			limit.ID,
			beforeState,
			afterState,
			"Limit deactivated via API",
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", err)
			logger.With(
				libLog.String("operation", "service.limit.deactivate.audit"),
				libLog.String("limit.id", limit.ID.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to record audit event")

			return fmt.Errorf("failed to record audit event: %w", err)
		}

		return nil
	})
	if txErr != nil {
		return nil, fmt.Errorf("failed to deactivate limit: %w", txErr)
	}

	logger.With(
		libLog.String("operation", "service.limit.deactivate"),
		libLog.String("limit.id", id.String()),
		libLog.String("limit.status", string(limit.Status)),
	).Log(ctx, libLog.LevelInfo, "Limit deactivated successfully")

	return limit, nil
}
