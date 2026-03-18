// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// CountTransactionsByRoute method that counts transactions by route, status, and date range.
//
//	@Summary		Count Transactions by Route
//	@Description	Count transactions matching the given route, status, and date range
//	@Tags			Transactions
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			route			query		string	true	"Transaction route UUID"
//	@Param			status			query		string	true	"Transaction status (e.g., APPROVED)"
//	@Param			start_date		query		string	true	"Start date (RFC3339 format)"
//	@Param			end_date		query		string	true	"End date (RFC3339 format)"
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

	// Validate required parameters
	if route == "" {
		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "Transaction", "route")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Missing required query parameter: route", err)

		logger.Errorf("Missing required query parameter: route")

		return http.WithError(c, err)
	}

	if status == "" {
		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "Transaction", "status")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Missing required query parameter: status", err)

		logger.Errorf("Missing required query parameter: status")

		return http.WithError(c, err)
	}

	if startDateStr == "" {
		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "Transaction", "start_date")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Missing required query parameter: start_date", err)

		logger.Errorf("Missing required query parameter: start_date")

		return http.WithError(c, err)
	}

	if endDateStr == "" {
		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "Transaction", "end_date")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Missing required query parameter: end_date", err)

		logger.Errorf("Missing required query parameter: end_date")

		return http.WithError(c, err)
	}

	// Validate route is a valid UUID
	if _, err := uuid.Parse(route); err != nil {
		validationErr := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "Transaction", "route must be a valid UUID")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid route UUID", validationErr)

		logger.Errorf("Invalid route UUID: %s", route)

		return http.WithError(c, validationErr)
	}

	// Parse start_date
	startDate, err := time.Parse(time.RFC3339, startDateStr)
	if err != nil {
		validationErr := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "Transaction", "start_date must be in RFC3339 format")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid start_date format", validationErr)

		logger.Errorf("Invalid start_date format: %s", startDateStr)

		return http.WithError(c, validationErr)
	}

	// Parse end_date
	endDate, err := time.Parse(time.RFC3339, endDateStr)
	if err != nil {
		validationErr := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, "Transaction", "end_date must be in RFC3339 format")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid end_date format", validationErr)

		logger.Errorf("Invalid end_date format: %s", endDateStr)

		return http.WithError(c, validationErr)
	}

	logger.Infof("Counting transactions by route: org=%s ledger=%s route=%s status=%s", organizationID, ledgerID, route, status)

	result, err := handler.Query.CountTransactionsByRoute(ctx, organizationID, ledgerID, route, status, startDate, endDate)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count transactions by route", err)

		logger.Errorf("Failed to count transactions by route: %v", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully counted transactions: totalCount=%d", result.TotalCount)

	return http.OK(c, result)
}
