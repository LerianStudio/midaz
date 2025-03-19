package http

import (
	"bytes"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libConstants "github.com/LerianStudio/lib-commons/commons/constants"
	libHTTP "github.com/LerianStudio/lib-commons/commons/net/http"
	libRedis "github.com/LerianStudio/lib-commons/commons/redis"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"time"
)

// QueryHeader entity from query parameter from get apis
type QueryHeader struct {
	Metadata     *bson.M
	Limit        int
	Page         int
	Cursor       string
	SortOrder    string
	StartDate    time.Time
	EndDate      time.Time
	UseMetadata  bool
	PortfolioID  string
	ToAssetCodes []string
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
		metadata     *bson.M
		portfolioID  string
		toAssetCodes []string
		startDate    time.Time
		endDate      time.Time
		cursor       string
		limit        = 10
		page         = 1
		sortOrder    = "desc"
		useMetadata  = false
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
		Metadata:     metadata,
		Limit:        limit,
		Page:         page,
		Cursor:       cursor,
		SortOrder:    sortOrder,
		StartDate:    startDate,
		EndDate:      endDate,
		UseMetadata:  useMetadata,
		PortfolioID:  portfolioID,
		ToAssetCodes: toAssetCodes,
	}

	return query, nil
}

// ValidateDates validate dates
func validateDates(startDate, endDate *time.Time) error {
	maxDateRangeMonths := libCommons.SafeInt64ToInt(libCommons.GetenvIntOrDefault("MAX_PAGINATION_MONTH_DATE_RANGE", 1))

	defaultStartDate := time.Now().AddDate(0, -maxDateRangeMonths, 0)
	defaultEndDate := time.Now()

	if !startDate.IsZero() && !endDate.IsZero() {
		if !libCommons.IsValidDate(libCommons.NormalizeDate(*startDate, nil)) || !libCommons.IsValidDate(libCommons.NormalizeDate(*endDate, nil)) {
			return pkg.ValidateBusinessError(constant.ErrInvalidDateFormat, "")
		}

		if !libCommons.IsInitialDateBeforeFinalDate(*startDate, *endDate) {
			return pkg.ValidateBusinessError(constant.ErrInvalidFinalDate, "")
		}

		if !libCommons.IsDateRangeWithinMonthLimit(*startDate, *endDate, maxDateRangeMonths) {
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

	t, err := strconv.Atoi(iTTL)
	if err != nil {
		t = libRedis.RedisTTL
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
