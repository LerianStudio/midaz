// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"

	"tracer/pkg/constant"
	"tracer/pkg/logging"
	"tracer/pkg/model"
)

// GetTransactionValidationQuery handles retrieving a transaction validation record by ID.
type GetTransactionValidationQuery struct {
	repo TransactionValidationRepository
}

// NewGetTransactionValidationQuery creates a new GetTransactionValidationQuery instance.
func NewGetTransactionValidationQuery(repo TransactionValidationRepository) *GetTransactionValidationQuery {
	return &GetTransactionValidationQuery{
		repo: repo,
	}
}

// Execute retrieves a transaction validation record by ID.
func (q *GetTransactionValidationQuery) Execute(ctx context.Context, validationID uuid.UUID) (*model.TransactionValidation, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.transaction-validation.get")
	defer span.End()

	logger = logging.WithTrace(ctx, logger)

	// Fast-fail on nil UUID
	if validationID == uuid.Nil {
		logger.With(
			libLog.String("operation", "service.transaction-validation.get"),
			libLog.String("validation.id", validationID.String()),
		).Log(ctx, libLog.LevelError, "Invalid validation id: cannot be nil UUID")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid validation id", constant.ErrInvalidPathParameter)

		return nil, constant.ErrInvalidPathParameter
	}

	logger.With(
		libLog.String("operation", "service.transaction-validation.get"),
		libLog.String("validation.id", validationID.String()),
	).Log(ctx, libLog.LevelInfo, "Getting transaction validation record")

	validation, err := q.repo.GetByID(ctx, validationID)
	if err != nil {
		if errors.Is(err, constant.ErrTransactionValidationNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Transaction validation not found", err)
			return nil, err
		}

		libOpentelemetry.HandleSpanError(span, "Failed to get transaction validation", err)

		return nil, err
	}

	err = libOpentelemetry.SetSpanAttributesFromValue(span, "transaction_validation", validation, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set span attributes", err)
	}

	logger.With(
		libLog.String("operation", "service.transaction-validation.get"),
		libLog.String("validation.id", validation.ID.String()),
		libLog.String("validation.decision", string(validation.Decision)),
	).Log(ctx, libLog.LevelInfo, "Transaction validation record retrieved")

	return validation, nil
}
