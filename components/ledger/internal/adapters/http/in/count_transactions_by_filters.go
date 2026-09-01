// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// validTransactionStatuses contains the allowlist of valid transaction statuses for filtering.
var validTransactionStatuses = map[string]bool{
	constant.CREATED:  true,
	constant.APPROVED: true,
	constant.PENDING:  true,
	constant.CANCELED: true,
	constant.NOTED:    true,
}

// countTransactionsByFilters counts the transactions matching filter within the
// org+ledger scope.
func (handler *TransactionHandler) countTransactionsByFilters(ctx context.Context, organizationID, ledgerID uuid.UUID, filter transaction.CountFilter) (int64, error) {
	return handler.Query.CountTransactionsByFilters(ctx, organizationID, ledgerID, filter)
}

// buildCountFilter validates and assembles a CountFilter from raw query values. It
// is the sole validator of the count query filters: an out-of-allowlist status, a
// non-RFC-3339 date, or an inverted range yields a canonical business error rather
// than a native Huma 422. Missing dates default to today's UTC day.
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
