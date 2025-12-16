package http

import (
	"bytes"
	"fmt"
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

const (
	// defaultMaxPaginationLimit is the default maximum number of items per page
	defaultMaxPaginationLimit = 100
	// defaultMetadataMaxLength is the default maximum length for metadata string values
	defaultMetadataMaxLength = 2000
	// defaultPaginationLimit is the default number of items per page
	defaultPaginationLimit = 10
	// defaultPaginationPage is the default page number
	defaultPaginationPage = 1
)

// QueryHeader entity from query parameter from get apis
type QueryHeader struct {
	Metadata              *bson.M
	Limit                 int
	Page                  int
	Cursor                string
	SortOrder             string
	StartDate             time.Time
	EndDate               time.Time
	UseMetadata           bool
	PortfolioID           string
	OperationType         string
	ToAssetCodes          []string
	HolderID              *string
	ExternalID            *string
	Document              *string
	AccountID             *string
	LedgerID              *string
	BankingDetailsBranch  *string
	BankingDetailsAccount *string
	BankingDetailsIban    *string
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

// ValidateParameters validate and return struct of default parameters
func ValidateParameters(params map[string]string) (*QueryHeader, error) {
	query := initializeQueryHeader()

	err := parseQueryParameters(params, query)
	if err != nil {
		return nil, err
	}

	err = validateDates(&query.StartDate, &query.EndDate)
	if err != nil {
		return nil, err
	}

	err = validatePagination(query.Cursor, query.SortOrder, query.Limit)
	if err != nil {
		return nil, err
	}

	if !libCommons.IsNilOrEmpty(&query.PortfolioID) {
		_, err := uuid.Parse(query.PortfolioID)
		if err != nil {
			// Return typed error directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
			return nil, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, "", "portfolio_id")
		}
	}

	return query, nil
}

// initializeQueryHeader creates a new QueryHeader with default values
func initializeQueryHeader() *QueryHeader {
	return &QueryHeader{
		Limit:     defaultPaginationLimit,
		Page:      defaultPaginationPage,
		SortOrder: "asc",
	}
}

// parseQueryParameters parses all query parameters and populates the QueryHeader
func parseQueryParameters(params map[string]string, query *QueryHeader) error {
	for key, value := range params {
		if err := parseQueryParameter(key, value, query); err != nil {
			return err
		}
	}

	return nil
}

// parseQueryParameter parses a single query parameter
func parseQueryParameter(key, value string, query *QueryHeader) error {
	// Handle date parameters separately as they can return errors
	if strings.Contains(key, "start_date") {
		return parseStartDate(value, query)
	}

	if strings.Contains(key, "end_date") {
		return parseEndDate(value, query)
	}

	// Handle all other parameters that don't return errors
	parseSimpleQueryParameter(key, value, query)

	return nil
}

// parseSimpleQueryParameter handles query parameters that don't require error handling
func parseSimpleQueryParameter(key, value string, query *QueryHeader) {
	// Handle basic pagination and sorting parameters
	if parsePaginationParameter(key, value, query) {
		return
	}

	// Handle entity-specific parameters
	if parseEntityParameter(key, value, query) {
		return
	}

	// Handle banking details parameters
	parseBankingDetailsParameter(key, value, query)
}

// parsePaginationParameter parses pagination and sorting related parameters
func parsePaginationParameter(key, value string, query *QueryHeader) bool {
	switch {
	case strings.Contains(key, "metadata."):
		if query.Metadata == nil {
			query.Metadata = &bson.M{}
		}

		(*query.Metadata)[key] = value
		query.UseMetadata = true

		return true
	case strings.Contains(key, "limit"):
		query.Limit, _ = strconv.Atoi(value)
		return true
	case strings.Contains(key, "page"):
		query.Page, _ = strconv.Atoi(value)
		return true
	case strings.Contains(key, "cursor"):
		query.Cursor = value
		return true
	case strings.Contains(key, "sort_order"):
		query.SortOrder = strings.ToLower(value)
		return true
	case strings.Contains(key, "portfolio_id"):
		query.PortfolioID = value
		return true
	case strings.Contains(strings.ToLower(key), "type"):
		query.OperationType = strings.ToUpper(value)
		return true
	case strings.Contains(key, "to"):
		query.ToAssetCodes = strings.Split(value, ",")
		return true
	}

	return false
}

// parseEntityParameter parses entity-specific parameters
func parseEntityParameter(key, value string, query *QueryHeader) bool {
	switch {
	case strings.Contains(key, "holder_id"):
		query.HolderID = &value
		return true
	case strings.Contains(key, "external_id"):
		query.ExternalID = &value
		return true
	case strings.Contains(key, "document"):
		query.Document = &value
		return true
	case strings.Contains(key, "account_id"):
		query.AccountID = &value
		return true
	case strings.Contains(key, "ledger_id"):
		query.LedgerID = &value
		return true
	}

	return false
}

// parseBankingDetailsParameter parses banking details related parameters
func parseBankingDetailsParameter(key, value string, query *QueryHeader) {
	switch {
	case strings.Contains(key, "banking_details_branch"):
		query.BankingDetailsBranch = &value
	case strings.Contains(key, "banking_details_account"):
		query.BankingDetailsAccount = &value
	case strings.Contains(key, "banking_details_iban"):
		query.BankingDetailsIban = &value
	}
}

// parseStartDate parses the start_date parameter.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func parseStartDate(value string, query *QueryHeader) error {
	parsedDate, _, err := libCommons.ParseDateTime(value, false)
	if err != nil {
		// Return typed error directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
		return pkg.ValidateBusinessError(constant.ErrInvalidDatetimeFormat, "", value)
	}

	query.StartDate = parsedDate

	return nil
}

