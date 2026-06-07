// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"

	"github.com/LerianStudio/lib-commons/v5/commons"
	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/google/uuid"
)

// QueryHeader entity from query parameter from get apis
type QueryHeader struct {
	Limit            int
	Page             int
	Cursor           string
	SortOrder        string
	StartDate        time.Time
	EndDate          time.Time
	OrganizationID   uuid.UUID
	SegmentID        uuid.UUID
	LedgerID         uuid.UUID
	TransactionRoute *string
	Enable           *bool
	ToAssetCodes     []string
}

// Pagination entity from query parameter from get apis
type Pagination struct {
	Limit     int
	Page      int
	Cursor    string
	SortOrder string
	StartDate time.Time
	EndDate   time.Time
}

func ValidateParameters(params map[string]string) (*QueryHeader, error) {
	query := &QueryHeader{
		Limit:     10,
		Page:      1,
		SortOrder: "desc",
	}

	if errParseParams := parseParams(params, query); errParseParams != nil {
		return nil, errParseParams
	}

	err := validateDates(&query.StartDate, &query.EndDate)
	if err != nil {
		return nil, err
	}

	err = validatePagination(query.Cursor, query.SortOrder, query.Page, query.Limit)
	if err != nil {
		return nil, err
	}

	return query, nil
}

func parseParams(params map[string]string, query *QueryHeader) error {
	for key, value := range params {
		if err := parseParam(key, value, query); err != nil {
			return err
		}
	}

	return nil
}

func parseParam(key, value string, query *QueryHeader) error {
	keyLower := strings.ToLower(key)

	switch keyLower {
	case "limit":
		return parseLimit(value, query)
	case "page":
		return parsePage(value, query)
	case "cursor":
		query.Cursor = value
		return nil
	case "sort_order":
		query.SortOrder = strings.ToLower(value)
		return nil
	case "start_date":
		return parseStartDate(value, query)
	case "end_date":
		return parseEndDate(value, query)
	case "segmentid":
		return parseSegmentID(value, query)
	case "ledgerid":
		return parseLedgerID(value, query)
	case "transactionroute":
		query.TransactionRoute = &value
		return nil
	case "enable":
		return parseEnable(value, query)
	case "to":
		query.ToAssetCodes = strings.Split(value, ",")
		return nil
	}

	return nil
}

func parseLimit(value string, query *QueryHeader) error {
	limit, err := strconv.Atoi(value)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "limit")
	}

	if limit < 1 {
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "limit")
	}

	query.Limit = limit

	return nil
}

func parsePage(value string, query *QueryHeader) error {
	page, err := strconv.Atoi(value)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameterPage, "")
	}

	if page < 1 {
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameterPage, "")
	}

	query.Page = page

	return nil
}

func parseStartDate(value string, query *QueryHeader) error {
	startDate, err := time.Parse("2006-01-02", value)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, "")
	}

	query.StartDate = startDate

	return nil
}

func parseEndDate(value string, query *QueryHeader) error {
	endDate, err := time.Parse("2006-01-02", value)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, "")
	}

	query.EndDate = endDate

	return nil
}

func parseSegmentID(value string, query *QueryHeader) error {
	segmentID, err := uuid.Parse(value)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "segmentId")
	}

	query.SegmentID = segmentID

	return nil
}

func parseLedgerID(value string, query *QueryHeader) error {
	ledgerID, err := uuid.Parse(value)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "ledgerId")
	}

	query.LedgerID = ledgerID

	return nil
}

func parseEnable(value string, query *QueryHeader) error {
	enable, err := strconv.ParseBool(value)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "enable")
	}

	query.Enable = &enable

	return nil
}

func validateDates(startDate, endDate *time.Time) error {
	maxDateRangeMonths := commons.SafeInt64ToInt(commons.GetenvIntOrDefault("MAX_PAGINATION_MONTH_DATE_RANGE", 1))

	defaultStartDate := time.Now().AddDate(0, -maxDateRangeMonths, 0)
	defaultEndDate := time.Now()

	if !startDate.IsZero() && !endDate.IsZero() {
		if !commons.IsValidDate(commons.NormalizeDate(*startDate, nil)) || !commons.IsValidDate(commons.NormalizeDate(*endDate, nil)) {
			return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, "")
		}

		if !commons.IsInitialDateBeforeFinalDate(*startDate, *endDate) {
			return pkg.ValidateBusinessError(constant.ErrInvalidFinalDate, "")
		}

		if !commons.IsDateRangeWithinMonthLimit(*startDate, *endDate, maxDateRangeMonths) {
			return pkg.ValidateBusinessError(constant.ErrDateRangeExceedsLimit, "", maxDateRangeMonths)
		}
	}

	if startDate.IsZero() && endDate.IsZero() {
		*startDate = defaultStartDate
		*endDate = defaultEndDate
	}

	if (!startDate.IsZero() && endDate.IsZero()) ||
		(startDate.IsZero() && !endDate.IsZero()) {
		return pkg.ValidateBusinessError(constant.ErrInvalidDateRange, "")
	}

	return nil
}

func validatePagination(cursor, sortOrder string, page, limit int) error {
	maxPaginationLimit := commons.SafeInt64ToInt(commons.GetenvIntOrDefault("MAX_PAGINATION_LIMIT", 100))

	if limit > maxPaginationLimit {
		return pkg.ValidateBusinessError(constant.ErrPaginationLimitExceeded, "", maxPaginationLimit)
	}

	if (sortOrder != string(feeconstant.Asc)) && (sortOrder != string(feeconstant.Desc)) {
		return pkg.ValidateBusinessError(constant.ErrInvalidSortOrder, "")
	}

	if page < 1 {
		return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameterPage, "")
	}

	if !commons.IsNilOrEmpty(&cursor) {
		_, err := commonsHttp.DecodeCursor(cursor)
		if err != nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "cursor")
		}
	}

	return nil
}
