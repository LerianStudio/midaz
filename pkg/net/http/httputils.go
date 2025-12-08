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
	EntityName            *string
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
	var (
		metadata              *bson.M
		portfolioID           string
		operationType         string
		toAssetCodes          []string
		startDate             time.Time
		endDate               time.Time
		cursor                string
		limit                 = 10
		page                  = 1
		sortOrder             = "asc"
		useMetadata           = false
		holderID              *string
		externalID            *string
		document              *string
		accountID             *string
		ledgerID              *string
		bankingDetailsBranch  *string
		bankingDetailsAccount *string
		bankingDetailsIban    *string
		entityName            *string
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
			parsedDate, _, err := libCommons.ParseDateTime(value, false)
			if err != nil {
				return nil, pkg.ValidateBusinessError(constant.ErrInvalidDatetimeFormat, "", value)
			}

			startDate = parsedDate
		case strings.Contains(key, "end_date"):
			parsedDate, _, err := libCommons.ParseDateTime(value, true)
			if err != nil {
				return nil, pkg.ValidateBusinessError(constant.ErrInvalidDatetimeFormat, "", value)
			}

			endDate = parsedDate
		case strings.Contains(key, "portfolio_id"):
			portfolioID = value
		case strings.Contains(strings.ToLower(key), "type"):
			operationType = strings.ToUpper(value)
		case strings.Contains(key, "to"):
			toAssetCodes = strings.Split(value, ",")
		case strings.Contains(key, "holder_id"):
			holderID = &value
		case strings.Contains(key, "external_id"):
			externalID = &value
		case strings.Contains(key, "document"):
			document = &value
		case strings.Contains(key, "account_id"):
			accountID = &value
		case strings.Contains(key, "ledger_id"):
			ledgerID = &value
		case strings.Contains(key, "banking_details_branch"):
			bankingDetailsBranch = &value
		case strings.Contains(key, "banking_details_account"):
			bankingDetailsAccount = &value
		case strings.Contains(key, "banking_details_iban"):
			bankingDetailsIban = &value
		case strings.Contains(key, "entity_name"):
			entityName = &value
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
		Metadata:              metadata,
		Limit:                 limit,
		Page:                  page,
		Cursor:                cursor,
		SortOrder:             sortOrder,
		StartDate:             startDate,
		EndDate:               endDate,
		UseMetadata:           useMetadata,
		PortfolioID:           portfolioID,
		OperationType:         operationType,
		ToAssetCodes:          toAssetCodes,
		HolderID:              holderID,
		ExternalID:            externalID,
		Document:              document,
		AccountID:             accountID,
		LedgerID:              ledgerID,
		BankingDetailsBranch:  bankingDetailsBranch,
		BankingDetailsAccount: bankingDetailsAccount,
		BankingDetailsIban:    bankingDetailsIban,
		EntityName:            entityName,
	}

	return query, nil
}

// validateDates validates and normalizes start/end date range for pagination queries.
// Mutates the provided pointers to apply defaults when both dates are zero.
// Default range: last N months (via MAX_PAGINATION_MONTH_DATE_RANGE env var, default=1).
// Set MAX_PAGINATION_MONTH_DATE_RANGE=0 for unlimited range (since epoch).
// Enforces all-or-nothing: both dates required if any provided.
// Returns error if dates are invalid, out of order, or only one is provided.
func validateDates(startDate, endDate *time.Time) error {
	// Limits query range to prevent expensive DB operations on large datasets
	maxDateRangeMonths := libCommons.SafeInt64ToInt(libCommons.GetenvIntOrDefault("MAX_PAGINATION_MONTH_DATE_RANGE", 1))

	if startDate.IsZero() && endDate.IsZero() {
		now := time.Now()

		defaultStartDate := time.Unix(0, 0).UTC()
		
		if maxDateRangeMonths != 0 {
			defaultStartDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, -maxDateRangeMonths, 0)
		}

		*startDate = defaultStartDate
		*endDate = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, time.UTC)

		return nil
	}

	if (!startDate.IsZero() && endDate.IsZero()) ||
		(startDate.IsZero() && !endDate.IsZero()) {
		return pkg.ValidateBusinessError(constant.ErrInvalidDateRange, "")
	}

	if !libCommons.IsValidDateTime(libCommons.NormalizeDateTime(*startDate, nil, false)) || !libCommons.IsValidDateTime(libCommons.NormalizeDateTime(*endDate, nil, true)) {
		return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, "")
	}

	if !libCommons.IsInitialDateBeforeFinalDate(*startDate, *endDate) {
		return pkg.ValidateBusinessError(constant.ErrInvalidFinalDate, "")
	}

	return nil
}

// ValidatePagination validate pagination parameters
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

// GetIdempotencyKeyAndTTL returns idempotency key and ttl if pass through.
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

// GetFileFromHeader method that get file from header and give a string fom this dsl gold file
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

func validateMetadataValueWithDepth(value any, depth int) (any, error) {
	const maxDepth = 10
	if depth > maxDepth {
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidMetadataNesting, "")
	}

	switch v := value.(type) {
	case string:
		if len(v) > 2000 {
			return nil, pkg.ValidateBusinessError(constant.ErrMetadataValueLengthExceeded, "")
		}
		return v, nil
	case float64, int, int64, float32, bool:
		return v, nil
	case nil:
		return nil, nil
	case map[string]any:
		return nil, pkg.ValidateBusinessError(constant.ErrInvalidMetadataNesting, "")
	case []any:
		validatedArray := make([]any, 0, len(v))
		for _, item := range v {
			validItem, err := validateMetadataValueWithDepth(item, depth+1)
			if err != nil {
				return nil, err
			}
			validatedArray = append(validatedArray, validItem)
		}
		return validatedArray, nil
	default:
		return nil, pkg.ValidateBusinessError(constant.ErrBadRequest, "")
	}
}
