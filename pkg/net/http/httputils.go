// Package http provides HTTP utilities and helpers for the Midaz ledger system.
// This file contains HTTP utility functions for query parameter parsing, pagination,
// date validation, and file handling.
package http

import (
	"bytes"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// QueryHeader represents parsed and validated query parameters from GET API requests.
//
// This struct encapsulates all common query parameters used across list/search endpoints,
// including pagination, filtering, sorting, and date range parameters.
type QueryHeader struct {
	Metadata      *bson.M   // MongoDB metadata filter (for metadata.key queries)
	Limit         int       // Maximum number of items per page (default: 10, max: 100)
	Page          int       // Current page number (1-indexed, default: 1)
	Cursor        string    // Cursor for cursor-based pagination
	SortOrder     string    // Sort order: "asc" or "desc" (default: "asc")
	StartDate     time.Time // Start date for date range filtering
	EndDate       time.Time // End date for date range filtering
	UseMetadata   bool      // Whether metadata filtering is being used
	PortfolioID   string    // Portfolio ID filter (UUID format)
	OperationType string    // Operation type filter (e.g., "DEBIT", "CREDIT")
	ToAssetCodes  []string  // Asset code filters (comma-separated)
}

// Pagination represents pagination parameters for list queries.
//
// This struct contains the core pagination fields used for both offset-based
// and cursor-based pagination strategies.
type Pagination struct {
	Limit     int       // Maximum number of items per page
	Page      int       // Current page number (for offset pagination)
	Cursor    string    // Cursor value (for cursor-based pagination)
	SortOrder string    // Sort order: "asc" or "desc"
	StartDate time.Time // Start date for date range filtering
	EndDate   time.Time // End date for date range filtering
}

// ValidateParameters parses and validates query parameters from HTTP requests.
//
// This function extracts query parameters from a map, applies default values, and validates
// them according to business rules. It handles:
//   - Pagination (limit, page, cursor)
//   - Sorting (sort_order)
//   - Date ranges (start_date, end_date)
//   - Metadata filtering (metadata.*)
//   - Entity filtering (portfolio_id, type, to)
//
// Default Values:
//   - limit: 10
//   - page: 1
//   - sort_order: "asc"
//   - start_date: Unix epoch (or MAX_PAGINATION_MONTH_DATE_RANGE months ago)
//   - end_date: current time
//
// Validation Rules:
//   - Limit must not exceed MAX_PAGINATION_LIMIT (default: 100)
//   - Sort order must be "asc" or "desc"
//   - Date range must not exceed MAX_PAGINATION_MONTH_DATE_RANGE months
//   - Dates must be in "yyyy-mm-dd" format
//   - Portfolio ID must be a valid UUID
//   - Cursor must be a valid base64-encoded cursor
//
// Parameters:
//   - params: Map of query parameter names to values
//
// Returns:
//   - *QueryHeader: Parsed and validated query parameters
//   - error: Validation error if parameters are invalid
//
// Example:
//
//	params := map[string]string{
//	    "limit": "50",
//	    "page": "2",
//	    "sort_order": "desc",
//	    "start_date": "2024-01-01",
//	    "end_date": "2024-12-31",
//	    "portfolio_id": "123e4567-e89b-12d3-a456-426614174000",
//	}
//	query, err := http.ValidateParameters(params)
//	if err != nil {
//	    return http.WithError(c, err)
//	}
func ValidateParameters(params map[string]string) (*QueryHeader, error) {
	var (
		metadata      *bson.M
		portfolioID   string
		operationType string
		toAssetCodes  []string
		startDate     time.Time
		endDate       time.Time
		cursor        string
		limit         = 10
		page          = 1
		sortOrder     = "asc"
		useMetadata   = false
	)

	for key, value := range params {
		switch {
		case strings.Contains(key, "metadata."):
			metadata = &bson.M{key: value}
			useMetadata = true
		case strings.Contains(key, "limit"):
			limit, _ = strconv.Atoi(value)
		case strings.Contains(key, "page"):
			page, _ = strconv.Atoi(value)
		case strings.Contains(key, "cursor"):
			cursor = value
		case strings.Contains(key, "sort_order"):
			sortOrder = strings.ToLower(value)
		case strings.Contains(key, "start_date"):
			startDate, _ = time.Parse("2006-01-02", value)
		case strings.Contains(key, "end_date"):
			endDate, _ = time.Parse("2006-01-02", value)
		case strings.Contains(key, "portfolio_id"):
			portfolioID = value
		case strings.Contains(strings.ToLower(key), "type"):
			operationType = strings.ToUpper(value)
		case strings.Contains(key, "to"):
			toAssetCodes = strings.Split(value, ",")
		}
	}

	err := validateDates(&startDate, &endDate)
	if err != nil {
		return nil, err
	}

	err = validatePagination(cursor, sortOrder, limit)
	if err != nil {
		return nil, err
	}

	if !libCommons.IsNilOrEmpty(&portfolioID) {
		_, err := uuid.Parse(portfolioID)
		if err != nil {
			return nil, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "portfolio_id")
		}
	}

	query := &QueryHeader{
		Metadata:      metadata,
		Limit:         limit,
		Page:          page,
		Cursor:        cursor,
		SortOrder:     sortOrder,
		StartDate:     startDate,
		EndDate:       endDate,
		UseMetadata:   useMetadata,
		PortfolioID:   portfolioID,
		OperationType: operationType,
		ToAssetCodes:  toAssetCodes,
	}

	return query, nil
}

