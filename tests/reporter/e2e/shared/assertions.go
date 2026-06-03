// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package shared

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ############################################################################
// Polling Assertions
// ############################################################################

// AssertReportCompleted polls GET /v1/reports/:id until the report reaches "Finished" status.
// It fails the test fatally if the report transitions to "Error" or the timeout is reached.
// On success, returns the final ReportResponse.
func AssertReportCompleted(t *testing.T, ctx context.Context, client *ManagerClient, reportID string, timeout time.Duration) ReportResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	var (
		lastReport ReportResponse
		lastErr    error
	)

	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				t.Fatalf("report %s did not complete within %v: last error: %v", reportID, timeout, lastErr)
			}

			if lastReport.ID != "" {
				t.Fatalf("report %s did not complete within %v: last status: %s", reportID, timeout, lastReport.Status)
			}

			t.Fatalf("report %s did not complete within %v", reportID, timeout)

			return ReportResponse{}

		case <-ticker.C:
			status, report, err := client.GetReport(ctx, reportID)
			if err != nil {
				lastErr = err

				continue
			}

			if status != http.StatusOK {
				lastErr = nil

				continue
			}

			lastReport = report
			lastErr = nil

			switch report.Status {
			case StatusFinished:
				return report
			case StatusError:
				errMsg := "unknown"
				if report.Metadata != nil {
					if e, ok := report.Metadata["error"]; ok {
						errMsg = fmt.Sprintf("%v", e)
					}
					if d, ok := report.Metadata["error_detail"]; ok {
						errMsg = fmt.Sprintf("%s | detail: %v", errMsg, d)
					}
				}

				t.Fatalf("report %s failed with error: %s", reportID, errMsg)

				return ReportResponse{}
			}
		}
	}
}

// AssertReportFailed polls GET /v1/reports/:id until the report reaches "Error" status.
// It fails the test fatally if the report transitions to "Finished" or the timeout is reached.
// On success, returns the final ReportResponse.
func AssertReportFailed(t *testing.T, ctx context.Context, client *ManagerClient, reportID string, timeout time.Duration) ReportResponse {
	t.Helper()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()

	var (
		lastReport ReportResponse
		lastErr    error
	)

	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				t.Fatalf("report %s did not fail within %v: last error: %v", reportID, timeout, lastErr)
			}

			if lastReport.ID != "" {
				t.Fatalf("report %s did not fail within %v: last status: %s", reportID, timeout, lastReport.Status)
			}

			t.Fatalf("report %s did not fail within %v", reportID, timeout)

			return ReportResponse{}

		case <-ticker.C:
			status, report, err := client.GetReport(ctx, reportID)
			if err != nil {
				lastErr = err

				continue
			}

			if status != http.StatusOK {
				lastErr = nil

				continue
			}

			lastReport = report
			lastErr = nil

			switch report.Status {
			case StatusError:
				return report
			case StatusFinished:
				t.Fatalf("report %s completed unexpectedly (expected failure)", reportID)

				return ReportResponse{}
			}
		}
	}
}

// ############################################################################
// HTTP Assertions
// ############################################################################

// AssertHTTPStatus asserts that the actual HTTP status code matches the expected value.
func AssertHTTPStatus(t *testing.T, got, want int) {
	t.Helper()

	require.Equal(t, want, got, "unexpected HTTP status code")
}

// AssertJSONField asserts that the given key in the response body map has the expected value.
func AssertJSONField(t *testing.T, body map[string]any, key string, want any) {
	t.Helper()

	got, ok := body[key]
	require.True(t, ok, "response body should contain key %q", key)
	assert.Equal(t, want, got, "field %q should have expected value", key)
}

// AssertErrorCode asserts that the response body contains a "code" field matching the expected error code.
func AssertErrorCode(t *testing.T, body map[string]any, expectedCode string) {
	t.Helper()

	code, ok := body["code"]
	require.True(t, ok, "error response should contain 'code' field")
	assert.Equal(t, expectedCode, code, "error code should match")
}

// AssertPagination validates the pagination envelope of a paginated response.
// Checks that Items is not nil and has at least expectMinItems entries.
func AssertPagination(t *testing.T, resp PaginatedResponse, expectMinItems int) {
	t.Helper()

	require.NotNil(t, resp.Items, "paginated response items should not be nil")
	assert.GreaterOrEqual(t, len(resp.Items), expectMinItems, "should have at least %d items", expectMinItems)
	assert.Greater(t, resp.Limit, 0, "limit should be positive")
}

// AssertValidUUID validates that a string is a valid UUID v4 format.
// Expected format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (lowercase hex with dashes).
func AssertValidUUID(t *testing.T, value string) {
	t.Helper()

	assert.Regexp(t, `^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`, value, "should be valid UUID")
}

// ############################################################################
// Content Assertions
// ############################################################################

