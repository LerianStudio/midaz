// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	shared "github.com/LerianStudio/midaz/v4/tests/reporter/e2e/shared"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TC-QP-001: GET /v1/templates?limit=abc returns 400 with TPL-0019.
func TestQP_InvalidLimitNonNumeric(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllTemplatesRaw(ctx, map[string]string{"limit": "abc"})
	require.NoError(t, err, "request should not return a transport error")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for non-numeric limit")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0019")
}

// TC-QP-002: GET /v1/templates?limit=0 returns 400 with TPL-0019.
func TestQP_InvalidLimitZero(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllTemplatesRaw(ctx, map[string]string{"limit": "0"})
	require.NoError(t, err, "request should not return a transport error")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for zero limit")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0019")
}

// TC-QP-003: GET /v1/templates?limit=-1 returns 400 with TPL-0019.
func TestQP_InvalidLimitNegative(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllTemplatesRaw(ctx, map[string]string{"limit": "-1"})
	require.NoError(t, err, "request should not return a transport error")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for negative limit")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0019")
}

// TC-QP-004: GET /v1/templates?limit=200 returns 400 with TPL-0024 (max is 100).
func TestQP_LimitExceedsMax(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllTemplatesRaw(ctx, map[string]string{"limit": "200"})
	require.NoError(t, err, "request should not return a transport error")
	// ErrPaginationLimitExceeded is mapped to ValidationError in pkg/errors.go,
	// which WithError() translates to HTTP 400 (BadRequest).
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for limit exceeding max (ValidationError mapping)")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0024")
}

// TC-QP-005: GET /v1/templates?page=0 returns 400 with TPL-0019.
func TestQP_InvalidPageZero(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllTemplatesRaw(ctx, map[string]string{"page": "0"})
	require.NoError(t, err, "request should not return a transport error")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for zero page")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0019")
}

// TC-QP-006: GET /v1/templates?sort_order=invalid returns 400 with TPL-0025.
func TestQP_InvalidSortOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllTemplatesRaw(ctx, map[string]string{"sort_order": "invalid"})
	require.NoError(t, err, "request should not return a transport error")
	// ErrInvalidSortOrder is mapped to ValidationError in pkg/errors.go,
	// which WithError() translates to HTTP 400 (BadRequest).
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for invalid sort_order (ValidationError mapping)")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0025")
}

// TC-QP-007: GET /v1/templates?output_format=docx returns 400 with TPL-0003.
func TestQP_InvalidOutputFormatInQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllTemplatesRaw(ctx, map[string]string{"output_format": "docx"})
	require.NoError(t, err, "request should not return a transport error")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for invalid output_format")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0003")
}

// ############################################################################
// Report Query Param Validation (TC-QP-009 to TC-QP-012)
// ############################################################################

// TC-QP-009: GET /v1/reports?limit=abc returns 400 with TPL-0019.
func TestQP_ReportsInvalidLimitNonNumeric(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllReportsRaw(ctx, map[string]string{"limit": "abc"})
	require.NoError(t, err, "request should not return a transport error")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for non-numeric limit")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0019")
}

// TC-QP-010: GET /v1/reports?limit=0 returns 400 with TPL-0019.
func TestQP_ReportsInvalidLimitZero(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllReportsRaw(ctx, map[string]string{"limit": "0"})
	require.NoError(t, err, "request should not return a transport error")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for zero limit")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0019")
}

// TC-QP-011: GET /v1/reports?page=0 returns 400 with TPL-0019.
func TestQP_ReportsInvalidPageZero(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllReportsRaw(ctx, map[string]string{"page": "0"})
	require.NoError(t, err, "request should not return a transport error")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for zero page")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0019")
}

// TC-QP-012: GET /v1/reports?sort_order=invalid returns error with TPL-0025.
func TestQP_ReportsInvalidSortOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	resp, err := apiClient.GetAllReportsRaw(ctx, map[string]string{"sort_order": "invalid"})
	require.NoError(t, err, "request should not return a transport error")
	// ErrInvalidSortOrder is mapped to ValidationError in pkg/errors.go,
	// which WithError() translates to HTTP 400 (BadRequest).
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "should return 400 for invalid sort_order (ValidationError mapping)")

	var body map[string]any

	require.NoError(t, json.Unmarshal(resp.Body(), &body), "response should be valid JSON")
	shared.AssertErrorCode(t, body, "TPL-0025")
}

// TC-QP-008: Default sort order is desc - templates listed without sort_order
// should be ordered by createdAt descending.
func TestQP_DefaultSortOrderDesc(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Use a unique tag to isolate this test's templates from other parallel tests.
	tag := shared.UniqueID("sort")
	tplBytes := shared.LoadFixture(t, shared.FixtureValidHTML)

	_, first, err := apiClient.CreateTemplate(ctx, tplBytes, "sort_first.tpl", shared.FormatHTML, tag+" first")
	require.NoError(t, err, "should create first template")
	require.NotNil(t, first, "first template response should not be nil")

	firstID, ok := first["id"].(string)
	require.True(t, ok, "first template response should contain 'id' string field")
	require.NotEmpty(t, firstID, "first template should have an id")

	// Brief pause to guarantee ordering by createdAt.
	time.Sleep(1 * time.Second)

	_, second, err := apiClient.CreateTemplate(ctx, tplBytes, "sort_second.tpl", shared.FormatHTML, tag+" second")
	require.NoError(t, err, "should create second template")
	require.NotNil(t, second, "second template response should not be nil")

	secondID, ok := second["id"].(string)
	require.True(t, ok, "second template response should contain 'id' string field")
	require.NotEmpty(t, secondID, "second template should have an id")

	// Cleanup: delete both templates after the test.
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = apiClient.DeleteTemplate(cleanupCtx, firstID)
		_, _ = apiClient.DeleteTemplate(cleanupCtx, secondID)
	})

	// List templates filtered by the unique tag — isolates from other parallel tests.
	status, paginated, err := apiClient.GetAllTemplates(ctx, map[string]string{
		"description": tag,
	})
	require.NoError(t, err, "GetAllTemplates should not return an error")
	assert.Equal(t, http.StatusOK, status, "should return 200")
	require.Equal(t, 2, len(paginated.Items), "should have exactly 2 items matching the tag")

	// Find positions of both templates in the list.
	posFirst := -1
	posSecond := -1

	for i, item := range paginated.Items {
		id, _ := item["id"].(string)

		switch id {
		case firstID:
			posFirst = i
		case secondID:
			posSecond = i
		}
	}

	require.NotEqual(t, -1, posFirst, "first template should be in the list")
	require.NotEqual(t, -1, posSecond, "second template should be in the list")

	// In descending order, the second (newer) template should appear before the first (older).
	assert.Less(t, posSecond, posFirst,
		"second (newer) template should appear before first (older) template in desc order")
}
