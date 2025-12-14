package http

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	libConstants "github.com/LerianStudio/lib-commons/v2/commons/constants"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestValidateParameters_DefaultValues(t *testing.T) {
	params := make(map[string]string)

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 10, result.Limit)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, "asc", result.SortOrder)
	assert.Empty(t, result.Cursor)
	assert.False(t, result.UseMetadata)
	assert.Nil(t, result.Metadata)
}

func TestValidateParameters_WithLimit(t *testing.T) {
	params := map[string]string{
		"limit": "50",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Equal(t, 50, result.Limit)
}

func TestValidateParameters_WithPage(t *testing.T) {
	params := map[string]string{
		"page": "5",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Equal(t, 5, result.Page)
}

func TestValidateParameters_WithCursor(t *testing.T) {
	// Create a valid base64 cursor
	cursor := "eyJpZCI6IjEyMyJ9" // {"id":"123"} in base64

	params := map[string]string{
		"cursor": cursor,
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Equal(t, cursor, result.Cursor)
}

func TestValidateParameters_WithSortOrderDesc(t *testing.T) {
	params := map[string]string{
		"sort_order": "DESC",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Equal(t, "desc", result.SortOrder)
}

func TestValidateParameters_WithSortOrderAsc(t *testing.T) {
	params := map[string]string{
		"sort_order": "ASC",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Equal(t, "asc", result.SortOrder)
}

func TestValidateParameters_WithInvalidSortOrder(t *testing.T) {
	params := map[string]string{
		"sort_order": "invalid",
	}

	result, err := ValidateParameters(params)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateParameters_WithMetadata(t *testing.T) {
	params := map[string]string{
		"metadata.key": "value",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.True(t, result.UseMetadata)
	assert.NotNil(t, result.Metadata)
	assert.Equal(t, &bson.M{"metadata.key": "value"}, result.Metadata)
}

func TestValidateParameters_WithValidDates(t *testing.T) {
	params := map[string]string{
		"start_date": "2024-01-01",
		"end_date":   "2024-01-31",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.False(t, result.StartDate.IsZero())
	assert.False(t, result.EndDate.IsZero())
}

func TestValidateParameters_WithInvalidStartDate(t *testing.T) {
	params := map[string]string{
		"start_date": "invalid-date",
		"end_date":   "2024-01-31",
	}

	result, err := ValidateParameters(params)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateParameters_WithInvalidEndDate(t *testing.T) {
	params := map[string]string{
		"start_date": "2024-01-01",
		"end_date":   "invalid-date",
	}

	result, err := ValidateParameters(params)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateParameters_WithPortfolioID(t *testing.T) {
	validUUID := "123e4567-e89b-12d3-a456-426614174000"
	params := map[string]string{
		"portfolio_id": validUUID,
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Equal(t, validUUID, result.PortfolioID)
}

func TestValidateParameters_WithInvalidPortfolioID(t *testing.T) {
	params := map[string]string{
		"portfolio_id": "invalid-uuid",
	}

	result, err := ValidateParameters(params)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateParameters_WithOperationType(t *testing.T) {
	params := map[string]string{
		"type": "credit",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Equal(t, "CREDIT", result.OperationType)
}

func TestValidateParameters_WithToAssetCodes(t *testing.T) {
	params := map[string]string{
		"to": "USD,EUR,BRL",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Equal(t, []string{"USD", "EUR", "BRL"}, result.ToAssetCodes)
}

func TestValidateParameters_LimitExceeded(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Getenv("MAX_PAGINATION_LIMIT")
	defer os.Setenv("MAX_PAGINATION_LIMIT", originalEnv)

	os.Setenv("MAX_PAGINATION_LIMIT", "100")

	params := map[string]string{
		"limit": "150",
	}

	result, err := ValidateParameters(params)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateParameters_WithInvalidCursor(t *testing.T) {
	params := map[string]string{
		"cursor": "invalid-cursor-not-base64",
	}

	result, err := ValidateParameters(params)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateDates_BothZero(t *testing.T) {
	startDate := time.Time{}
	endDate := time.Time{}
	err := validateDates(&startDate, &endDate)
	require.NoError(t, err)
	assert.False(t, startDate.IsZero())
	assert.False(t, endDate.IsZero())

	assert.Equal(t, 0, startDate.Hour())
	assert.Equal(t, 0, startDate.Minute())
	assert.Equal(t, 0, startDate.Second())

	assert.Equal(t, 23, endDate.Hour())
	assert.Equal(t, 59, endDate.Minute())
	assert.Equal(t, 59, endDate.Second())

	assert.True(t, endDate.After(startDate) || endDate.Equal(startDate))
}

func TestValidateDates_OnlyStartDateProvided(t *testing.T) {
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Time{}

	err := validateDates(&startDate, &endDate)

	assert.Error(t, err)
}

func TestValidateDates_OnlyEndDateProvided(t *testing.T) {
	startDate := time.Time{}
	endDate := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)

	err := validateDates(&startDate, &endDate)

	assert.Error(t, err)
}

func TestValidateDates_StartDateAfterEndDate(t *testing.T) {
	startDate := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	err := validateDates(&startDate, &endDate)

	assert.Error(t, err)
}

func TestValidateDates_ValidDateRange(t *testing.T) {
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	err := validateDates(&startDate, &endDate)

	require.NoError(t, err)
}

func TestValidateDates_WithMaxDateRangeZero(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Getenv("MAX_PAGINATION_MONTH_DATE_RANGE")
	defer os.Setenv("MAX_PAGINATION_MONTH_DATE_RANGE", originalEnv)

	os.Setenv("MAX_PAGINATION_MONTH_DATE_RANGE", "0")

	startDate := time.Time{}
	endDate := time.Time{}

	err := validateDates(&startDate, &endDate)

	require.NoError(t, err)
	// When max is 0, startDate should be epoch
	assert.Equal(t, int64(0), startDate.Unix())
}

func TestValidatePagination_ValidParams(t *testing.T) {
	err := validatePagination("", "asc", 10)

	require.NoError(t, err)
}

func TestValidatePagination_ValidParamsDesc(t *testing.T) {
	err := validatePagination("", "desc", 50)

	require.NoError(t, err)
}

func TestValidatePagination_InvalidSortOrder(t *testing.T) {
	err := validatePagination("", "invalid", 10)

	assert.Error(t, err)
}

func TestValidatePagination_LimitExceeded(t *testing.T) {
	// Save original env and restore after test
	originalEnv := os.Getenv("MAX_PAGINATION_LIMIT")
	defer os.Setenv("MAX_PAGINATION_LIMIT", originalEnv)

	os.Setenv("MAX_PAGINATION_LIMIT", "100")

	err := validatePagination("", "asc", 150)

	assert.Error(t, err)
}

func TestValidatePagination_InvalidCursor(t *testing.T) {
	err := validatePagination("invalid-cursor", "asc", 10)

	assert.Error(t, err)
}

func TestValidatePagination_ValidCursor(t *testing.T) {
	// Valid base64 encoded cursor
	cursor := "eyJpZCI6IjEyMyJ9"

	err := validatePagination(cursor, "asc", 10)

	require.NoError(t, err)
}

func TestGetIdempotencyKeyAndTTL_WithValidValues(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		key, ttl := GetIdempotencyKeyAndTTL(c)
		assert.Equal(t, "test-key", key)
		assert.Equal(t, 60*time.Second, ttl)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(libConstants.IdempotencyKey, "test-key")
	req.Header.Set(libConstants.IdempotencyTTL, "60")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestGetIdempotencyKeyAndTTL_WithInvalidTTL(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		key, ttl := GetIdempotencyKeyAndTTL(c)
		assert.Equal(t, "test-key", key)
		// Default TTL when invalid
		assert.True(t, ttl > 0)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(libConstants.IdempotencyKey, "test-key")
	req.Header.Set(libConstants.IdempotencyTTL, "invalid")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestGetIdempotencyKeyAndTTL_WithNegativeTTL(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		key, ttl := GetIdempotencyKeyAndTTL(c)
		assert.Equal(t, "test-key", key)
		// Default TTL when negative
		assert.True(t, ttl > 0)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(libConstants.IdempotencyKey, "test-key")
	req.Header.Set(libConstants.IdempotencyTTL, "-1")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestGetIdempotencyKeyAndTTL_WithEmptyHeaders(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		key, ttl := GetIdempotencyKeyAndTTL(c)
		assert.Empty(t, key)
		assert.True(t, ttl > 0)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestGetFileFromHeader_NoFile(t *testing.T) {
	app := fiber.New()

	app.Post("/upload", func(c *fiber.Ctx) error {
		_, err := GetFileFromHeader(c)
		assert.Error(t, err)
		return c.SendStatus(fiber.StatusBadRequest)
	})

	req := httptest.NewRequest("POST", "/upload", nil)
	req.Header.Set("Content-Type", "multipart/form-data")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestGetFileFromHeader_InvalidExtension(t *testing.T) {
	app := fiber.New()

	app.Post("/upload", func(c *fiber.Ctx) error {
		_, err := GetFileFromHeader(c)
		assert.Error(t, err)
		return c.SendStatus(fiber.StatusBadRequest)
	})

	// Create multipart form with invalid file extension
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile(libConstants.DSL, "test.txt")
	_, _ = io.WriteString(part, "file content")
	writer.Close()

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestGetFileFromHeader_EmptyFile(t *testing.T) {
	app := fiber.New()

	app.Post("/upload", func(c *fiber.Ctx) error {
		_, err := GetFileFromHeader(c)
		assert.Error(t, err)
		return c.SendStatus(fiber.StatusBadRequest)
	})

	// Create multipart form with empty file
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile(libConstants.DSL, "test"+libConstants.FileExtension)
	_, _ = io.WriteString(part, "")
	writer.Close()

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestGetFileFromHeader_ValidFile(t *testing.T) {
	app := fiber.New()

	expectedContent := "valid file content"

	app.Post("/upload", func(c *fiber.Ctx) error {
		content, err := GetFileFromHeader(c)
		if err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		assert.Equal(t, expectedContent, content)
		return c.SendStatus(fiber.StatusOK)
	})

	// Create multipart form with valid file
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile(libConstants.DSL, "test"+libConstants.FileExtension)
	_, _ = io.WriteString(part, expectedContent)
	writer.Close()

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestQueryHeader_ToOffsetPagination(t *testing.T) {
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	qh := &QueryHeader{
		Limit:         20,
		Page:          3,
		Cursor:        "some-cursor",
		SortOrder:     "desc",
		StartDate:     startDate,
		EndDate:       endDate,
		UseMetadata:   true,
		PortfolioID:   "portfolio-123",
		OperationType: "CREDIT",
		ToAssetCodes:  []string{"USD", "EUR"},
	}

	pagination := qh.ToOffsetPagination()

	assert.Equal(t, 20, pagination.Limit)
	assert.Equal(t, 3, pagination.Page)
	assert.Equal(t, "desc", pagination.SortOrder)
	assert.Equal(t, startDate, pagination.StartDate)
	assert.Equal(t, endDate, pagination.EndDate)
	// Cursor should be empty for offset pagination
	assert.Empty(t, pagination.Cursor)
}

func TestQueryHeader_ToCursorPagination(t *testing.T) {
	startDate := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 31, 23, 59, 59, 0, time.UTC)

	qh := &QueryHeader{
		Limit:         20,
		Page:          3,
		Cursor:        "some-cursor",
		SortOrder:     "desc",
		StartDate:     startDate,
		EndDate:       endDate,
		UseMetadata:   true,
		PortfolioID:   "portfolio-123",
		OperationType: "CREDIT",
		ToAssetCodes:  []string{"USD", "EUR"},
	}

	pagination := qh.ToCursorPagination()

	assert.Equal(t, 20, pagination.Limit)
	assert.Equal(t, "some-cursor", pagination.Cursor)
	assert.Equal(t, "desc", pagination.SortOrder)
	assert.Equal(t, startDate, pagination.StartDate)
	assert.Equal(t, endDate, pagination.EndDate)
	// Page should be empty for cursor pagination
	assert.Equal(t, 0, pagination.Page)
}

func TestValidateParameters_AllParams(t *testing.T) {
	params := map[string]string{
		"limit":        "25",
		"page":         "2",
		"sort_order":   "desc",
		"start_date":   "2024-01-01",
		"end_date":     "2024-01-31",
		"portfolio_id": "123e4567-e89b-12d3-a456-426614174000",
		"type":         "debit",
		"to":           "USD,BRL",
		"metadata.key": "value",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Equal(t, 25, result.Limit)
	assert.Equal(t, 2, result.Page)
	assert.Equal(t, "desc", result.SortOrder)
	assert.Equal(t, "123e4567-e89b-12d3-a456-426614174000", result.PortfolioID)
	assert.Equal(t, "DEBIT", result.OperationType)
	assert.Equal(t, []string{"USD", "BRL"}, result.ToAssetCodes)
	assert.True(t, result.UseMetadata)
	assert.NotNil(t, result.Metadata)
}

func TestValidateDates_SameDay(t *testing.T) {
	startDate := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2024, 1, 15, 23, 59, 59, 0, time.UTC)

	err := validateDates(&startDate, &endDate)

	require.NoError(t, err)
}

func TestGetIdempotencyKeyAndTTL_WithZeroTTL(t *testing.T) {
	app := fiber.New()

	expectedDefaultTTL := time.Duration(libRedis.TTL) * time.Second

	app.Get("/test", func(c *fiber.Ctx) error {
		key, ttl := GetIdempotencyKeyAndTTL(c)
		assert.Equal(t, "test-key", key)
		// Zero TTL should fall back to default
		assert.Equal(t, expectedDefaultTTL, ttl, "zero TTL should fall back to libRedis.TTL * time.Second")
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(libConstants.IdempotencyKey, "test-key")
	req.Header.Set(libConstants.IdempotencyTTL, "0")

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestGetIdempotencyKeyAndTTL_WithMissingTTLHeader(t *testing.T) {
	app := fiber.New()

	expectedDefaultTTL := time.Duration(libRedis.TTL) * time.Second

	app.Get("/test", func(c *fiber.Ctx) error {
		key, ttl := GetIdempotencyKeyAndTTL(c)
		assert.Equal(t, "test-key", key)
		// Missing TTL header should fall back to default
		assert.Equal(t, expectedDefaultTTL, ttl, "missing TTL header should fall back to libRedis.TTL * time.Second")
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(libConstants.IdempotencyKey, "test-key")
	// Intentionally NOT setting IdempotencyTTL header

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestGetIdempotencyKeyAndTTL_DefaultValueIsCorrect(t *testing.T) {
	app := fiber.New()

	expectedDefaultTTL := time.Duration(libRedis.TTL) * time.Second

	app.Get("/test", func(c *fiber.Ctx) error {
		_, ttl := GetIdempotencyKeyAndTTL(c)
		// Verify the default TTL is libRedis.TTL converted to seconds
		assert.Equal(t, expectedDefaultTTL, ttl,
			"default TTL should be libRedis.TTL (%d) * time.Second", libRedis.TTL)
		// Verify the TTL is positive
		assert.True(t, ttl > 0, "default TTL should be positive")
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	// No headers - testing pure default behavior

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}
