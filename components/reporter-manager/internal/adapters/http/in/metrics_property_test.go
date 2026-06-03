//go:build property

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/quick"
	"time"

	"github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/reporter/components/manager/internal/services"
	"github.com/LerianStudio/reporter/pkg"
	"github.com/LerianStudio/reporter/pkg/mongodb/report"
	"github.com/LerianStudio/reporter/pkg/mongodb/template"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

// callParseErrorPeriodDays invokes parseErrorPeriodDays through a Fiber handler
// so it receives a valid *fiber.Ctx with proper fasthttp internals. Returns the
// parsed days value and any error. The queryValue parameter is set as the
// "errorPeriodDays" query string; pass empty string for absent parameter.
func callParseErrorPeriodDays(t *testing.T, queryValue string, hasParam bool) (int, error) {
	t.Helper()

	app := fiber.New()

	var (
		resultDays int
		resultErr  error
	)

	app.Get("/test", func(c *fiber.Ctx) error {
		resultDays, resultErr = parseErrorPeriodDays(c)
		return c.SendStatus(fiber.StatusOK)
	})

	url := "/test"
	if hasParam {
		url = fmt.Sprintf("/test?errorPeriodDays=%s", queryValue)
	}

	req := httptest.NewRequest("GET", url, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("fiber app.Test failed: %v", err)
	}

	_ = resp.Body.Close()

	return resultDays, resultErr
}

