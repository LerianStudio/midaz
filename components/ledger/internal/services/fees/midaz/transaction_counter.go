// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package midaz

import (
	"context"
	"errors"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"
	"go.opentelemetry.io/otel/attribute"
)

// TransactionCounter counts transactions by route via the Midaz transaction service.
type TransactionCounter interface {
	CountByRoute(ctx context.Context, params http.CountParams) (int64, error)
}

// ErrNilMidazClient is returned when a nil MidazClient is provided to NewTransactionCounter.
var ErrNilMidazClient = errors.New("MidazClient is required")

// midazTransactionCounter implements TransactionCounter by delegating to MidazClient.
type midazTransactionCounter struct {
	client http.MidazClient
}

// NewTransactionCounter creates a new TransactionCounter backed by the given MidazClient.
func NewTransactionCounter(client http.MidazClient) (TransactionCounter, error) {
	if client == nil {
		return nil, ErrNilMidazClient
	}

	return &midazTransactionCounter{client: client}, nil
}

// CountByRoute counts transactions matching the given route, status, and date range.
func (tc *midazTransactionCounter) CountByRoute(ctx context.Context, params http.CountParams) (int64, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "billing.count_transactions")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.organization_id", params.OrganizationID.String()),
		attribute.String("app.request.ledger_id", params.LedgerID.String()),
		attribute.String("app.request.route", params.Route),
		attribute.String("app.request.status", params.Status),
	)

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Counting transactions by route: org=%s, ledger=%s, route=%s, status=%s",
		params.OrganizationID.String(), params.LedgerID.String(),
		params.Route, params.Status))

	count, err := tc.client.CountTransactionsByRoute(ctx, params)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to count transactions by route", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error counting transactions by route: %v", err))

		return 0, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Transaction count result: count=%d, route=%s", count, params.Route))

	return count, nil
}
