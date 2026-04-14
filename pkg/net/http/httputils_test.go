// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	libConstants "github.com/LerianStudio/lib-commons/v4/commons/constants"
	libHTTP "github.com/LerianStudio/lib-commons/v4/commons/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
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
	legacyCursor := "eyJpZCI6IjEyMyJ9" // {"id":"123"} in base64

	params := map[string]string{
		"cursor": legacyCursor,
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.NotEqual(t, legacyCursor, result.Cursor)

	decodedCursor, err := libHTTP.DecodeCursor(result.Cursor)
	require.NoError(t, err)
	assert.Equal(t, "123", decodedCursor.ID)
	assert.Equal(t, libHTTP.CursorDirectionPrev, decodedCursor.Direction)
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

func TestValidateParameters_WithSegmentID(t *testing.T) {
	validUUID := "123e4567-e89b-12d3-a456-426614174000"
	params := map[string]string{
		"segment_id": validUUID,
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Equal(t, validUUID, result.SegmentID)
}

func TestValidateParameters_WithInvalidSegmentID(t *testing.T) {
	params := map[string]string{
		"segment_id": "invalid-uuid",
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
	// type parameter also populates FilterType with original case
	require.NotNil(t, result.FilterType)
	assert.Equal(t, "credit", *result.FilterType)
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
	t.Setenv("MAX_PAGINATION_LIMIT", "100")

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
	t.Setenv("MAX_PAGINATION_MONTH_DATE_RANGE", "0")

	startDate := time.Time{}
	endDate := time.Time{}

	err := validateDates(&startDate, &endDate)

	require.NoError(t, err)
	// When max is 0, startDate should be epoch
	assert.Equal(t, int64(0), startDate.Unix())
}

func TestValidatePagination_ValidParams(t *testing.T) {
	_, err := validatePagination("", "asc", 10)

	require.NoError(t, err)
}

func TestValidatePagination_ValidParamsDesc(t *testing.T) {
	_, err := validatePagination("", "desc", 50)

	require.NoError(t, err)
}

func TestValidatePagination_InvalidSortOrder(t *testing.T) {
	_, err := validatePagination("", "invalid", 10)

	assert.Error(t, err)
}

func TestValidatePagination_LimitExceeded(t *testing.T) {
	t.Setenv("MAX_PAGINATION_LIMIT", "100")

	_, err := validatePagination("", "asc", 150)

	assert.Error(t, err)
}

func TestValidatePagination_InvalidCursor(t *testing.T) {
	_, err := validatePagination("invalid-cursor", "asc", 10)

	assert.Error(t, err)
}

func TestValidatePagination_ValidV4Cursor(t *testing.T) {
	cursor := "eyJpZCI6IjEyMyIsImRpcmVjdGlvbiI6Im5leHQifQ=="

	normalizedCursor, err := validatePagination(cursor, "asc", 10)

	require.NoError(t, err)
	assert.Equal(t, cursor, normalizedCursor)
}

func TestValidatePagination_NormalizesLegacyCursor(t *testing.T) {
	legacyCursor := "eyJpZCI6IjEyMyIsInBvaW50c19uZXh0Ijp0cnVlfQ=="

	normalizedCursor, err := validatePagination(legacyCursor, "asc", 10)

	require.NoError(t, err)
	assert.NotEqual(t, legacyCursor, normalizedCursor)

	decodedCursor, err := libHTTP.DecodeCursor(normalizedCursor)
	require.NoError(t, err)
	assert.Equal(t, "123", decodedCursor.ID)
	assert.Equal(t, libHTTP.CursorDirectionNext, decodedCursor.Direction)

	decodedLegacy, err := base64.StdEncoding.DecodeString(legacyCursor)
	require.NoError(t, err)
	assert.Contains(t, string(decodedLegacy), "points_next")
}

func TestValidatePagination_NormalizesLegacyCursorWithoutPointsNext(t *testing.T) {
	legacyCursor := "eyJpZCI6IjEyMyJ9"

	normalizedCursor, err := validatePagination(legacyCursor, "asc", 10)

	require.NoError(t, err)

	decodedCursor, err := libHTTP.DecodeCursor(normalizedCursor)
	require.NoError(t, err)
	assert.Equal(t, "123", decodedCursor.ID)
	assert.Equal(t, libHTTP.CursorDirectionPrev, decodedCursor.Direction)
}

func TestGetIdempotencyKeyAndTTL_WithValidValues(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		key, ttl := GetIdempotencyKeyAndTTL(c)
		assert.Equal(t, "test-key", key)
		assert.Equal(t, time.Duration(60), ttl)
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
		assert.Equal(t, time.Duration(300), ttl)
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
		assert.Equal(t, time.Duration(300), ttl)
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
		assert.Equal(t, time.Duration(300), ttl)
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
	part, err := writer.CreateFormFile(libConstants.DSL, "test.txt")
	require.NoError(t, err)
	_, err = io.WriteString(part, "file content")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

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
	part, err := writer.CreateFormFile(libConstants.DSL, "test"+libConstants.FileExtension)
	require.NoError(t, err)
	_, err = io.WriteString(part, "")
	require.NoError(t, err)
	require.NoError(t, writer.Close())

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
	part, err := writer.CreateFormFile(libConstants.DSL, "test"+libConstants.FileExtension)
	require.NoError(t, err)
	_, err = io.WriteString(part, expectedContent)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

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
	// type parameter also populates FilterType with original case
	require.NotNil(t, result.FilterType)
	assert.Equal(t, "debit", *result.FilterType)
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

func TestGetUUIDFromLocals_ValidUUID(t *testing.T) {
	app := fiber.New()
	testUUID := uuid.New()

	app.Get("/test/:id", func(c *fiber.Ctx) error {
		c.Locals("id", testUUID)
		result, err := GetUUIDFromLocals(c, "id")
		assert.NoError(t, err)
		assert.Equal(t, testUUID, result)
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test/"+testUUID.String(), nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestGetUUIDFromLocals_NilValue(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		result, err := GetUUIDFromLocals(c, "id")
		assert.Error(t, err)
		assert.Equal(t, uuid.Nil, result)
		return c.SendStatus(fiber.StatusBadRequest)
	})

	req := httptest.NewRequest("GET", "/test", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestGetUUIDFromLocals_WrongType(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		c.Locals("id", "not-a-uuid-object")
		result, err := GetUUIDFromLocals(c, "id")
		assert.Error(t, err)
		assert.Equal(t, uuid.Nil, result)
		return c.SendStatus(fiber.StatusBadRequest)
	})

	req := httptest.NewRequest("GET", "/test", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestGetUUIDFromLocals_WrongTypeInteger(t *testing.T) {
	app := fiber.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		c.Locals("id", 12345)
		result, err := GetUUIDFromLocals(c, "id")
		assert.Error(t, err)
		assert.Equal(t, uuid.Nil, result)
		return c.SendStatus(fiber.StatusBadRequest)
	})

	req := httptest.NewRequest("GET", "/test", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
}

func TestGetUUIDFromLocals_DifferentKeys(t *testing.T) {
	app := fiber.New()
	holderID := uuid.New()
	aliasID := uuid.New()

	app.Get("/test", func(c *fiber.Ctx) error {
		c.Locals("holder_id", holderID)
		c.Locals("alias_id", aliasID)

		resultHolder, err := GetUUIDFromLocals(c, "holder_id")
		assert.NoError(t, err)
		assert.Equal(t, holderID, resultHolder)

		resultAlias, err := GetUUIDFromLocals(c, "alias_id")
		assert.NoError(t, err)
		assert.Equal(t, aliasID, resultAlias)

		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)

	resp, err := app.Test(req, -1)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
}

func TestEscapeSearchMetacharacters_NoSpecialChars(t *testing.T) {
	result := EscapeSearchMetacharacters("Lerian Financial")
	assert.Equal(t, "Lerian Financial", result)
}

func TestEscapeSearchMetacharacters_Percent(t *testing.T) {
	result := EscapeSearchMetacharacters("100% Match")
	assert.Equal(t, `100\% Match`, result)
}

func TestEscapeSearchMetacharacters_Underscore(t *testing.T) {
	result := EscapeSearchMetacharacters("test_name")
	assert.Equal(t, `test\_name`, result)
}

func TestEscapeSearchMetacharacters_Backslash(t *testing.T) {
	result := EscapeSearchMetacharacters(`path\to\file`)
	assert.Equal(t, `path\\to\\file`, result)
}

func TestEscapeSearchMetacharacters_AllSpecialChars(t *testing.T) {
	result := EscapeSearchMetacharacters(`100% Match_Test\Path`)
	assert.Equal(t, `100\% Match\_Test\\Path`, result)
}

func TestEscapeSearchMetacharacters_EmptyString(t *testing.T) {
	result := EscapeSearchMetacharacters("")
	assert.Equal(t, "", result)
}

func TestEscapeSearchMetacharacters_OnlyPercent(t *testing.T) {
	result := EscapeSearchMetacharacters("%")
	assert.Equal(t, `\%`, result)
}

func TestEscapeSearchMetacharacters_OnlyUnderscore(t *testing.T) {
	result := EscapeSearchMetacharacters("_")
	assert.Equal(t, `\_`, result)
}

func TestEscapeSearchMetacharacters_MultiplePercents(t *testing.T) {
	result := EscapeSearchMetacharacters("%%")
	assert.Equal(t, `\%\%`, result)
}

func TestValidateParameters_WithName(t *testing.T) {
	params := map[string]string{
		"name": "BRL Ledger",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.NotNil(t, result.Name)
	assert.Equal(t, "BRL Ledger", *result.Name)
}

func TestValidateParameters_WithLegalName(t *testing.T) {
	params := map[string]string{
		"legal_name": "Lerian Financial",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.NotNil(t, result.LegalName)
	assert.Equal(t, "Lerian Financial", *result.LegalName)
}

func TestValidateParameters_WithDoingBusinessAs(t *testing.T) {
	params := map[string]string{
		"doing_business_as": "Lerian FS",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.NotNil(t, result.DoingBusinessAs)
	assert.Equal(t, "Lerian FS", *result.DoingBusinessAs)
}

func TestValidateParameters_NameTooLong(t *testing.T) {
	longName := strings.Repeat("a", 257)
	params := map[string]string{
		"name": longName,
	}

	result, err := ValidateParameters(params)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateParameters_LegalNameTooLong(t *testing.T) {
	longName := strings.Repeat("a", 257)
	params := map[string]string{
		"legal_name": longName,
	}

	result, err := ValidateParameters(params)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateParameters_DoingBusinessAsTooLong(t *testing.T) {
	longName := strings.Repeat("a", 257)
	params := map[string]string{
		"doing_business_as": longName,
	}

	result, err := ValidateParameters(params)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestValidateParameters_NameExactly256Chars(t *testing.T) {
	name := strings.Repeat("a", 256)
	params := map[string]string{
		"name": name,
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.NotNil(t, result.Name)
	assert.Equal(t, name, *result.Name)
}

func TestValidateParameters_NameSingleChar(t *testing.T) {
	params := map[string]string{
		"name": "A",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.NotNil(t, result.Name)
	assert.Equal(t, "A", *result.Name)
}

func TestValidateParameters_NameWithWhitespaceOnly(t *testing.T) {
	params := map[string]string{
		"name": "   ",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Nil(t, result.Name)
}

func TestValidateParameters_NameTrimmed(t *testing.T) {
	params := map[string]string{
		"name": "  BRL  ",
	}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.NotNil(t, result.Name)
	assert.Equal(t, "BRL", *result.Name)
}

func TestValidateParameters_SearchFieldsNilByDefault(t *testing.T) {
	params := make(map[string]string)

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Nil(t, result.Name)
	assert.Nil(t, result.LegalName)
	assert.Nil(t, result.DoingBusinessAs)
}

func TestValidateParameters_WithDirectionDebit(t *testing.T) {
	params := map[string]string{"direction": "debit"}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	require.NotNil(t, result.Direction)
	assert.Equal(t, "debit", *result.Direction)
}

func TestValidateParameters_WithDirectionCredit(t *testing.T) {
	params := map[string]string{"direction": "CREDIT"}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	require.NotNil(t, result.Direction)
	assert.Equal(t, "credit", *result.Direction)
}

func TestValidateParameters_WithInvalidDirection(t *testing.T) {
	params := map[string]string{"direction": "invalid"}

	result, err := ValidateParameters(params)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "direction")
	assert.Nil(t, result)
}

func TestValidateParameters_WithRouteID(t *testing.T) {
	routeID := uuid.New().String()
	params := map[string]string{"route_id": routeID}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	require.NotNil(t, result.RouteID)
	assert.Equal(t, routeID, *result.RouteID)
}

func TestValidateParameters_WithInvalidRouteID(t *testing.T) {
	params := map[string]string{"route_id": "not-a-uuid"}

	result, err := ValidateParameters(params)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "route_id")
	assert.Nil(t, result)
}

func TestValidateParameters_DirectionAndRouteIDNilByDefault(t *testing.T) {
	params := make(map[string]string)

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	assert.Nil(t, result.Direction)
	assert.Nil(t, result.RouteID)
}

// TestQueryHeader_GenericFilterFields verifies that the generic filter fields
// exist in QueryHeader struct and can be assigned pointer values.
// These fields enable filtering across multiple GET list endpoints.
func TestQueryHeader_GenericFilterFields(t *testing.T) {
	tests := []struct {
		name      string
		setupFunc func() *QueryHeader
		assertFn  func(t *testing.T, qh *QueryHeader)
	}{
		{
			name: "Status field accepts pointer string",
			setupFunc: func() *QueryHeader {
				status := "ACTIVE"
				return &QueryHeader{Status: &status}
			},
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.Status)
				assert.Equal(t, "ACTIVE", *qh.Status)
			},
		},
		{
			name: "FilterType field accepts pointer string",
			setupFunc: func() *QueryHeader {
				filterType := "customer"
				return &QueryHeader{FilterType: &filterType}
			},
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.FilterType)
				assert.Equal(t, "customer", *qh.FilterType)
			},
		},
		{
			name: "AssetCode field accepts pointer string",
			setupFunc: func() *QueryHeader {
				assetCode := "BRL"
				return &QueryHeader{AssetCode: &assetCode}
			},
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.AssetCode)
				assert.Equal(t, "BRL", *qh.AssetCode)
			},
		},
		{
			name: "EntityID field accepts pointer string",
			setupFunc: func() *QueryHeader {
				entityID := "123e4567-e89b-12d3-a456-426614174000"
				return &QueryHeader{EntityID: &entityID}
			},
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.EntityID)
				assert.Equal(t, "123e4567-e89b-12d3-a456-426614174000", *qh.EntityID)
			},
		},
		{
			name: "TransactionID field accepts pointer string",
			setupFunc: func() *QueryHeader {
				txID := "123e4567-e89b-12d3-a456-426614174001"
				return &QueryHeader{TransactionID: &txID}
			},
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.TransactionID)
				assert.Equal(t, "123e4567-e89b-12d3-a456-426614174001", *qh.TransactionID)
			},
		},
		{
			name: "ParentTransactionID field accepts pointer string",
			setupFunc: func() *QueryHeader {
				parentTxID := "123e4567-e89b-12d3-a456-426614174002"
				return &QueryHeader{ParentTransactionID: &parentTxID}
			},
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.ParentTransactionID)
				assert.Equal(t, "123e4567-e89b-12d3-a456-426614174002", *qh.ParentTransactionID)
			},
		},
		{
			name: "KeyValue field accepts pointer string",
			setupFunc: func() *QueryHeader {
				keyValue := "custom-key-value"
				return &QueryHeader{KeyValue: &keyValue}
			},
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.KeyValue)
				assert.Equal(t, "custom-key-value", *qh.KeyValue)
			},
		},
		{
			name: "all generic filter fields nil by default",
			setupFunc: func() *QueryHeader {
				return &QueryHeader{}
			},
			assertFn: func(t *testing.T, qh *QueryHeader) {
				assert.Nil(t, qh.Status)
				assert.Nil(t, qh.FilterType)
				assert.Nil(t, qh.AssetCode)
				assert.Nil(t, qh.EntityID)
				assert.Nil(t, qh.TransactionID)
				assert.Nil(t, qh.ParentTransactionID)
				assert.Nil(t, qh.KeyValue)
			},
		},
		{
			name: "all generic filter fields can be set simultaneously",
			setupFunc: func() *QueryHeader {
				status := "ACTIVE"
				filterType := "customer"
				assetCode := "USD"
				entityID := "entity-123"
				txID := "tx-456"
				parentTxID := "parent-tx-789"
				keyValue := "key-val"
				return &QueryHeader{
					Status:              &status,
					FilterType:          &filterType,
					AssetCode:           &assetCode,
					EntityID:            &entityID,
					TransactionID:       &txID,
					ParentTransactionID: &parentTxID,
					KeyValue:            &keyValue,
				}
			},
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.Status)
				require.NotNil(t, qh.FilterType)
				require.NotNil(t, qh.AssetCode)
				require.NotNil(t, qh.EntityID)
				require.NotNil(t, qh.TransactionID)
				require.NotNil(t, qh.ParentTransactionID)
				require.NotNil(t, qh.KeyValue)
				assert.Equal(t, "ACTIVE", *qh.Status)
				assert.Equal(t, "customer", *qh.FilterType)
				assert.Equal(t, "USD", *qh.AssetCode)
				assert.Equal(t, "entity-123", *qh.EntityID)
				assert.Equal(t, "tx-456", *qh.TransactionID)
				assert.Equal(t, "parent-tx-789", *qh.ParentTransactionID)
				assert.Equal(t, "key-val", *qh.KeyValue)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qh := tt.setupFunc()
			tt.assertFn(t, qh)
		})
	}
}

