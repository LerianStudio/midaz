// Package rest provides REST API client implementations for the MDZ CLI.
// This file contains utility functions for HTTP request handling and error formatting.
package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// APIError represents an error response from the Midaz API.
//
// This struct maps to the standard error format returned by Midaz services,
// allowing the CLI to parse and display user-friendly error messages.
type APIError struct {
	Title   string            `json:"title"`            // Error title
	Code    string            `json:"code"`             // Error code (e.g., "0001")
	Message string            `json:"message"`          // Detailed error message
	Fields  map[string]string `json:"fields,omitempty"` // Field-specific validation errors
}

// formatAPIError transforms API error JSON into a formatted error message.
//
// This function:
// 1. Unmarshals JSON error response
// 2. Formats error with code, title, and message
// 3. Adds field-specific errors if present
// 4. Returns formatted error for display
//
// Parameters:
//   - jsonData: JSON error response from API
//
// Returns:
//   - error: Formatted error message
func formatAPIError(jsonData []byte) error {
	var apiError APIError

	err := json.Unmarshal(jsonData, &apiError)
	if err != nil {
		return errors.New("failed to parse error JSON")
	}

	// Format the main error message
	formattedError := fmt.Sprintf("Error %s: %s\nMessage: %s",
		apiError.Code, apiError.Title, apiError.Message)

	// Check for fields in “Fields” before adding
	if len(apiError.Fields) > 0 {
		formattedError += "\n\nFields:"
		for field, desc := range apiError.Fields {
			formattedError += fmt.Sprintf("\n- %s: %s", field, desc)
		}
	}

	return errors.New(formattedError)
}

// checkResponse validates HTTP response status code and formats errors.
//
// This function:
// 1. Checks if response status matches expected status
// 2. Handles 401 Unauthorized with specific message
// 3. Reads and formats API error for other status codes
// 4. Returns nil if status matches expected
//
// Parameters:
//   - resp: HTTP response to validate
//   - statusCode: Expected HTTP status code
//
// Returns:
//   - error: nil if status matches, formatted error otherwise
func checkResponse(resp *http.Response, statusCode int) error {
	if resp.StatusCode != statusCode {
		if resp.StatusCode == http.StatusUnauthorized {
			return errors.New("unauthorized: invalid credentials")
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return errors.New("failed to read response body: " + err.Error())
		}
		defer resp.Body.Close()

		return formatAPIError(bodyBytes)
	}

	return nil
}

// BuildPaginatedURL constructs a URL with pagination and filter query parameters.
//
// This function builds URLs for list operations with support for:
//   - Pagination: limit and page parameters
//   - Sorting: sort_order parameter (asc/desc)
//   - Date filtering: start_date and end_date parameters
//
// Parameters:
//   - baseURL: Base API endpoint URL
//   - limit: Maximum number of items per page
//   - page: Page number (1-indexed)
//   - sortOrder: Sort direction ("asc" or "desc")
//   - startDate: Start date filter (YYYY-MM-DD)
//   - endDate: End date filter (YYYY-MM-DD)
//
// Returns:
//   - string: Complete URL with query parameters
//   - error: URL parsing error
func BuildPaginatedURL(baseURL string, limit, page int, sortOrder, startDate, endDate string) (string, error) {
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return "", errors.New("parsing base URL: " + err.Error())
	}

	query := reqURL.Query()
	query.Set("limit", strconv.Itoa(limit))
	query.Set("page", strconv.Itoa(page))

	if sortOrder != "" {
		query.Set("sort_order", sortOrder)
	}

	if startDate != "" {
		query.Set("start_date", startDate)
	}

	if endDate != "" {
		query.Set("end_date", endDate)
	}

	reqURL.RawQuery = query.Encode()

	return reqURL.String(), nil
}
