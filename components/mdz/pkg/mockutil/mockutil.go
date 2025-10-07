// Package mockutil provides testing utilities for the MDZ CLI.
//
// This package contains helper functions for creating mock HTTP responses
// from JSON files, enabling comprehensive unit testing of CLI commands
// without making actual API calls.
package mockutil

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/jarcoal/httpmock"
)

// loadJSONResponse reads a JSON file and returns its contents.
//
// Parameters:
//   - filename: Path to JSON file
//
// Returns:
//   - []byte: File contents
//   - error: Error if file cannot be read
func loadJSONResponse(filename string) ([]byte, error) {
	filename = filepath.Clean(filename)
	return os.ReadFile(filename)
}

// MockResponseFromFile creates an httpmock responder from a JSON file.
//
// This function is used in tests to mock API responses:
// 1. Reads JSON file
// 2. Creates httpmock responder with specified status code
// 3. Returns responder for use with httpmock
//
// Parameters:
//   - status: HTTP status code for the mock response
//   - path: Path to JSON file containing response body
//
// Returns:
//   - httpmock.Responder: Mock responder for testing
func MockResponseFromFile(status int, path string) httpmock.Responder {
	data, err := loadJSONResponse(path)
	if err != nil {
		return httpmock.NewStringResponder(http.StatusInternalServerError,
			"Failed to load mock response")
	}

	return httpmock.NewBytesResponder(status, data)
}