// TestValidateParameters_GenericFilterFields verifies that ValidateParameters
// correctly parses and validates the generic filter fields from query parameters.
func TestValidateParameters_GenericFilterFields(t *testing.T) {
	validUUID := "123e4567-e89b-12d3-a456-426614174000"
	validParentUUID := "123e4567-e89b-12d3-a456-426614174001"

	tests := []struct {
		name        string
		params      map[string]string
		wantErr     bool
		errContains string
		assertFn    func(t *testing.T, qh *QueryHeader)
	}{
		{
			name: "status parameter is parsed and stored as-is",
			params: map[string]string{
				"status": "ACTIVE",
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.Status)
				assert.Equal(t, "ACTIVE", *qh.Status)
			},
		},
		{
			name: "status parameter preserves lowercase input",
			params: map[string]string{
				"status": "active",
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.Status)
				assert.Equal(t, "active", *qh.Status)
			},
		},
		{
			name: "filter_type parameter is parsed",
			params: map[string]string{
				"filter_type": "deposit",
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.FilterType)
				assert.Equal(t, "deposit", *qh.FilterType)
			},
		},
		{
			name: "type parameter populates both OperationType and FilterType",
			params: map[string]string{
				"type": "deposit",
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				// OperationType stores uppercase (backward compat)
				assert.Equal(t, "DEPOSIT", qh.OperationType)
				// FilterType stores original case (for new handlers)
				require.NotNil(t, qh.FilterType)
				assert.Equal(t, "deposit", *qh.FilterType)
			},
		},
		{
			name: "asset_code parameter is parsed",
			params: map[string]string{
				"asset_code": "USD",
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.AssetCode)
				assert.Equal(t, "USD", *qh.AssetCode)
			},
		},
		{
			name: "entity_id parameter is parsed",
			params: map[string]string{
				"entity_id": "entity-123",
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.EntityID)
				assert.Equal(t, "entity-123", *qh.EntityID)
			},
		},
		{
			name: "transaction_id parameter is parsed with valid UUID",
			params: map[string]string{
				"transaction_id": validUUID,
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.TransactionID)
				assert.Equal(t, validUUID, *qh.TransactionID)
			},
		},
		{
			name: "transaction_id parameter with invalid UUID returns error",
			params: map[string]string{
				"transaction_id": "invalid-uuid",
			},
			wantErr:     true,
			errContains: "transaction_id",
		},
		{
			name: "parent_transaction_id parameter is parsed with valid UUID",
			params: map[string]string{
				"parent_transaction_id": validParentUUID,
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.ParentTransactionID)
				assert.Equal(t, validParentUUID, *qh.ParentTransactionID)
			},
		},
		{
			name: "parent_transaction_id parameter with invalid UUID returns error",
			params: map[string]string{
				"parent_transaction_id": "not-a-uuid",
			},
			wantErr:     true,
			errContains: "parent_transaction_id",
		},
		{
			name: "key_value parameter is parsed",
			params: map[string]string{
				"key_value": "savings",
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.KeyValue)
				assert.Equal(t, "savings", *qh.KeyValue)
			},
		},
		{
			name: "multiple generic filter parameters are parsed together",
			params: map[string]string{
				"status":                "ACTIVE",
				"filter_type":           "customer",
				"asset_code":            "BRL",
				"entity_id":             "entity-456",
				"transaction_id":        validUUID,
				"parent_transaction_id": validParentUUID,
				"key_value":             "checking",
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.Status)
				require.NotNil(t, qh.FilterType)
				require.NotNil(t, qh.AssetCode)
				require.NotNil(t, qh.EntityID)
				require.NotNil(t, qh.TransactionID)
				require.NotNil(t, qh.ParentTransactionID)
				require.NotNil(t, qh.KeyValue)
				assert.Equal(t, "ACTIVE", *qh.Status)
				assert.Equal(t, "customer", *qh.FilterType)
				assert.Equal(t, "BRL", *qh.AssetCode)
				assert.Equal(t, "entity-456", *qh.EntityID)
				assert.Equal(t, validUUID, *qh.TransactionID)
				assert.Equal(t, validParentUUID, *qh.ParentTransactionID)
				assert.Equal(t, "checking", *qh.KeyValue)
			},
		},
		{
			name: "generic filter parameters combine with pagination",
			params: map[string]string{
				"status": "INACTIVE",
				"limit":  "50",
				"page":   "2",
			},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				require.NotNil(t, qh.Status)
				assert.Equal(t, "INACTIVE", *qh.Status)
				assert.Equal(t, 50, qh.Limit)
				assert.Equal(t, 2, qh.Page)
			},
		},
		{
			name:    "empty params returns nil for all generic filter fields",
			params:  map[string]string{},
			wantErr: false,
			assertFn: func(t *testing.T, qh *QueryHeader) {
				assert.Nil(t, qh.Status)
				assert.Nil(t, qh.FilterType)
				assert.Nil(t, qh.AssetCode)
				assert.Nil(t, qh.EntityID)
				assert.Nil(t, qh.TransactionID)
				assert.Nil(t, qh.ParentTransactionID)
				assert.Nil(t, qh.KeyValue)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qh, err := ValidateParameters(tt.params)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, qh)
			if tt.assertFn != nil {
				tt.assertFn(t, qh)
			}
		})
	}
}
