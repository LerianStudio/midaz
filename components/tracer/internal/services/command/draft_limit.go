// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"

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
)

const opDraftLimit = "service.limit.draft"

// DraftLimitCommand handles limit draft transition (INACTIVE → DRAFT).
type DraftLimitCommand struct {
	repo        LimitRepository
	clock       clock.Clock
	auditWriter AuditWriter
	txBeginner  pgdb.TxBeginner
}

// NewDraftLimitCommand creates a new DraftLimitCommand with dependencies.
// Returns an error if repo or clk is nil to catch invalid dependency injection at construction time.
// txBeginner may be nil — in that case Execute will surface pgdb.ErrNilConnection when
// it attempts to start the persistence transaction.
func NewDraftLimitCommand(repo LimitRepository, clk clock.Clock, auditWriter AuditWriter, txBeginner pgdb.TxBeginner) (*DraftLimitCommand, error) {
	if repo == nil {
		return nil, ErrNilLimitRepository
	}

	if clk == nil {
		return nil, ErrNilClock
	}

	return &DraftLimitCommand{
		repo:        repo,
		clock:       clk,
		auditWriter: auditWriter,
		txBeginner:  txBeginner,
	}, nil
}

// Execute transitions a limit to DRAFT status.
// Idempotent: if already DRAFT, returns the limit without error.
// Returns error if limit is not found or transition is invalid (e.g., from ACTIVE or DELETED).
//
// The status update and the audit event are persisted atomically inside a single
// database transaction via executeInTx: either both land or neither does.
func (c *DraftLimitCommand) Execute(ctx context.Context, id uuid.UUID) (*model.Limit, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, opDraftLimit)
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Check context cancellation first
	if ctx.Err() != nil {
		libOpentelemetry.HandleSpanError(span, "Context cancelled", ctx.Err())
		logger.With(
			libLog.String("operation", opDraftLimit),
		).Log(ctx, libLog.LevelWarn, "Context cancelled")

		return nil, ctx.Err()
	}

	// Validate input
	if id == uuid.Nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid input: nil UUID", constant.ErrLimitInvalidID)
		logger.With(
			libLog.String("operation", opDraftLimit),
		).Log(ctx, libLog.LevelWarn, "Invalid input: nil UUID")

		return nil, constant.ErrLimitInvalidID
	}

	span.SetAttributes(
		attribute.String("app.request.limit_id", id.String()),
		attribute.String("app.request.operation", "draft"),
	)

	logger.With(
		libLog.String("operation", opDraftLimit),
		libLog.String("limit.id", id.String()),
	).Log(ctx, libLog.LevelInfo, "Transitioning limit to draft")

	limit, err := c.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, constant.ErrLimitNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", err)
			logger.With(
				libLog.String("operation", opDraftLimit),
				libLog.String("limit.id", id.String()),
			).Log(ctx, libLog.LevelWarn, "Limit not found")

			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get limit from repository", err)
		logger.With(
			libLog.String("operation", opDraftLimit),
			libLog.String("limit.id", id.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to get limit")

		return nil, fmt.Errorf("failed to get limit: %w", err)
	}

	// Defensive check: treat nil limit as not found (guards against repo returning nil, nil)
	if limit == nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Limit not found", constant.ErrLimitNotFound)
		logger.With(
			libLog.String("operation", opDraftLimit),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelWarn, "Limit not found")

		return nil, constant.ErrLimitNotFound
	}

	// Idempotency: if already draft, return the limit (no-op)
	if limit.Status == model.LimitStatusDraft {
		logger.With(
			libLog.String("operation", opDraftLimit),
			libLog.String("limit.id", id.String()),
		).Log(ctx, libLog.LevelInfo, "Limit already in draft (idempotent no-op)")

		return limit, nil
	}

	// Capture "before" state for audit (after idempotency check to avoid unnecessary work)
	beforeState := LimitToMap(limit)

	// Capture original status before mutation for accurate logging
	originalStatus := limit.Status

	// Use domain model's SetStatus for transition validation
	if err := limit.SetStatus(model.LimitStatusDraft, c.clock.Now()); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid state transition", err)
		logger.With(
			libLog.String("operation", opDraftLimit),
			libLog.String("limit.id", id.String()),
			libLog.String("limit.status_from", string(originalStatus)),
			libLog.String("limit.status_to", "DRAFT"),
		).Log(ctx, libLog.LevelWarn, "Invalid transition")

		return nil, pkg.ValidateBusinessError(constant.ErrLimitInvalidStatusChange, constant.EntityLimit)
	}

	afterState := LimitToMap(limit)

	// Persist status change + audit event atomically.
	txErr := executeInTx(ctx, c.txBeginner, func(db pgdb.DB) error {
		if err := c.repo.UpdateStatusWithTx(ctx, db, id, model.LimitStatusDraft, limit.UpdatedAt); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to update limit status", err)
			logger.With(
				libLog.String("operation", opDraftLimit),
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
			model.AuditEventLimitDrafted,
			model.AuditActionDraft,
			limit.ID,
			beforeState,
			afterState,
			"Limit transitioned to draft via API",
		); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to record audit event", err)
			logger.With(
				libLog.String("operation", opDraftLimit+".audit"),
				libLog.String("limit.id", limit.ID.String()),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to record audit event")

			return fmt.Errorf("failed to record audit event: %w", err)
		}

		return nil
	})
	if txErr != nil {
		return nil, fmt.Errorf("failed to transition limit to draft: %w", txErr)
	}

	logger.With(
		libLog.String("operation", opDraftLimit),
		libLog.String("limit.id", id.String()),
		libLog.String("limit.status", string(limit.Status)),
	).Log(ctx, libLog.LevelInfo, "Limit transitioned to draft successfully")

	return limit, nil
}
