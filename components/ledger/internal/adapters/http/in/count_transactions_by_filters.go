// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// validTransactionStatuses contains the allowlist of valid transaction statuses for filtering.
var validTransactionStatuses = map[string]bool{
	constant.CREATED:  true,
	constant.APPROVED: true,
	constant.PENDING:  true,
	constant.CANCELED: true,
	constant.NOTED:    true,
}

// CountTransactionsByFilters counts transactions matching optional filters.
func (handler *TransactionHandler) CountTransactionsByFilters(c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_transactions_by_filters")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	filter, err := parseCountFilter(c)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid query parameters", err)

		return http.WithError(c, err)
	}

	count, err := handler.countTransactionsByFilters(ctx, organizationID, ledgerID, filter)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to count transactions by filters", err)

		return http.WithError(c, err)
	}

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}

// countTransactionsByFilters is the transport-agnostic count core shared by the
// Fiber wrapper above and the Huma shell (count_handler_huma.go). It carries no
// Fiber/Huma types so both transports delegate to identical query behavior.
func (handler *TransactionHandler) countTransactionsByFilters(ctx context.Context, organizationID, ledgerID uuid.UUID, filter transaction.CountFilter) (int64, error) {
	return handler.Query.CountTransactionsByFilters(ctx, organizationID, ledgerID, filter)
}

// parseCountFilter extracts optional query parameters from the Fiber context and
// delegates validation to the transport-agnostic buildCountFilter core.
func parseCountFilter(c *fiber.Ctx) (transaction.CountFilter, error) {
	return buildCountFilter(c.Query("route"), c.Query("status"), c.Query("start_date"), c.Query("end_date"))
}

// buildCountFilter validates and assembles a CountFilter from raw query values. It
// is transport-agnostic (plain strings) so the Fiber wrapper and the Huma shell
// share one validation pipeline — the sole validator of the count query filters,
// keeping both paths byte-identical (no native Huma 422).
func buildCountFilter(routeStr, statusStr, startDateStr, endDateStr string) (transaction.CountFilter, error) {
	var filter transaction.CountFilter

	filter.Route = strings.TrimSpace(routeStr)

	status := strings.TrimSpace(statusStr)
	if status != "" {
		upper := strings.ToUpper(status)
		if !validTransactionStatuses[upper] {
			return filter, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "status")
		}

		filter.Status = upper
	}

	now := time.Now().UTC()

	startDateStr = strings.TrimSpace(startDateStr)
	if startDateStr != "" {
		parsed, err := time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			return filter, pkg.ValidateBusinessError(constant.ErrInvalidDatetimeFormat, "", "start_date", "RFC 3339 (e.g. 2025-01-01T00:00:00Z)")
		}

		filter.StartDate = parsed
	} else {
		filter.StartDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}

	endDateStr = strings.TrimSpace(endDateStr)
	if endDateStr != "" {
		parsed, err := time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			return filter, pkg.ValidateBusinessError(constant.ErrInvalidDatetimeFormat, "", "end_date", "RFC 3339 (e.g. 2025-01-01T23:59:59Z)")
		}

		filter.EndDate = parsed
	} else {
		filter.EndDate = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, time.UTC)
	}

	if filter.StartDate.After(filter.EndDate) {
		return filter, pkg.ValidateBusinessError(constant.ErrInvalidFinalDate, "")
	}

	return filter, nil
}
