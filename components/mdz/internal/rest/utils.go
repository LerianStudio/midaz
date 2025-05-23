package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/LerianStudio/midaz/components/mdz/pkg/errors"
)

// APIError struct to represent the error received
type APIError struct {
	Title   string            `json:"title"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// formatAPIError function that transforms the JSON error into an error type with customized formatting
func formatAPIError(jsonData []byte, statusCode int) error {
	var apiError APIError

	err := json.Unmarshal(jsonData, &apiError)
	if err != nil {
		// If we can't parse the error, create a generic one
		return errors.FromHTTPResponse(statusCode, string(jsonData))
	}

	// Create enhanced error from API response
	enhancedErr := errors.FromHTTPResponse(statusCode, apiError.Message)

	// Add field-specific errors as context
	for field, desc := range apiError.Fields {
		_ = enhancedErr.WithContext(field, desc)
		_ = enhancedErr.WithSuggestions(fmt.Sprintf("Check field '%s': %s", field, desc))
	}

	return enhancedErr
}

func checkResponse(resp *http.Response, statusCode int) error {
	if resp.StatusCode != statusCode {
		if resp.StatusCode == http.StatusUnauthorized {
			return errors.New(errors.ErrorTypeAuth, "unauthorized: invalid credentials").
				WithSuggestions("Check your credentials", "Run 'mdz login' to authenticate")
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return errors.Wrap(err, errors.ErrorTypeNetwork, "failed to read response body")
		}
		defer resp.Body.Close()

		return formatAPIError(bodyBytes, resp.StatusCode)
	}

	return nil
}

// BuildPaginatedURL builds a URL with pagination parameters and common filters
func BuildPaginatedURL(baseURL string, limit, page int, sortOrder, startDate, endDate string) (string, error) {
	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return "", errors.Wrap(err, errors.ErrorTypeInternal, "parsing base URL")
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
