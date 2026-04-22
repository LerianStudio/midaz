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

// TestValidateSearchTermLength_StatusCaseInsensitive tests that status filter is case-insensitive.
func TestValidateSearchTermLength_StatusCaseInsensitive(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		inputStatus    string
		expectedStatus string
	}{
		{name: "lowercase active", inputStatus: "active", expectedStatus: "ACTIVE"},
		{name: "uppercase ACTIVE", inputStatus: "ACTIVE", expectedStatus: "ACTIVE"},
		{name: "mixed case Active", inputStatus: "Active", expectedStatus: "ACTIVE"},
		{name: "lowercase inactive", inputStatus: "inactive", expectedStatus: "INACTIVE"},
		{name: "uppercase INACTIVE", inputStatus: "INACTIVE", expectedStatus: "INACTIVE"},
		{name: "mixed case InActive", inputStatus: "InActive", expectedStatus: "INACTIVE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			params := map[string]string{"status": tc.inputStatus}
			result, err := ValidateParameters(params)

			require.NoError(t, err)
			require.NotNil(t, result.Status)
			assert.Equal(t, tc.expectedStatus, *result.Status, "status should be uppercased")
		})
	}
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

// TestValidateParameters_NewFilterFields tests the new filter fields added for CRM, onboarding,
// and transaction listing endpoints (P1-01).
func TestValidateParameters_NewFilterFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		params         map[string]string
		expectedStatus *string
		expectedType   *string
		expectedAsset  *string
		expectedEntity *string
		expectedKey    *string
	}{
		{
			name:           "all new filter fields nil by default",
			params:         map[string]string{},
			expectedStatus: nil,
			expectedType:   nil,
			expectedAsset:  nil,
			expectedEntity: nil,
			expectedKey:    nil,
		},
		{
			name:           "status filter parsed correctly",
			params:         map[string]string{"status": "ACTIVE"},
			expectedStatus: ptr("ACTIVE"),
			expectedType:   nil,
			expectedAsset:  nil,
			expectedEntity: nil,
			expectedKey:    nil,
		},
		{
			name:           "asset_code filter parsed correctly",
			params:         map[string]string{"asset_code": "BRL"},
			expectedStatus: nil,
			expectedType:   nil,
			expectedAsset:  ptr("BRL"),
			expectedEntity: nil,
			expectedKey:    nil,
		},
		{
			name:           "entity_id filter parsed correctly",
			params:         map[string]string{"entity_id": "123e4567-e89b-12d3-a456-426614174000"},
			expectedStatus: nil,
			expectedType:   nil,
			expectedAsset:  nil,
			expectedEntity: ptr("123e4567-e89b-12d3-a456-426614174000"),
			expectedKey:    nil,
		},
		{
			name:           "key_value filter parsed correctly",
			params:         map[string]string{"key_value": "savings"},
			expectedStatus: nil,
			expectedType:   nil,
			expectedAsset:  nil,
			expectedEntity: nil,
			expectedKey:    ptr("savings"),
		},
		{
			name:           "all new filter fields together",
			params:         map[string]string{"status": "INACTIVE", "asset_code": "USD", "entity_id": "abc-123", "key_value": "checking"},
			expectedStatus: ptr("INACTIVE"),
			expectedType:   nil,
			expectedAsset:  ptr("USD"),
			expectedEntity: ptr("abc-123"),
			expectedKey:    ptr("checking"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := ValidateParameters(tc.params)

			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify Status field
			if tc.expectedStatus == nil {
				assert.Nil(t, result.Status, "Status should be nil")
			} else {
				require.NotNil(t, result.Status, "Status should not be nil")
				assert.Equal(t, *tc.expectedStatus, *result.Status)
			}

			// Verify Type field (generic type filter, distinct from OperationType)
			if tc.expectedType == nil {
				assert.Nil(t, result.Type, "Type should be nil")
			} else {
				require.NotNil(t, result.Type, "Type should not be nil")
				assert.Equal(t, *tc.expectedType, *result.Type)
			}

			// Verify AssetCode field
			if tc.expectedAsset == nil {
				assert.Nil(t, result.AssetCode, "AssetCode should be nil")
			} else {
				require.NotNil(t, result.AssetCode, "AssetCode should not be nil")
				assert.Equal(t, *tc.expectedAsset, *result.AssetCode)
			}

			// Verify EntityID field
			if tc.expectedEntity == nil {
				assert.Nil(t, result.EntityID, "EntityID should be nil")
			} else {
				require.NotNil(t, result.EntityID, "EntityID should not be nil")
				assert.Equal(t, *tc.expectedEntity, *result.EntityID)
			}

			// Verify KeyValue field
			if tc.expectedKey == nil {
				assert.Nil(t, result.KeyValue, "KeyValue should be nil")
			} else {
				require.NotNil(t, result.KeyValue, "KeyValue should not be nil")
				assert.Equal(t, *tc.expectedKey, *result.KeyValue)
			}
		})
	}
}