// parseEndDate parses the end_date parameter.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func parseEndDate(value string, query *QueryHeader) error {
	parsedDate, _, err := libCommons.ParseDateTime(value, true)
	if err != nil {
		// Return typed error directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
		return pkg.ValidateBusinessError(constant.ErrInvalidDatetimeFormat, "", value)
	}

	query.EndDate = parsedDate

	return nil
}

// validateDates validates and normalizes start/end date range for pagination queries.
// Mutates the provided pointers to apply defaults when both dates are zero.
// Default range: last N months (via MAX_PAGINATION_MONTH_DATE_RANGE env var, default=1).
// Set MAX_PAGINATION_MONTH_DATE_RANGE=0 for unlimited range (since epoch).
// Enforces all-or-nothing: both dates required if any provided.
// Returns error if dates are invalid, out of order, or only one is provided.
func validateDates(startDate, endDate *time.Time) error {
	if startDate.IsZero() && endDate.IsZero() {
		setDefaultDateRange(startDate, endDate)
		return nil
	}

	if err := validateBothDatesProvided(startDate, endDate); err != nil {
		return err
	}

	if err := validateDateFormats(startDate, endDate); err != nil {
		return err
	}

	if err := validateDateOrder(startDate, endDate); err != nil {
		return err
	}

	return nil
}

// setDefaultDateRange sets default start and end dates when both are zero
func setDefaultDateRange(startDate, endDate *time.Time) {
	maxDateRangeMonths := libCommons.SafeInt64ToInt(libCommons.GetenvIntOrDefault("MAX_PAGINATION_MONTH_DATE_RANGE", 1))
	now := time.Now()

	defaultStartDate := time.Unix(0, 0).UTC()

	if maxDateRangeMonths != 0 {
		defaultStartDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, -maxDateRangeMonths, 0)
	}

	*startDate = defaultStartDate
	*endDate = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, time.UTC)
}

// validateBothDatesProvided ensures both dates are provided or both are empty.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func validateBothDatesProvided(startDate, endDate *time.Time) error {
	if (!startDate.IsZero() && endDate.IsZero()) || (startDate.IsZero() && !endDate.IsZero()) {
		// Return typed error directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
		return pkg.ValidateBusinessError(constant.ErrInvalidDateRange, "")
	}

	return nil
}

// validateDateFormats validates that both dates have valid formats.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func validateDateFormats(startDate, endDate *time.Time) error {
	if !libCommons.IsValidDateTime(libCommons.NormalizeDateTime(*startDate, nil, false)) ||
		!libCommons.IsValidDateTime(libCommons.NormalizeDateTime(*endDate, nil, true)) {
		// Return typed error directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
		return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, "")
	}

	return nil
}

// validateDateOrder validates that start date is before end date.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func validateDateOrder(startDate, endDate *time.Time) error {
	if !libCommons.IsInitialDateBeforeFinalDate(*startDate, *endDate) {
		// Return typed error directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
		return pkg.ValidateBusinessError(constant.ErrInvalidFinalDate, "")
	}

	return nil
}