// validateDates validates and normalizes start and end date parameters.
//
// This function performs comprehensive date validation for date range queries:
//   - If both dates are zero (not provided), sets defaults
//   - If only one date is provided, returns an error (both required)
//   - Validates date format (yyyy-mm-dd)
//   - Ensures start date is before end date
//   - Validates date range doesn't exceed maximum allowed months
//
// Default Behavior:
//   - If no dates provided: startDate = MAX_PAGINATION_MONTH_DATE_RANGE months ago, endDate = now
//   - If MAX_PAGINATION_MONTH_DATE_RANGE = 0: startDate = Unix epoch (1970-01-01)
//
// Parameters:
//   - startDate: Pointer to start date (will be modified if zero)
//   - endDate: Pointer to end date (will be modified if zero)
//
// Returns:
//   - error: Validation error if dates are invalid, nil if valid
//
// Possible Errors:
//   - ErrInvalidDateRange: Only one date provided (both required)
//   - ErrInvalidDateFormat: Date format is invalid
//   - ErrInvalidFinalDate: End date is before start date
//
// Example:
//
//	startDate, _ := time.Parse("2006-01-02", "2024-01-01")
//	endDate, _ := time.Parse("2006-01-02", "2024-12-31")
//	if err := validateDates(&startDate, &endDate); err != nil {
//	    return err
//	}
func validateDates(startDate, endDate *time.Time) error {
	maxDateRangeMonths := libCommons.SafeInt64ToInt(libCommons.GetenvIntOrDefault("MAX_PAGINATION_MONTH_DATE_RANGE", 1))

	if startDate.IsZero() && endDate.IsZero() {
		defaultStartDate := time.Unix(0, 0).UTC()
		if maxDateRangeMonths != 0 {
			defaultStartDate = time.Now().AddDate(0, -maxDateRangeMonths, 0)
		}

		*startDate = defaultStartDate
		*endDate = time.Now()

		return nil
	}

	if (!startDate.IsZero() && endDate.IsZero()) ||
		(startDate.IsZero() && !endDate.IsZero()) {
		return pkg.ValidateBusinessError(constant.ErrInvalidDateRange, "")
	}

	if !libCommons.IsValidDate(libCommons.NormalizeDate(*startDate, nil)) || !libCommons.IsValidDate(libCommons.NormalizeDate(*endDate, nil)) {
		return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, "")
	}

	if !libCommons.IsInitialDateBeforeFinalDate(*startDate, *endDate) {
		return pkg.ValidateBusinessError(constant.ErrInvalidFinalDate, "")
	}

	return nil
}

// validatePagination validates pagination query parameters.
//
// This function validates:
//   - Limit doesn't exceed MAX_PAGINATION_LIMIT (default: 100)
//   - Sort order is either "asc" or "desc"
//   - Cursor (if provided) is a valid base64-encoded cursor
//
// Parameters:
//   - cursor: Base64-encoded cursor for cursor-based pagination (optional)
//   - sortOrder: Sort order direction ("asc" or "desc")
//   - limit: Maximum number of items per page
//
// Returns:
//   - error: Validation error if parameters are invalid, nil if valid
//
// Possible Errors:
//   - ErrPaginationLimitExceeded: Limit exceeds maximum allowed
//   - ErrInvalidSortOrder: Sort order is not "asc" or "desc"
//   - ErrInvalidQueryParameter: Cursor is invalid
//
// Example:
//
//	if err := validatePagination("", "desc", 50); err != nil {
//	    return err
//	}
func validatePagination(cursor, sortOrder string, limit int) error {
	maxPaginationLimit := libCommons.SafeInt64ToInt(libCommons.GetenvIntOrDefault("MAX_PAGINATION_LIMIT", 100))

	if limit > maxPaginationLimit {
		return pkg.ValidateBusinessError(constant.ErrPaginationLimitExceeded, "", maxPaginationLimit)
	}

	if (sortOrder != string(constant.Asc)) && (sortOrder != string(constant.Desc)) {
		return pkg.ValidateBusinessError(constant.ErrInvalidSortOrder, "")
	}

	if !libCommons.IsNilOrEmpty(&cursor) {
		_, err := libHTTP.DecodeCursor(cursor)
		if err != nil {
			return pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "cursor")
		}
	}

	return nil
}

