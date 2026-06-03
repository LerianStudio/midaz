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

	"github.com/LerianStudio/reporter/tests/e2e/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ############################################################################
// Validation Tests — Error Scenarios
// ############################################################################

func TestDeadline_ValidationMissingRequiredFields(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Missing name (empty)
	resp, err := apiClient.CreateDeadlineRaw(ctx, map[string]any{
		"name":      "",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDate(),
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode(), "empty name should return 400")
}

func TestDeadline_ValidationInvalidType(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.CreateDeadline(ctx, map[string]any{
		"name":      shared.UniqueID("dl") + " invalid type",
		"type":      "invalid_type",
		"dueDate":   futureDate(),
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status, "invalid type should return 400")
	shared.AssertErrorCode(t, body, "TPL-0046")
}

func TestDeadline_ValidationInvalidFrequency(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.CreateDeadline(ctx, map[string]any{
		"name":      shared.UniqueID("dl") + " invalid freq",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDate(),
		"frequency": "biweekly",
		"color":     "#AABBCC",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status, "invalid frequency should return 400")
	shared.AssertErrorCode(t, body, "TPL-0047")
}

func TestDeadline_ValidationInvalidColor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		color string
	}{
		{"plain text", "red"},
		{"missing hash", "AABBCC"},
		{"short hex", "#ABC"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			status, body, err := apiClient.CreateDeadline(ctx, map[string]any{
				"name":      shared.UniqueID("dl") + " color " + tc.name,
				"type":      shared.DeadlineTypeCustom,
				"dueDate":   futureDate(),
				"frequency": shared.FrequencyOnce,
				"color":     tc.color,
			})
			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, status, "invalid color %q should return 400", tc.color)
			shared.AssertErrorCode(t, body, "TPL-0048")
		})
	}
}

func TestDeadline_ValidationDueDateInPast(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pastDate := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)

	status, body, err := apiClient.CreateDeadline(ctx, map[string]any{
		"name":      shared.UniqueID("dl") + " past date",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   pastDate,
		"frequency": shared.FrequencyOnce,
		"color":     "#AABBCC",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status, "past due date should return 400")
	shared.AssertErrorCode(t, body, "TPL-0055")
}

func TestDeadline_ValidationMonthsOfYearOutOfRange(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.CreateDeadline(ctx, map[string]any{
		"name":         shared.UniqueID("dl") + " month 13",
		"type":         shared.DeadlineTypeCustom,
		"dueDate":      futureDate(),
		"frequency":    shared.FrequencyAnnual,
		"monthsOfYear": []int{13},
		"color":        "#AABBCC",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status, "month 13 should return 400")
	shared.AssertErrorCode(t, body, "TPL-0054")
}

func TestDeadline_ValidationSemiannualMissingMonthsOfYear(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.CreateDeadline(ctx, map[string]any{
		"name":      shared.UniqueID("dl") + " semi no months",
		"type":      shared.DeadlineTypeCustom,
		"dueDate":   futureDate(),
		"frequency": shared.FrequencySemiannual,
		"color":     "#AABBCC",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status, "semiannual without monthsOfYear should return 400")
	shared.AssertErrorCode(t, body, "TPL-0052")
}

func TestDeadline_ValidationAnnualWrongMonthCount(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.CreateDeadline(ctx, map[string]any{
		"name":         shared.UniqueID("dl") + " annual 2 months",
		"type":         shared.DeadlineTypeCustom,
		"dueDate":      futureDate(),
		"frequency":    shared.FrequencyAnnual,
		"monthsOfYear": []int{3, 6},
		"color":        "#AABBCC",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status, "annual with 2 months should return 400")
	shared.AssertErrorCode(t, body, "TPL-0056")
}

func TestDeadline_ValidationMonthsOfYearNotApplicable(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.CreateDeadline(ctx, map[string]any{
		"name":         shared.UniqueID("dl") + " weekly with months",
		"type":         shared.DeadlineTypeCustom,
		"dueDate":      futureDate(),
		"frequency":    shared.FrequencyWeekly,
		"monthsOfYear": []int{1, 6},
		"color":        "#AABBCC",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, status, "monthsOfYear with weekly should return 400")
	shared.AssertErrorCode(t, body, "TPL-0050")
}

func TestDeadline_ValidationTemplateNotFound(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, body, err := apiClient.CreateDeadline(ctx, map[string]any{
		"name":       shared.UniqueID("dl") + " bad template",
		"type":       shared.DeadlineTypeCustom,
		"dueDate":    futureDate(),
		"frequency":  shared.FrequencyOnce,
		"color":      "#AABBCC",
		"templateId": "00000000-0000-0000-0000-000000000000",
	})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, status, "non-existent template should return 404")

	// Check error response has code field
	_, hasCode := body["code"]
	if hasCode {
		shared.AssertErrorCode(t, body, "TPL-0011")
	}
}

func TestDeadline_ValidationNotFoundOnGet(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Try to update a non-existent deadline
	resp, err := apiClient.UpdateDeadlineRaw(ctx, "00000000-0000-0000-0000-000000000000", map[string]any{
		"name": "does not exist",
	})
	require.NoError(t, err)

	// Should return 404
	assert.Equal(t, http.StatusNotFound, resp.StatusCode(), "non-existent deadline should return 404")

	var body map[string]any
	if err := json.Unmarshal(resp.Body(), &body); err == nil {
		if _, hasCode := body["code"]; hasCode {
			shared.AssertErrorCode(t, body, "TPL-0011")
		}
	}
}