// validatePagination validates pagination parameters.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func validatePagination(cursor, sortOrder string, limit int) error {
	maxPaginationLimit := libCommons.SafeInt64ToInt(libCommons.GetenvIntOrDefault("MAX_PAGINATION_LIMIT", defaultMaxPaginationLimit))

	if limit > maxPaginationLimit {
		// Return typed error directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
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

// GetIdempotencyKeyAndTTL returns idempotency key and ttl if pass through.
func GetIdempotencyKeyAndTTL(c *fiber.Ctx) (string, time.Duration) {
	ikey := c.Get(libConstants.IdempotencyKey)
	iTTL := c.Get(libConstants.IdempotencyTTL)

	// Interpret TTL as seconds count and convert to time.Duration.
	t, err := strconv.Atoi(iTTL)
	if err != nil || t <= 0 {
		t = libRedis.TTL
	}

	ttl := time.Duration(t) * time.Second

	return ikey, ttl
}

// GetFileFromHeader method that get file from header and give a string fom this dsl gold file.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func GetFileFromHeader(ctx *fiber.Ctx) (string, error) {
	fileHeader, err := ctx.FormFile(libConstants.DSL)
	if err != nil {
		// Return typed error directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
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
		return "", fmt.Errorf("failed to open file: %w", err)
	}

	defer func(file multipart.File) {
		// File close errors are non-fatal - the file was already read successfully.
		// We intentionally ignore this error as it doesn't affect the operation result.
		_ = file.Close()
	}(file)

	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, file); err != nil {
		return "", pkg.ValidateBusinessError(constant.ErrInvalidDSLFileFormat, "", fileHeader.Filename)
	}

	fileString := buf.String()

	return fileString, nil
}

func (qh *QueryHeader) ToOffsetPagination() Pagination {
	return Pagination{
		Limit:     qh.Limit,
		Page:      qh.Page,
		SortOrder: qh.SortOrder,
		StartDate: qh.StartDate,
		EndDate:   qh.EndDate,
	}
}

func (qh *QueryHeader) ToCursorPagination() Pagination {
	return Pagination{
		Limit:     qh.Limit,
		Cursor:    qh.Cursor,
		SortOrder: qh.SortOrder,
		StartDate: qh.StartDate,
		EndDate:   qh.EndDate,
	}
}

func GetBooleanParam(c *fiber.Ctx, queryParamName string) bool {
	return strings.ToLower(c.Query(queryParamName, "false")) == "true"
}

// ValidateMetadataValue validates a metadata value, ensuring it meets specific criteria for type and length.
// It supports strings, numbers, booleans, nil, and arrays without nested maps or overly long strings.
func ValidateMetadataValue(value any) (any, error) {
	return validateMetadataValueWithDepth(value, 0)
}

// validateMetadataValueWithDepth validates metadata with depth tracking.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func validateMetadataValueWithDepth(value any, depth int) (any, error) {
	const maxDepth = 10
	if depth > maxDepth {
		// Return typed error directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidMetadataNesting, "")
	}

	switch v := value.(type) {
	case string:
		return validateMetadataString(v)
	case float64, int, int64, float32, bool:
		return v, nil
	case nil:
		return nil, nil
	case map[string]any:
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidMetadataNesting, "")
	case []any:
		return validateMetadataArray(v, depth)
	default:
		return nil, pkg.ValidateBusinessError(constant.ErrBadRequest, "")
	}
}

// validateMetadataString validates a metadata string value.
// Returns typed business errors directly to preserve error type for proper HTTP status code mapping.
func validateMetadataString(v string) (any, error) {
	if len(v) > defaultMetadataMaxLength {
		// Return typed error directly - do NOT wrap with fmt.Errorf to preserve error type for errors.As()
		return nil, pkg.ValidateBusinessError(constant.ErrMetadataValueLengthExceeded, "")
	}

	return v, nil
}

// validateMetadataArray validates a metadata array value
func validateMetadataArray(v []any, depth int) (any, error) {
	validatedArray := make([]any, 0, len(v))

	for _, item := range v {
		validItem, err := validateMetadataValueWithDepth(item, depth+1)
		if err != nil {
			return nil, err
		}

		validatedArray = append(validatedArray, validItem)
	}

	return validatedArray, nil
}
