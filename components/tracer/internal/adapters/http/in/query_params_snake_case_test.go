// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tests that snake_case query params are bound correctly by Fiber's QueryParser.
// These tests exercise the struct tags directly via Fiber's query binding.

func TestSnakeCaseQueryParamBinding(t *testing.T) {
	tests := []struct {
		name     string
		handler  func(*fiber.Ctx) error
		url      string
		expected map[string]interface{}
	}{
		{
			name: "ListRulesInput",
			handler: func(c *fiber.Ctx) error {
				var input ListRulesInput
				if err := c.QueryParser(&input); err != nil {
					return c.SendStatus(fiber.StatusBadRequest)
				}

				return c.JSON(input)
			},
			url: "/test?sort_by=created_at&sort_order=DESC&account_id=550e8400-e29b-41d4-a716-446655440000&transaction_type=CARD&sub_type=debit",
			expected: map[string]interface{}{
				"SortBy":          "created_at",
				"SortOrder":       "DESC",
				"AccountID":       "550e8400-e29b-41d4-a716-446655440000",
				"TransactionType": "CARD",
				"SubType":         "debit",
			},
		},
		{
			name: "ListLimitsInput",
			handler: func(c *fiber.Ctx) error {
				var input ListLimitsInput
				if err := c.QueryParser(&input); err != nil {
					return c.SendStatus(fiber.StatusBadRequest)
				}

				return c.JSON(input)
			},
			url: "/test?sort_by=created_at&sort_order=ASC&account_id=550e8400-e29b-41d4-a716-446655440000&limit_type=DAILY",
			expected: map[string]interface{}{
				"SortBy":    "created_at",
				"SortOrder": "ASC",
				"AccountID": "550e8400-e29b-41d4-a716-446655440000",
				"LimitType": "DAILY",
			},
		},
		{
			name: "ListAuditEventsInput",
			handler: func(c *fiber.Ctx) error {
				var input ListAuditEventsInput
				if err := c.QueryParser(&input); err != nil {
					return c.SendStatus(fiber.StatusBadRequest)
				}

				return c.JSON(input)
			},
			url: "/test?sort_by=created_at&sort_order=DESC&start_date=2026-01-01T00:00:00Z&end_date=2026-12-31T23:59:59Z&event_type=RULE_CREATED&resource_type=rule&resource_id=550e8400-e29b-41d4-a716-446655440000&actor_type=user&actor_id=user-123&matched_rule_id=660e8400-e29b-41d4-a716-446655440000",
			expected: map[string]interface{}{
				"SortBy":        "created_at",
				"SortOrder":     "DESC",
				"StartDate":     "2026-01-01T00:00:00Z",
				"EndDate":       "2026-12-31T23:59:59Z",
				"EventType":     "RULE_CREATED",
				"ResourceType":  "rule",
				"ResourceID":    "550e8400-e29b-41d4-a716-446655440000",
				"ActorType":     "user",
				"ActorID":       "user-123",
				"MatchedRuleID": "660e8400-e29b-41d4-a716-446655440000",
			},
		},
		{
			name: "ListTransactionValidationsInput",
			handler: func(c *fiber.Ctx) error {
				var input ListTransactionValidationsInput
				if err := c.QueryParser(&input); err != nil {
					return c.SendStatus(fiber.StatusBadRequest)
				}

				return c.JSON(input)
			},
			url: "/test?sort_by=created_at&sort_order=DESC&start_date=2026-01-01T00:00:00Z&end_date=2026-12-31T23:59:59Z&account_id=550e8400-e29b-41d4-a716-446655440000&matched_rule_id=660e8400-e29b-41d4-a716-446655440000&exceeded_limit_id=770e8400-e29b-41d4-a716-446655440000&transaction_type=CARD",
			expected: map[string]interface{}{
				"SortBy":          "created_at",
				"SortOrder":       "DESC",
				"StartDate":       "2026-01-01T00:00:00Z",
				"EndDate":         "2026-12-31T23:59:59Z",
				"AccountID":       "550e8400-e29b-41d4-a716-446655440000",
				"MatchedRuleID":   "660e8400-e29b-41d4-a716-446655440000",
				"ExceededLimitID": "770e8400-e29b-41d4-a716-446655440000",
				"TransactionType": "CARD",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/test", tt.handler)

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, fiber.StatusOK, resp.StatusCode)

			var result map[string]interface{}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

			for key, expectedVal := range tt.expected {
				assert.Equal(t, expectedVal, result[key], "field %s mismatch", key)
			}
		})
	}
}