// GetIdempotencyKeyAndTTL extracts idempotency headers from the HTTP request.
//
// This function retrieves the idempotency key and TTL (time-to-live) from HTTP headers.
// Idempotency keys are used to prevent duplicate processing of the same request, which is
// critical for financial operations.
//
// The function reads two headers:
//   - Idempotency-Key: Unique identifier for the request
//   - Idempotency-TTL: Time-to-live in seconds for the idempotency record
//
// If TTL is not provided, invalid, or <= 0, the default TTL from libRedis.TTL is used.
//
// Parameters:
//   - c: Fiber context containing the HTTP request headers
//
// Returns:
//   - string: Idempotency key from the header (empty string if not provided)
//   - time.Duration: TTL duration in seconds (default if not provided or invalid)
//
// Example:
//
//	ikey, ttl := http.GetIdempotencyKeyAndTTL(c)
//	if ikey != "" {
//	    // Check if this request was already processed
//	    if exists := redis.Exists(ikey); exists {
//	        return http.Conflict(c, "0084", "Duplicate Request", "Request already processed")
//	    }
//	    // Store idempotency record
//	    redis.Set(ikey, result, ttl)
//	}
func GetIdempotencyKeyAndTTL(c *fiber.Ctx) (string, time.Duration) {
	ikey := c.Get(libConstants.IdempotencyKey)
	iTTL := c.Get(libConstants.IdempotencyTTL)

	// Interpret TTL as seconds count. Downstream Redis helpers multiply by time.Second.
	t, err := strconv.Atoi(iTTL)
	if err != nil || t <= 0 {
		t = libRedis.TTL
	}

	ttl := time.Duration(t)

	return ikey, ttl
}

// GetFileFromHeader extracts and validates a DSL file from a multipart form upload.
//
// This function retrieves a file from the "dsl" form field, validates it, and returns
// its contents as a string. It's specifically designed for handling Gold DSL transaction
// definition files.
//
// Validation Rules:
//   - File must be present in the form data
//   - Filename must have the correct extension (from libConstants.FileExtension)
//   - File size must be greater than 0 (not empty)
//   - File must be readable
//
// Parameters:
//   - ctx: Fiber context containing the multipart form data
//
// Returns:
//   - string: Contents of the DSL file
//   - error: Validation or read error
//
// Possible Errors:
//   - ErrInvalidDSLFileFormat: File not found, wrong extension, or read error
//   - ErrEmptyDSLFile: File size is 0
//
// Example:
//
//	dslContent, err := http.GetFileFromHeader(c)
//	if err != nil {
//	    return http.WithError(c, err)
//	}
//	// Parse DSL content
//	transaction, err := parser.Parse(dslContent)
//
// Security Note:
//   - The function panics if file.Close() fails. This is intentional to catch
//     resource leaks during development. Consider handling this more gracefully
//     in production.
func GetFileFromHeader(ctx *fiber.Ctx) (string, error) {
	fileHeader, err := ctx.FormFile(libConstants.DSL)
	if err != nil {
		return "", pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, "")
	}

	if !strings.Contains(fileHeader.Filename, libConstants.FileExtension) {
		return "", pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, "", fileHeader.Filename)
	}

	if fileHeader.Size == 0 {
		return "", pkg.ValidateBusinessError(constant.ErrEmptyDSLFile, "", fileHeader.Filename)
	}

	file, err := fileHeader.Open()
	if err != nil {
		return "", err
	}

	defer func(file multipart.File) {
		err := file.Close()
		if err != nil {
			panic(0)
		}
	}(file)

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return "", pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, "", fileHeader.Filename)
	}

	fileString := buf.String()

	return fileString, nil
}

// ToOffsetPagination converts QueryHeader to offset-based Pagination.
//
// This method extracts pagination fields suitable for offset-based pagination
// (using page numbers). The Cursor field is excluded as it's not used in offset pagination.
//
// Returns:
//   - Pagination: Pagination struct with limit, page, sort order, and date range
//
// Example:
//
//	query, _ := http.ValidateParameters(params)
//	pagination := query.ToOffsetPagination()
//	accounts, err := repository.ListWithOffset(pagination)
func (qh *QueryHeader) ToOffsetPagination() Pagination {
	return Pagination{
		Limit:     qh.Limit,
		Page:      qh.Page,
		SortOrder: qh.SortOrder,
		StartDate: qh.StartDate,
		EndDate:   qh.EndDate,
	}
}

// ToCursorPagination converts QueryHeader to cursor-based Pagination.
//
// This method extracts pagination fields suitable for cursor-based pagination.
// The Page field is excluded as it's not used in cursor-based pagination.
//
// Cursor-based pagination is more efficient for large datasets and provides
// consistent results even when data is being modified.
//
// Returns:
//   - Pagination: Pagination struct with limit, cursor, sort order, and date range
//
// Example:
//
//	query, _ := http.ValidateParameters(params)
//	pagination := query.ToCursorPagination()
//	accounts, nextCursor, err := repository.ListWithCursor(pagination)
func (qh *QueryHeader) ToCursorPagination() Pagination {
	return Pagination{
		Limit:     qh.Limit,
		Cursor:    qh.Cursor,
		SortOrder: qh.SortOrder,
		StartDate: qh.StartDate,
		EndDate:   qh.EndDate,
	}
}
