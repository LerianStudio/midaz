// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package midaz

import (
	"context"
	"errors"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	pkg "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// CountParams holds the parameters for counting transactions by route and window.
type CountParams struct {
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
	Route          string
	Status         string
	StartDate      time.Time
	EndDate        time.Time
}

// TransactionCounter counts transactions by route via the ledger query layer.
type TransactionCounter interface {
	CountByRoute(ctx context.Context, params CountParams) (int64, error)
}

// ErrNilResolverCounter is returned when a nil MidazResolver is provided to NewTransactionCounter.
var ErrNilResolverCounter = errors.New("MidazResolver is required and cannot be nil")

// midazTransactionCounter implements TransactionCounter by delegating to the in-process MidazResolver.
type midazTransactionCounter struct {
	resolver pkg.MidazResolver
}

// NewTransactionCounter creates a new TransactionCounter backed by the given MidazResolver.
func NewTransactionCounter(resolver pkg.MidazResolver) (TransactionCounter, error) {
	if resolver == nil {
		return nil, ErrNilResolverCounter
	}

	return &midazTransactionCounter{resolver: resolver}, nil
}

// CountByRoute counts transactions matching the given route, status, and date range.
func (tc *midazTransactionCounter) CountByRoute(ctx context.Context, params CountParams) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "billing.count_transactions")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", params.OrganizationID.String()),
		attribute.String("app.request.ledger_id", params.LedgerID.String()),
		attribute.String("app.request.route", params.Route),
		attribute.String("app.request.status", params.Status),
	)

	count, err := tc.resolver.CountTransactionsByRoute(ctx, params.OrganizationID, params.LedgerID, params.Route, params.Status, params.StartDate, params.EndDate)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to count transactions by route", err)
		logger.Log(ctx, libLog.LevelError, "Error counting transactions by route", libLog.Err(err))

		return 0, err
	}

	span.SetAttributes(attribute.Int64("db.rows_returned", count))

	return count, nil
}