func TestListRulesInput_CamelCaseIgnored(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		var input ListRulesInput
		if err := c.QueryParser(&input); err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}

		return c.JSON(input)
	})

	// Send old camelCase params - these should NOT bind after the change
	req := httptest.NewRequest(http.MethodGet,
		"/test?sortBy=createdAt&sortOrder=DESC&accountId=550e8400-e29b-41d4-a716-446655440000",
		nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, fiber.StatusOK, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	// After snake_case change, camelCase params should be ignored (empty)
	assert.Equal(t, "", result["SortBy"])
	assert.Equal(t, "", result["SortOrder"])
	assert.Nil(t, result["AccountID"])
}

func TestListLimitsInput_CamelCaseIgnored(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		var input ListLimitsInput
		if err := c.QueryParser(&input); err != nil {
			return c.SendStatus(400)
		}

		return c.JSON(input)
	})

	// Send camelCase params — they should NOT bind
	req := httptest.NewRequest("GET", "/test?sortBy=created_at&sortOrder=DESC&limitType=DAILY", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	// camelCase params should not bind to snake_case tags
	assert.Empty(t, result["SortBy"], "camelCase sortBy should not bind to sort_by tag")
	assert.Empty(t, result["SortOrder"], "camelCase sortOrder should not bind to sort_order tag")
	assert.Empty(t, result["LimitType"], "camelCase limitType should not bind to limit_type tag")
}

func TestListAuditEventsInput_CamelCaseIgnored(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		var input ListAuditEventsInput
		if err := c.QueryParser(&input); err != nil {
			return c.SendStatus(400)
		}

		return c.JSON(input)
	})

	// Send camelCase params — they should NOT bind
	req := httptest.NewRequest("GET", "/test?sortBy=created_at&startDate=2026-01-01T00:00:00Z&eventType=RULE_CREATED", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	// camelCase params should not bind to snake_case tags
	assert.Empty(t, result["SortBy"], "camelCase sortBy should not bind to sort_by tag")
	assert.Empty(t, result["StartDate"], "camelCase startDate should not bind to start_date tag")
	assert.Nil(t, result["EventType"], "camelCase eventType should not bind to event_type tag")
}

func TestListTransactionValidationsInput_CamelCaseIgnored(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c *fiber.Ctx) error {
		var input ListTransactionValidationsInput
		if err := c.QueryParser(&input); err != nil {
			return c.SendStatus(400)
		}

		return c.JSON(input)
	})

	// Send camelCase params — they should NOT bind
	req := httptest.NewRequest("GET", "/test?sortBy=created_at&startDate=2026-01-01T00:00:00Z&accountId=test-uuid", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))

	// camelCase params should not bind to snake_case tags
	assert.Empty(t, result["SortBy"], "camelCase sortBy should not bind to sort_by tag")
	assert.Empty(t, result["StartDate"], "camelCase startDate should not bind to start_date tag")
	assert.Empty(t, result["AccountID"], "camelCase accountId should not bind to account_id tag")
}

// Test that snake_case sort values pass validation whitelist
func TestListRulesInput_SnakeCaseSortByValidation(t *testing.T) {
	input := &ListRulesInput{
		SortBy:    "created_at",
		SortOrder: "DESC",
	}
	err := input.Validate()
	assert.NoError(t, err, "created_at should be an allowed sort field")
}

func TestListLimitsInput_SnakeCaseSortByValidation(t *testing.T) {
	input := &ListLimitsInput{
		SortBy:    "max_amount",
		SortOrder: "ASC",
	}
	err := input.Validate()
	assert.NoError(t, err, "max_amount should be an allowed sort field")
}

func TestListAuditEventsInput_SnakeCaseSortByValidation(t *testing.T) {
	input := &ListAuditEventsInput{
		SortBy:    "event_type",
		SortOrder: "DESC",
	}
	err := input.Validate()
	assert.NoError(t, err, "event_type should be an allowed sort field")
}

func TestListTransactionValidationsInput_SnakeCaseSortByValidation(t *testing.T) {
	input := &ListTransactionValidationsInput{
		SortBy:    "processing_time_ms",
		SortOrder: "DESC",
	}
	err := input.Validate()
	assert.NoError(t, err, "processing_time_ms should be an allowed sort field")
}
