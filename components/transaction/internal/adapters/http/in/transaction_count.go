// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// CountTransactionsByRoute method that counts transactions by optional route, status, and date range.
// When no dates are provided, defaults to the current day (UTC).
//
//	@Summary		Count Transactions
//	@Description	Count transactions matching the given optional filters. Defaults to today's date range when start_date/end_date are omitted.
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			route			query		string	false	"Transaction route UUID"
//	@Param			status			query		string	false	"Transaction status (e.g., APPROVED)"
//	@Param			start_date		query		string	false	"Start date (RFC3339 format). Defaults to start of today UTC"
//	@Param			end_date		query		string	false	"End date (RFC3339 format). Defaults to end of today UTC"
//	@Success		200				{object}	query.CountResponse
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/count [get]
func (handler *TransactionHandler) CountTransactionsByRoute(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "http.transaction.count")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	route := c.Query("route")
	status := c.Query("status")
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")

	// Validate route is a valid UUID when provided
	if route != "" {
		if _, err := uuid.Parse(route); err != nil {
			validationErr := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "route")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid route UUID", validationErr)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid route UUID: %s", route))

			return http.WithError(c, validationErr)
		}
	}

	// Default to today (UTC) when dates are not provided
	now := time.Now().UTC()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, time.UTC)

	startDate := startOfDay
	endDate := endOfDay

	if startDateStr != "" {
		parsed, err := time.Parse(time.RFC3339, startDateStr)
		if err != nil {
			validationErr := pkg.ValidateBusinessError(constant.ErrInvalidDatetimeFormat, "", "start_date", "RFC3339 (e.g. 2026-01-01T00:00:00Z)")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid start_date format", validationErr)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid start_date format: %s", startDateStr))

			return http.WithError(c, validationErr)
		}

		startDate = parsed
	}

	if endDateStr != "" {
		parsed, err := time.Parse(time.RFC3339, endDateStr)
		if err != nil {
			validationErr := pkg.ValidateBusinessError(constant.ErrInvalidDatetimeFormat, "", "end_date", "RFC3339 (e.g. 2026-01-01T00:00:00Z)")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid end_date format", validationErr)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Invalid end_date format: %s", endDateStr))

			return http.WithError(c, validationErr)
		}

		endDate = parsed
	}

	// Validate start_date is before end_date
	if !libCommons.IsInitialDateBeforeFinalDate(startDate, endDate) {
		validationErr := pkg.ValidateBusinessError(constant.ErrInvalidFinalDate, "")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "start_date must be before end_date", validationErr)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("start_date (%s) is not before end_date (%s)", startDate, endDate))

		return http.WithError(c, validationErr)
	}

	filter := transaction.CountFilter{
		Route:  route,
		Status: status,
		From:   startDate,
		To:     endDate,
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Counting transactions: org=%s ledger=%s route=%s status=%s from=%s to=%s", organizationID, ledgerID, filter.Route, filter.Status, filter.From, filter.To))

	result, err := handler.Query.CountTransactionsByRoute(ctx, organizationID, ledgerID, filter)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to count transactions by route", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to count transactions by route: %v", err))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully counted transactions: totalCount=%d", result.TotalCount))

	return http.OK(c, result)
}