// AssertContentType asserts that the Content-Type header contains the expected media type.
func AssertContentType(t *testing.T, headers http.Header, expected string) {
	t.Helper()

	ct := headers.Get("Content-Type")
	assert.Contains(t, ct, expected, "Content-Type should contain %q", expected)
}

// pdfMagicBytes is the magic byte sequence at the start of all valid PDF files.
var pdfMagicBytes = []byte("%PDF-")

// AssertPDFContent asserts that the provided data starts with the PDF magic bytes (%PDF-).
func AssertPDFContent(t *testing.T, data []byte) {
	t.Helper()

	require.True(t, len(data) > len(pdfMagicBytes), "PDF data should not be empty")
	assert.True(t, bytes.HasPrefix(data, pdfMagicBytes), "data should start with PDF magic bytes (%%PDF-)")
}

// AssertHTMLContent asserts that the data contains valid HTML structure and the expected strings.
func AssertHTMLContent(t *testing.T, data []byte, mustContain []string) {
	t.Helper()

	content := string(data)
	assert.Contains(t, strings.ToLower(content), "<html", "data should contain HTML opening tag")

	for _, s := range mustContain {
		assert.Contains(t, content, s, "HTML content should contain %q", s)
	}
}

// AssertCSVContent asserts that the data is valid CSV and the header row contains the expected headers.
func AssertCSVContent(t *testing.T, data []byte, expectedHeaders []string) {
	t.Helper()

	reader := csv.NewReader(bytes.NewReader(data))

	headers, err := reader.Read()
	require.NoError(t, err, "should be able to read CSV header row")

	for _, expected := range expectedHeaders {
		assert.Contains(t, headers, expected, "CSV headers should contain %q", expected)
	}
}

// AssertXMLContent asserts that the data is valid, well-formed XML.
func AssertXMLContent(t *testing.T, data []byte) {
	t.Helper()

	decoder := xml.NewDecoder(bytes.NewReader(data))

	for {
		_, err := decoder.Token()
		if err != nil {
			// io.EOF means we successfully parsed all tokens.
			if errors.Is(err, io.EOF) {
				return
			}

			t.Fatalf("data is not valid XML: %v", err)
		}
	}
}

// ############################################################################
// Deadline Assertions
// ############################################################################

// AssertDeadlineFields validates that the response body contains all required deadline fields.
func AssertDeadlineFields(t *testing.T, body map[string]any) {
	t.Helper()

	id, ok := body["id"].(string)
	require.True(t, ok, "response should contain 'id' string field")
	AssertValidUUID(t, id)

	require.Contains(t, body, "name", "response should contain 'name'")
	require.Contains(t, body, "type", "response should contain 'type'")
	require.Contains(t, body, "dueDate", "response should contain 'dueDate'")
	require.Contains(t, body, "frequency", "response should contain 'frequency'")
	require.Contains(t, body, "color", "response should contain 'color'")
	require.Contains(t, body, "status", "response should contain 'status'")
	require.Contains(t, body, "createdAt", "response should contain 'createdAt'")
	require.Contains(t, body, "updatedAt", "response should contain 'updatedAt'")
}

// AssertDeadlineStatus asserts that the deadline status matches the expected value.
func AssertDeadlineStatus(t *testing.T, body map[string]any, expectedStatus string) {
	t.Helper()

	status, ok := body["status"].(string)
	require.True(t, ok, "response should contain 'status' string field")
	assert.Equal(t, expectedStatus, status, "deadline status should be %q", expectedStatus)
}

// AssertValidationResponse validates the template builder validation response.
func AssertValidationResponse(t *testing.T, body map[string]any, expectedValid bool, expectedMinErrors int) {
	t.Helper()

	valid, ok := body["valid"].(bool)
	require.True(t, ok, "response should contain 'valid' bool field")
	assert.Equal(t, expectedValid, valid, "validation 'valid' field should match expected")

	if expectedMinErrors > 0 {
		errs, ok := body["errors"].([]any)
		require.True(t, ok, "response should contain 'errors' array when invalid")
		assert.GreaterOrEqual(t, len(errs), expectedMinErrors, "should have at least %d validation errors", expectedMinErrors)
	}
}

// AssertGeneratedCode validates the template builder generate-code response.
func AssertGeneratedCode(t *testing.T, body map[string]any, expectedContains []string) {
	t.Helper()

	code, ok := body["code"].(string)
	require.True(t, ok, "response should contain 'code' string field")
	require.NotEmpty(t, code, "generated code should not be empty")

	for _, s := range expectedContains {
		assert.Contains(t, code, s, "generated code should contain %q", s)
	}

	_, ok = body["mappedFields"]
	assert.True(t, ok, "response should contain 'mappedFields'")
}

// AssertTXTContent asserts that the data contains all the expected strings.
func AssertTXTContent(t *testing.T, data []byte, mustContain []string) {
	t.Helper()

	content := string(data)

	require.NotEmpty(t, content, "TXT content should not be empty")

	for _, s := range mustContain {
		assert.Contains(t, content, s, "TXT content should contain %q", s)
	}
}