// TestValidateParameters_TypeFieldDistinctFromOperationType verifies that the new Type field
// is separate from the existing OperationType field. The Type field is for account type filtering
// (e.g., "deposit", "savings"), while OperationType is for transaction operation types (e.g., "DEBIT", "CREDIT").
func TestValidateParameters_TypeFieldDistinctFromOperationType(t *testing.T) {
	t.Parallel()

	// When "type" query param is provided, it should populate OperationType (existing behavior)
	// AND the new Type field for account filtering
	params := map[string]string{"type": "deposit"}

	result, err := ValidateParameters(params)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Existing behavior: OperationType is set (uppercased)
	assert.Equal(t, "DEPOSIT", result.OperationType)

	// New behavior: Type field should also be populated
	require.NotNil(t, result.Type, "Type field should be populated when type query param is provided")
	assert.Equal(t, "deposit", *result.Type)
}

// TestValidateParameters_ExtendedFilters tests the extended filter fields (blocked, parent_account_id,
// legal_document, alias) added for onboarding list filters.
func TestValidateParameters_ExtendedFilters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                    string
		params                  map[string]string
		expectedBlocked         *bool
		expectedParentAccountID *string
		expectedLegalDocument   *string
		expectedAlias           *string
		expectError             bool
		errorContains           string
	}{
		{
			name:                    "all extended filter fields nil by default",
			params:                  map[string]string{},
			expectedBlocked:         nil,
			expectedParentAccountID: nil,
			expectedLegalDocument:   nil,
			expectedAlias:           nil,
			expectError:             false,
		},
		{
			name:            "blocked filter true",
			params:          map[string]string{"blocked": "true"},
			expectedBlocked: ptrBool(true),
			expectError:     false,
		},
		{
			name:            "blocked filter false",
			params:          map[string]string{"blocked": "false"},
			expectedBlocked: ptrBool(false),
			expectError:     false,
		},
		{
			name:            "blocked filter case insensitive TRUE",
			params:          map[string]string{"blocked": "TRUE"},
			expectedBlocked: ptrBool(true),
			expectError:     false,
		},
		{
			name:            "blocked filter case insensitive True",
			params:          map[string]string{"blocked": "True"},
			expectedBlocked: ptrBool(true),
			expectError:     false,
		},
		{
			name:            "blocked filter numeric 1",
			params:          map[string]string{"blocked": "1"},
			expectedBlocked: ptrBool(true),
			expectError:     false,
		},
		{
			name:            "blocked filter numeric 0",
			params:          map[string]string{"blocked": "0"},
			expectedBlocked: ptrBool(false),
			expectError:     false,
		},
		{
			name:          "blocked filter invalid value returns error",
			params:        map[string]string{"blocked": "invalid"},
			expectError:   true,
			errorContains: "blocked",
		},
		{
			name:                    "parent_account_id with valid UUID",
			params:                  map[string]string{"parent_account_id": "123e4567-e89b-12d3-a456-426614174000"},
			expectedParentAccountID: ptr("123e4567-e89b-12d3-a456-426614174000"),
			expectError:             false,
		},
		{
			name:          "parent_account_id with invalid UUID",
			params:        map[string]string{"parent_account_id": "not-a-valid-uuid"},
			expectError:   true,
			errorContains: "parent_account_id",
		},
		{
			name:                  "legal_document filter parsed correctly",
			params:                map[string]string{"legal_document": "12345678901"},
			expectedLegalDocument: ptr("12345678901"),
			expectError:           false,
		},
		{
			name:          "alias filter parsed correctly",
			params:        map[string]string{"alias": "my-account-alias"},
			expectedAlias: ptr("my-account-alias"),
			expectError:   false,
		},
		{
			name:          "alias filter trimmed",
			params:        map[string]string{"alias": "  trimmed-alias  "},
			expectedAlias: ptr("trimmed-alias"),
			expectError:   false,
		},
		{
			name:          "alias filter too long",
			params:        map[string]string{"alias": strings.Repeat("a", 257)},
			expectError:   true,
			errorContains: "alias",
		},
		{
			name:          "alias filter exactly 256 chars valid",
			params:        map[string]string{"alias": strings.Repeat("a", 256)},
			expectedAlias: ptr(strings.Repeat("a", 256)),
			expectError:   false,
		},
		{
			name:        "alias filter whitespace only becomes nil",
			params:      map[string]string{"alias": "   "},
			expectError: false,
		},
		{
			name:                    "all extended filters together",
			params:                  map[string]string{"blocked": "true", "parent_account_id": "123e4567-e89b-12d3-a456-426614174000", "legal_document": "CPF123", "alias": "test-alias"},
			expectedBlocked:         ptrBool(true),
			expectedParentAccountID: ptr("123e4567-e89b-12d3-a456-426614174000"),
			expectedLegalDocument:   ptr("CPF123"),
			expectedAlias:           ptr("test-alias"),
			expectError:             false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := ValidateParameters(tc.params)

			if tc.expectError {
				require.Error(t, err)
				if tc.errorContains != "" {
					assert.Contains(t, err.Error(), tc.errorContains)
				}
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			// Verify Blocked field
			if tc.expectedBlocked == nil {
				assert.Nil(t, result.Blocked, "Blocked should be nil")
			} else {
				require.NotNil(t, result.Blocked, "Blocked should not be nil")
				assert.Equal(t, *tc.expectedBlocked, *result.Blocked)
			}

			// Verify ParentAccountID field
			if tc.expectedParentAccountID == nil {
				assert.Nil(t, result.ParentAccountID, "ParentAccountID should be nil")
			} else {
				require.NotNil(t, result.ParentAccountID, "ParentAccountID should not be nil")
				assert.Equal(t, *tc.expectedParentAccountID, *result.ParentAccountID)
			}

			// Verify LegalDocument field
			if tc.expectedLegalDocument == nil {
				assert.Nil(t, result.LegalDocument, "LegalDocument should be nil")
			} else {
				require.NotNil(t, result.LegalDocument, "LegalDocument should not be nil")
				assert.Equal(t, *tc.expectedLegalDocument, *result.LegalDocument)
			}

			// Verify Alias field
			if tc.expectedAlias == nil {
				assert.Nil(t, result.Alias, "Alias should be nil")
			} else {
				require.NotNil(t, result.Alias, "Alias should not be nil")
				assert.Equal(t, *tc.expectedAlias, *result.Alias)
			}
		})
	}
}

