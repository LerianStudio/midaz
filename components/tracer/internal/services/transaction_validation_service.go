// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// Sentinel errors for TransactionValidationService.
var (
	// ErrNilGetQuery is returned when NewTransactionValidationService receives a nil getQuery.
	ErrNilGetQuery = errors.New("NewTransactionValidationService: getQuery cannot be nil")
	// ErrNilListQuery is returned when NewTransactionValidationService receives a nil listQuery.
	ErrNilListQuery = errors.New("NewTransactionValidationService: listQuery cannot be nil")
)

// TransactionValidationService is a facade that combines transaction validation queries.
// It implements the TransactionValidationService interface expected by the HTTP handler.
// NOTE: Transaction validations are immutable per SOX/GLBA requirements - only read operations.
type TransactionValidationService struct {
	getQuery  *query.GetTransactionValidationQuery
	listQuery *query.ListTransactionValidationsQuery
}

// NewTransactionValidationService creates a new transaction validation service facade.
// Returns an error if required dependencies are nil.
func NewTransactionValidationService(
	getQuery *query.GetTransactionValidationQuery,
	listQuery *query.ListTransactionValidationsQuery,
) (*TransactionValidationService, error) {
	if getQuery == nil {
		return nil, ErrNilGetQuery
	}

	if listQuery == nil {
		return nil, ErrNilListQuery
	}

	return &TransactionValidationService{
		getQuery:  getQuery,
		listQuery: listQuery,
	}, nil
}

// GetTransactionValidation retrieves a transaction validation record by ID.
func (s *TransactionValidationService) GetTransactionValidation(ctx context.Context, id uuid.UUID) (*model.TransactionValidation, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.transaction_validation.get")
	defer span.End()

	traceLogger := logging.WithTrace(ctx, logger)

	// Check for context cancellation/timeout before query execution
	if err := ctx.Err(); err != nil {
		spanMsg := "Context cancelled"
		if errors.Is(err, context.DeadlineExceeded) {
			spanMsg = "Context deadline exceeded"
		}

		libOtel.HandleSpanError(span, spanMsg, err)

		traceLogger.With(
			libLog.String("operation", "service.transaction_validation.get"),
			libLog.String("validation.id", id.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, spanMsg+" before query execution")

		return nil, fmt.Errorf("get transaction validation: %w", err)
	}

	result, err := s.getQuery.Execute(ctx, id)
	if err != nil {
		if errors.Is(err, constant.ErrTransactionValidationNotFound) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Transaction validation not found", err)
		} else if errors.Is(err, constant.ErrInvalidPathParameter) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Invalid path parameter", err)
		} else {
			libOtel.HandleSpanError(span, "Failed to get transaction validation", err)
		}

		traceLogger.With(
			libLog.String("operation", "service.transaction_validation.get"),
			libLog.String("validation.id", id.String()),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to get transaction validation")

		return nil, fmt.Errorf("get transaction validation: %w", err)
	}

	if result == nil {
		libOtel.HandleSpanBusinessErrorEvent(span, "Transaction validation not found", constant.ErrTransactionValidationNotFound)
		return nil, constant.ErrTransactionValidationNotFound
	}

	return result, nil
}

// ListTransactionValidations retrieves transaction validation records with filters.
func (s *TransactionValidationService) ListTransactionValidations(ctx context.Context, filters *model.TransactionValidationFilters) (*query.ListTransactionValidationsResult, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.transaction_validation.list")
	defer span.End()

	traceLogger := logging.WithTrace(ctx, logger)

	// Check for context cancellation before query execution
	if err := ctx.Err(); err != nil {
		libOtel.HandleSpanError(span, "Context cancelled", err)

		traceLogger.With(
			libLog.String("operation", "service.transaction_validation.list"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Context cancelled before query execution")

		return nil, fmt.Errorf("list transaction validations: %w", err)
	}

	result, err := s.listQuery.Execute(ctx, filters)
	if err != nil {
		if errors.Is(err, constant.ErrInvalidTransactionValidationFilters) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Invalid transaction validation filters", err)

			traceLogger.With(
				libLog.String("operation", "service.transaction_validation.list"),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelWarn, "Invalid filters provided")
		} else if errors.Is(err, constant.ErrInvalidCursor) {
			libOtel.HandleSpanBusinessErrorEvent(span, "Invalid pagination cursor", err)

			traceLogger.With(
				libLog.String("operation", "service.transaction_validation.list"),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelWarn, "Invalid cursor provided")
		} else {
			libOtel.HandleSpanError(span, "Failed to list transaction validations", err)

			traceLogger.With(
				libLog.String("operation", "service.transaction_validation.list"),
				libLog.String("error.message", err.Error()),
			).Log(ctx, libLog.LevelError, "Failed to list transaction validations")
		}

		return nil, fmt.Errorf("list transaction validations: %w", err)
	}

	return result, nil
}