// TestProperty_ParseErrorPeriodDays_AlwaysReturnsValidRangeOrError verifies that
// for ANY string input, parseErrorPeriodDays either returns an int in [1, 365]
// with nil error, or returns a non-nil error. It NEVER returns a value outside
// this range with a nil error.
func TestProperty_ParseErrorPeriodDays_AlwaysReturnsValidRangeOrError(t *testing.T) {
	t.Parallel()

	property := func(input string) bool {
		// Bound input length to prevent OOM
		if len(input) > 1000 {
			input = input[:1000]
		}

		days, err := callParseErrorPeriodDays(t, input, true)
		if err != nil {
			// Error path: valid outcome regardless of return value
			return true
		}

		// Success path: days MUST be in [1, 365]
		if days < minErrorPeriodDays || days > maxErrorPeriodDays {
			t.Logf("parseErrorPeriodDays returned %d (outside [%d, %d]) with nil error for input %q",
				days, minErrorPeriodDays, maxErrorPeriodDays, input)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: parseErrorPeriodDays returned value outside [1, 365] without error")
}

// TestProperty_ParseErrorPeriodDays_EmptyInputReturnsDefault verifies that when
// the query parameter is absent, the function always returns the default value
// (7 days) with no error.
func TestProperty_ParseErrorPeriodDays_EmptyInputReturnsDefault(t *testing.T) {
	t.Parallel()

	days, err := callParseErrorPeriodDays(t, "", false)

	require.NoError(t, err, "Absent parameter should not produce an error")
	require.Equal(t, defaultErrorPeriodDays, days,
		"Absent parameter should return default value %d, got %d", defaultErrorPeriodDays, days)
}

// TestProperty_ParseErrorPeriodDays_ValidIntegersInRange verifies that for any
// integer in [1, 365], parseErrorPeriodDays returns that exact integer with no
// error. This is the roundtrip property: valid integer string -> parse -> same integer.
func TestProperty_ParseErrorPeriodDays_ValidIntegersInRange(t *testing.T) {
	t.Parallel()

	property := func(n uint16) bool {
		// Map to valid range [1, 365]
		day := int(n%365) + 1
		input := strconv.Itoa(day)

		result, err := callParseErrorPeriodDays(t, input, true)
		if err != nil {
			t.Logf("parseErrorPeriodDays returned error for valid input %q: %v", input, err)
			return false
		}

		if result != day {
			t.Logf("parseErrorPeriodDays returned %d for input %q, expected %d", result, input, day)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: parseErrorPeriodDays rejected valid integer in [1, 365]")
}

// TestProperty_ParseErrorPeriodDays_OutOfRangeIntegersReturnError verifies that
// for any integer outside [1, 365], parseErrorPeriodDays returns an error. This
// covers both negative numbers, zero, and values > 365.
func TestProperty_ParseErrorPeriodDays_OutOfRangeIntegersReturnError(t *testing.T) {
	t.Parallel()

	property := func(n int) bool {
		// Only test values outside [1, 365]
		if n >= minErrorPeriodDays && n <= maxErrorPeriodDays {
			return true // skip valid range
		}

		input := strconv.Itoa(n)

		_, err := callParseErrorPeriodDays(t, input, true)

		if err == nil {
			t.Logf("parseErrorPeriodDays returned nil error for out-of-range input %q", input)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: parseErrorPeriodDays accepted out-of-range integer")
}

// TestProperty_ParseErrorPeriodDays_NonIntegerStringsReturnError verifies that
// any string that is not a valid integer representation always produces an error.
func TestProperty_ParseErrorPeriodDays_NonIntegerStringsReturnError(t *testing.T) {
	t.Parallel()

	property := func(input string) bool {
		if len(input) > 1000 {
			input = input[:1000]
		}

		// Skip empty strings (default path) and valid integers
		if input == "" {
			return true
		}

		if _, parseErr := strconv.Atoi(input); parseErr == nil {
			return true // skip valid integers, tested elsewhere
		}

		_, err := callParseErrorPeriodDays(t, input, true)

		if err == nil {
			t.Logf("parseErrorPeriodDays returned nil error for non-integer input %q", input)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: parseErrorPeriodDays accepted non-integer string")
}

// TestProperty_PeriodCalculation_NoOverlapNoGap verifies that given any valid
// errorPeriodDays value N, the current period and previous period each cover
// exactly N days with no overlap and no gap between them.
func TestProperty_PeriodCalculation_NoOverlapNoGap(t *testing.T) {
	t.Parallel()

	property := func(seed int64) bool {
		// Generate a valid day count in [1, 365]
		rng := rand.New(rand.NewSource(seed))
		periodDays := rng.Intn(maxErrorPeriodDays) + 1

		// Replicate the period calculation from GetMetrics (lines 88-90)
		now := time.Now().UTC()
		currentPeriodStart := now.Add(-time.Duration(periodDays) * hoursPerDay * time.Hour)
		previousPeriodStart := currentPeriodStart.Add(-time.Duration(periodDays) * hoursPerDay * time.Hour)

		// Property 1: Current period covers exactly N days
		currentDuration := now.Sub(currentPeriodStart)
		expectedDuration := time.Duration(periodDays) * hoursPerDay * time.Hour

		if currentDuration != expectedDuration {
			t.Logf("Current period duration mismatch: got %v, expected %v (days=%d)",
				currentDuration, expectedDuration, periodDays)
			return false
		}

		// Property 2: Previous period covers exactly N days
		previousDuration := currentPeriodStart.Sub(previousPeriodStart)

		if previousDuration != expectedDuration {
			t.Logf("Previous period duration mismatch: got %v, expected %v (days=%d)",
				previousDuration, expectedDuration, periodDays)
			return false
		}

		// Property 3: No gap — previous period ends exactly where current begins
		previousPeriodEnd := previousPeriodStart.Add(expectedDuration)

		if !previousPeriodEnd.Equal(currentPeriodStart) {
			t.Logf("Gap detected: previous period ends at %v but current starts at %v (days=%d)",
				previousPeriodEnd, currentPeriodStart, periodDays)
			return false
		}

		// Property 4: No overlap — previous period end <= current period start
		if previousPeriodEnd.After(currentPeriodStart) {
			t.Logf("Overlap detected: previous period ends at %v, current starts at %v (days=%d)",
				previousPeriodEnd, currentPeriodStart, periodDays)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: period calculation has overlap or gap")
}

// TestProperty_PeriodCalculation_TotalSpanIsTwiceN verifies that the combined
// span of current + previous periods always equals exactly 2*N days. This is
// a mathematical invariant of the period calculation.
func TestProperty_PeriodCalculation_TotalSpanIsTwiceN(t *testing.T) {
	t.Parallel()

	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		periodDays := rng.Intn(maxErrorPeriodDays) + 1

		now := time.Now().UTC()
		currentPeriodStart := now.Add(-time.Duration(periodDays) * hoursPerDay * time.Hour)
		previousPeriodStart := currentPeriodStart.Add(-time.Duration(periodDays) * hoursPerDay * time.Hour)

		totalSpan := now.Sub(previousPeriodStart)
		expectedTotalSpan := time.Duration(2*periodDays) * hoursPerDay * time.Hour

		if totalSpan != expectedTotalSpan {
			t.Logf("Total span mismatch: got %v, expected %v (days=%d)",
				totalSpan, expectedTotalSpan, periodDays)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: total period span is not 2*N days")
}

// TestProperty_MetricsResponse_AlwaysCompleteStructure verifies that for any
// valid errorPeriodDays input, the response always contains all 5 fields with
// non-negative values and values match what the repositories return.
func TestProperty_MetricsResponse_AlwaysCompleteStructure(t *testing.T) {
	t.Parallel()

	property := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		periodDays := rng.Intn(maxErrorPeriodDays) + 1

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTemplateRepo := template.NewMockRepository(ctrl)
		mockReportRepo := report.NewMockRepository(ctrl)

		// Generate random non-negative counts
		templateCount := int64(rng.Intn(10000))
		reportCount := int64(rng.Intn(10000))
		currentErrors := int64(rng.Intn(1000))
		previousErrors := int64(rng.Intn(1000))

		mockTemplateRepo.EXPECT().
			CountAll(gomock.Any()).
			Return(templateCount, nil)

		mockReportRepo.EXPECT().
			CountAll(gomock.Any()).
			Return(reportCount, nil)

		mockReportRepo.EXPECT().
			CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
			Return(currentErrors, nil)

		mockReportRepo.EXPECT().
			CountByStatus(gomock.Any(), "Error", gomock.Any(), gomock.Any()).
			Return(previousErrors, nil)

		dsCount := rng.Intn(20)
		dataSources := make(map[string]pkg.DataSource)

		for i := 0; i < dsCount; i++ {
			dataSources[fmt.Sprintf("ds-%d", i)] = pkg.DataSource{}
		}

		safeDatasources := pkg.NewSafeDataSources(dataSources)

		useCase := &services.UseCase{
			Logger:              log.NewNop(),
			Tracer:              noop.NewTracerProvider().Tracer("test"),
			TemplateRepo:        mockTemplateRepo,
			ReportRepo:          mockReportRepo,
			ExternalDataSources: safeDatasources,
		}

		handler, err := NewMetricsHandler(useCase)
		if err != nil {
			t.Logf("Failed to create handler: %v", err)
			return false
		}

		app := fiber.New()
		app.Get("/v1/metrics", setupMetricsContextMiddleware(), handler.GetMetrics)

		url := fmt.Sprintf("/v1/metrics?errorPeriodDays=%d", periodDays)
		req := httptest.NewRequest("GET", url, nil)
		resp, err := app.Test(req, -1)
		if err != nil {
			t.Logf("Request failed: %v", err)
			return false
		}

		if resp.StatusCode != fiber.StatusOK {
			t.Logf("Expected 200, got %d for periodDays=%d", resp.StatusCode, periodDays)
			return false
		}

		var body metricsResponse
		if decodeErr := json.NewDecoder(resp.Body).Decode(&body); decodeErr != nil {
			t.Logf("Failed to decode response: %v", decodeErr)
			return false
		}

		// All fields must be non-negative
		if body.Templates < 0 || body.Reports < 0 || body.DataSources < 0 ||
			body.Errors.Total < 0 || body.Errors.PreviousPeriodTotal < 0 {
			t.Logf("Negative value detected in response: %+v", body)
			return false
		}

		// Verify values match what we provided via mocks
		if body.Templates != templateCount {
			t.Logf("Templates mismatch: got %d, expected %d", body.Templates, templateCount)
			return false
		}

		if body.Reports != reportCount {
			t.Logf("Reports mismatch: got %d, expected %d", body.Reports, reportCount)
			return false
		}

		if body.DataSources != int64(dsCount) {
			t.Logf("DataSources mismatch: got %d, expected %d", body.DataSources, dsCount)
			return false
		}

		if body.Errors.Total != currentErrors {
			t.Logf("Errors.Total mismatch: got %d, expected %d", body.Errors.Total, currentErrors)
			return false
		}

		if body.Errors.PreviousPeriodTotal != previousErrors {
			t.Logf("Errors.PreviousPeriodTotal mismatch: got %d, expected %d",
				body.Errors.PreviousPeriodTotal, previousErrors)
			return false
		}

		return true
	}

	err := quick.Check(property, &quick.Config{MaxCount: 100})
	require.NoError(t, err, "Property violated: metrics response missing fields or has negative values")
}
