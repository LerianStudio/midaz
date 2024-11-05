package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// APIError struct to represent the error received
type APIError struct {
	Title   string            `json:"title"`
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// formatAPIError function that transforms the JSON error into an error type with customized formatting
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
