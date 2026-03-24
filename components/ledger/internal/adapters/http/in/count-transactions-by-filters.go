// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
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
//
//	@Summary		Count Transactions by Filters
//	@Description	Count transactions matching optional filters (route, status, date range)
//	@Tags			Transactions
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"	format(uuid)
//	@Param			ledger_id		path		string	true	"Ledger ID"			format(uuid)
//	@Param			route			query		string	false	"Filter by transaction route"
//	@Param			status			query		string	false	"Filter by transaction status (CREATED, APPROVED, PENDING, CANCELED, NOTED)"
//	@Param			start_date		query		string	false	"Start of date range (RFC 3339, defaults to today 00:00:00 UTC)"
//	@Param			end_date		query		string	false	"End of date range (RFC 3339, defaults to today 23:59:59 UTC)"
//	@Success		204
//	@Header			204	{integer}	X-Total-Count	"Total count of matching transactions"
//	@Failure		400	{object}	mmodel.Error
//	@Failure		401	{object}	mmodel.Error
//	@Failure		403	{object}	mmodel.Error
//	@Failure		500	{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/count [head]
func (handler *TransactionHandler) CountTransactionsByFilters(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

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

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid query parameters: %v", err))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Counting transactions for organization %s and ledger %s with filters: route=%s, status=%s",
		organizationID, ledgerID, filter.Route, filter.Status,
	))

	count, err := handler.Query.CountTransactionsByFilters(ctx, organizationID, ledgerID, filter)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count transactions by filters", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error counting transactions: %v", err))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Successfully counted transactions for organization %s and ledger %s: %d",
		organizationID, ledgerID, count,
	))

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}

// parseCountFilter extracts and validates optional query parameters for the count endpoint.
func parseCountFilter(c *fiber.Ctx) (transaction.CountFilter, error) {
	var filter transaction.CountFilter

	filter.Route = strings.TrimSpace(c.Query("route"))

	status := strings.TrimSpace(c.Query("status"))
	if status != "" {
		upper := strings.ToUpper(status)
		if !validTransactionStatuses[upper] {
			return filter, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "status")
		}

		filter.Status = upper
	}

	now := time.Now().UTC()

	startDateStr := strings.TrimSpace(c.Query("start_date"))
	if startDateStr != "" {
		parsed, err := time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			return filter, pkg.ValidateBusinessError(constant.ErrInvalidDatetimeFormat, "", "start_date", "RFC 3339 (e.g. 2025-01-01T00:00:00Z)")
		}

		filter.StartDate = parsed
	} else {
		filter.StartDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}

	endDateStr := strings.TrimSpace(c.Query("end_date"))
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