// ptr is a helper function to create a pointer to a string value.
func ptr(s string) *string {
	return &s
}

// ptrBool is a helper function to create a pointer to a bool value.
func ptrBool(b bool) *bool {
	return &b
}

// TestParseBoolParam tests the parseBoolParam helper function for boolean query parameter parsing.
func TestParseBoolParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		input         string
		expectedValue *bool
		expectError   bool
	}{
		// Valid true values
		{name: "true lowercase", input: "true", expectedValue: ptrBool(true), expectError: false},
		{name: "TRUE uppercase", input: "TRUE", expectedValue: ptrBool(true), expectError: false},
		{name: "True mixed case", input: "True", expectedValue: ptrBool(true), expectError: false},
		{name: "1 numeric true", input: "1", expectedValue: ptrBool(true), expectError: false},

		// Valid false values
		{name: "false lowercase", input: "false", expectedValue: ptrBool(false), expectError: false},
		{name: "FALSE uppercase", input: "FALSE", expectedValue: ptrBool(false), expectError: false},
		{name: "False mixed case", input: "False", expectedValue: ptrBool(false), expectError: false},
		{name: "0 numeric false", input: "0", expectedValue: ptrBool(false), expectError: false},

		// Invalid values - must return error
		{name: "invalid string", input: "invalid", expectedValue: nil, expectError: true},
		{name: "yes is invalid", input: "yes", expectedValue: nil, expectError: true},
		{name: "no is invalid", input: "no", expectedValue: nil, expectError: true},
		{name: "2 is invalid", input: "2", expectedValue: nil, expectError: true},
		{name: "empty string is invalid", input: "", expectedValue: nil, expectError: true},
		{name: "whitespace is invalid", input: " ", expectedValue: nil, expectError: true},
		{name: "tRuE weird case", input: "tRuE", expectedValue: ptrBool(true), expectError: false},
		{name: "fAlSe weird case", input: "fAlSe", expectedValue: ptrBool(false), expectError: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			result, err := parseBoolParam(tc.input)

			if tc.expectError {
				require.Error(t, err, "expected error for input: %q", tc.input)
				assert.Nil(t, result, "result should be nil when error is returned")
				return
			}

			require.NoError(t, err, "unexpected error for input: %q", tc.input)
			require.NotNil(t, result, "result should not be nil for valid input: %q", tc.input)
			assert.Equal(t, *tc.expectedValue, *result, "unexpected value for input: %q", tc.input)
		})
	}
}
