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
	"go.opentelemetry.io/otel/attribute"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// DeleteLimitCommand handles limit deletion (soft-delete).
type DeleteLimitCommand struct {
	repo        LimitRepository
	clock       clock.Clock
	auditWriter AuditWriter
	txBeginner  pgdb.TxBeginner
}

// NewDeleteLimitCommand creates a new DeleteLimitCommand with dependencies.
// Returns an error if repo or clk is nil to catch invalid dependency injection at construction time.
// txBeginner may be nil — in that case Execute will surface pgdb.ErrNilConnection when
// it attempts to start the persistence transaction.
func NewDeleteLimitCommand(repo LimitRepository, clk clock.Clock, auditWriter AuditWriter, txBeginner pgdb.TxBeginner) (*DeleteLimitCommand, error) {
	if repo == nil {
		return nil, ErrNilLimitRepository
	}

	if clk == nil {
		return nil, ErrNilClock
	}

	return &DeleteLimitCommand{
		repo:        repo,
		clock:       clk,
		auditWriter: auditWriter,
		txBeginner:  txBeginner,
	}, nil
}

// Execute soft-deletes a limit by setting status to DELETED.
// DELETED is a terminal state - the limit cannot be reactivated.
// Idempotent: if already DELETED, returns success without error.
// Returns error if limit is not found.
//
// The status update and the audit event are persisted atomically inside a single
// database transaction via executeInTx: either both land or neither does.
func (c *DeleteLimitCommand) Execute(ctx context.Context, id uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.limit.delete")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Check context cancellation first to avoid unnecessary work
	if ctx.Err() != nil {
		libOpentelemetry.HandleSpanError(span, "Context cancelled", ctx.Err())
		logger.With(
			libLog.String("operation", "service.limit.delete"),
		).Log(ctx, libLog.LevelWarn, "Context cancelled")

		return ctx.Err()
	}

	// Validate input
	if id == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid input: nil UUID", constant.ErrLimitInvalidID)
		logger.With(
			libLog.String("operation", "service.limit.delete"),
		).Log(ctx, libLog.LevelWarn, "Invalid input: nil UUID")

		return constant.ErrLimitInvalidID
	}

	span.SetAttributes(
		attribute.String("app.request.limit_id", id.String()),
		attribute.String("app.request.operation", "delete"),
	)

	logger.With(
		libLog.String("operation", "service.limit.delete"),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Deleting limit")

	limit, err := c.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, constant.ErrLimitNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", err)
			logger.With(
				libLog.String("operation", "service.limit.delete"),
				libLog.String("limit.id", id.String()),
			).Log(ctx, libLog.LevelWarn, "Limit not found")

			return err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get limit from repository", err)
		logger.With(
			libLog.String("operation", "service.limit.delete"),
			libLog.String("limit.id", id.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to get limit")

		return fmt.Errorf("failed to get limit: %w", err)
	}

	// Defensive check: treat nil limit as not found (guards against repo returning nil, nil)
	if limit == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", constant.ErrLimitNotFound)
		logger.With(
			libLog.String("operation", "service.limit.delete"),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelWarn, "Limit not found")

		return constant.ErrLimitNotFound
	}

	// Idempotency: if already deleted, return success (no-op)
	if limit.Status == model.LimitStatusDeleted {
		logger.With(
			libLog.String("operation", "service.limit.delete"),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelInfo, "Limit already deleted (idempotent no-op)")

		return nil
	}

	// Capture "before" state for audit (after idempotency check, before mutation)
	beforeState := LimitToMap(limit)

	// Capture original status before mutation for accurate logging
	originalStatus := limit.Status

	// Validate transition via model.Limit.SetStatus which enforces allowed transitions:
	// ACTIVE → DELETED and INACTIVE → DELETED are valid; DELETED → DELETED is handled
	// above as idempotent. See model.LimitStatus and model.Limit.SetStatus for rules.
	if err := limit.SetStatus(model.LimitStatusDeleted, c.clock.Now()); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid state transition", err)
		logger.With(
			libLog.String("operation", "service.limit.delete"),
			libLog.String("limit.id", id.String()),
			libLog.String("limit.status_from", string(originalStatus)),
			libLog.String("limit.status_to", "DELETED"),
		).Log(ctx, libLog.LevelWarn, "Invalid transition")

		return libCommons.ValidateBusinessError(constant.ErrLimitInvalidStatusChange, err.Error())
	}

	// Persist status change + audit event atomically.
	txErr := executeInTx(ctx, c.txBeginner, func(db pgdb.DB) error {
		// Use limit.UpdatedAt from SetStatus() for timestamp consistency
		if err := c.repo.UpdateStatusWithTx(ctx, db, id, model.LimitStatusDeleted, limit.UpdatedAt); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to update limit status", err)
			logger.With(
				libLog.String("operation", "service.limit.delete"),
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
			model.AuditEventLimitDeleted,
			model.AuditActionDelete,
			limit.ID,
			beforeState,
			nil, // no "after" state for delete
			"Limit deleted via API",
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", err)
			logger.With(
				libLog.String("operation", "service.limit.delete.audit"),
				libLog.String("limit.id", id.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to record audit event")

			return fmt.Errorf("failed to record audit event: %w", err)
		}

		return nil
	})
	if txErr != nil {
		return fmt.Errorf("failed to delete limit: %w", txErr)
	}

	logger.With(
		libLog.String("operation", "service.limit.delete"),
		libLog.String("limit.id", id.String()),
		libLog.String("limit.status", string(limit.Status)),
	).Log(ctx, libLog.LevelInfo, "Limit deleted successfully")

	return nil
}
