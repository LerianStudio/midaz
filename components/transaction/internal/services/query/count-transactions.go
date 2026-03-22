// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/google/uuid"
)

// CountResponse represents the response for a transaction count query.
//
// swagger:model CountResponse
// @Description CountResponse contains the total count of transactions matching the specified filters.
type CountResponse struct {
	// Period range for the query
	Period Period `json:"period"`

	// Route UUID used as filter (omitted when not filtered by route)
	// example: 550e8400-e29b-41d4-a716-446655440010
	Route string `json:"route,omitempty"`

	// Transaction status used as filter (omitted when not filtered by status)
	// example: APPROVED
	Status string `json:"status,omitempty"`

	// Total number of matching transactions
	// example: 773
	TotalCount int64 `json:"totalCount"`
} // @name CountResponse

// Period represents a time range.
//
// swagger:model Period
// @Description Period defines the from/to time range for a transaction count query.
type Period struct {
	// Start of the period (inclusive)
	// example: 2026-01-01T00:00:00Z
	// format: date-time
	From time.Time `json:"from" format:"date-time"`

	// End of the period (inclusive)
	// example: 2026-02-01T00:00:00Z
	// format: date-time
	To time.Time `json:"to" format:"date-time"`
} // @name Period


// CountTransactionsByRoute counts transactions for a given set of optional filters.
func (uc *UseCase) CountTransactionsByRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, filter transaction.CountFilter) (*CountResponse, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.count_transactions_by_route")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Counting transactions: route=%s status=%s from=%s to=%s", filter.Route, filter.Status, filter.From, filter.To))

	count, err := uc.TransactionRepo.CountByRoute(ctx, organizationID, ledgerID, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to count transactions by route", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error counting transactions by route: %v", err))

		return nil, err
	}

	return &CountResponse{
		Period: Period{
			From: filter.From,
			To:   filter.To,
		},
		Route:      filter.Route,
		Status:     filter.Status,
		TotalCount: count,
	}, nil
}
