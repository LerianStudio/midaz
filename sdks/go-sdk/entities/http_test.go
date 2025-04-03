package entities

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/internal/utils"
	"github.com/stretchr/testify/assert"
)

// TestHTTPClient_SendRequest tests the HTTP client's request sending functionality
func TestHTTPClient_SendRequest(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		assert.Equal(t, "midaz-go-sdk-test/1.0", r.Header.Get("User-Agent"))

		// Read body if it exists
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)

			if len(body) > 0 {
				var data map[string]interface{}
				json.Unmarshal(body, &data)
				assert.Equal(t, "test", data["key"])
			}
		}

		// Return successful response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"result": "success"})
	}))

	defer server.Close()

	// Create client
	client := newHTTPClient(http.DefaultClient, "test-token", "midaz-go-sdk-test/1.0", false)

	// Create test request
	body := map[string]string{"key": "test"}
	bodyBytes, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(bodyBytes))

	// Test response
	var response map[string]string
	err := client.sendRequest(req, &response)

	// Assertions
	assert.NoError(t, err)
	assert.Equal(t, "success", response["result"])
}

// TestHTTPClient_SendRequest_ErrorResponse tests error handling in HTTP requests
func TestHTTPClient_SendRequest_ErrorResponse(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
		errorBody  map[string]string
		errorCode  utils.ErrorCode
	}{
		{
			name:       "Not Found Error",
			statusCode: http.StatusNotFound,
			errorBody:  map[string]string{"error": "not_found", "message": "Organization not found"},
			errorCode:  utils.CodeNotFound,
		},
		{
			name:       "Validation Error",
			statusCode: http.StatusBadRequest,
			errorBody:  map[string]string{"code": string(utils.CodeValidation), "message": "Invalid input"},
			errorCode:  utils.CodeValidation,
		},
		{
			name:       "Authentication Error",
			statusCode: http.StatusUnauthorized,
			errorBody:  map[string]string{"error": "unauthorized", "message": "Invalid token"},
			errorCode:  utils.CodeAuthentication,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				json.NewEncoder(w).Encode(tc.errorBody)
			}))

			defer server.Close()

			// Create client
			client := newHTTPClient(http.DefaultClient, "test-token", "midaz-go-sdk-test/1.0", false)

			// Create test request
			req, _ := http.NewRequest(http.MethodGet, server.URL, nil)

			// Test response
			var response map[string]string
			err := client.sendRequest(req, &response)

			// Assertions
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.errorBody["message"])

			// Check error type
			sdkErr, ok := err.(*utils.MidazError)
			assert.True(t, ok, "Error should be of type *utils.MidazError")
			assert.Equal(t, tc.errorCode, sdkErr.Code, "Error code should match expected code")
		})
	}
}

// TestHTTPClient_SetCommonHeaders tests the setting of common HTTP headers
func TestHTTPClient_SetCommonHeaders(t *testing.T) {
	client := newHTTPClient(http.DefaultClient, "test-token", "midaz-go-sdk-test/1.0", false)

	// Test setting headers on a new request
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	client.setCommonHeaders(req)

	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
	assert.Equal(t, "midaz-go-sdk-test/1.0", req.Header.Get("User-Agent"))

	// Test not overriding existing headers
	req, _ = http.NewRequest(http.MethodGet, "https://example.com", nil)
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Authorization", "Custom Auth")
	req.Header.Set("User-Agent", "Custom Agent")

	client.setCommonHeaders(req)

	assert.Equal(t, "text/plain", req.Header.Get("Content-Type"))
	assert.Equal(t, "Custom Auth", req.Header.Get("Authorization"))
	assert.Equal(t, "Custom Agent", req.Header.Get("User-Agent"))
}

// TestHTTPClient_HandleErrorResponse tests error response handling
func TestHTTPClient_HandleErrorResponse(t *testing.T) {
	client := newHTTPClient(http.DefaultClient, "test-token", "midaz-go-sdk-test/1.0", false)

	// Create a test response with an error
	body := map[string]string{
		"error":   "test_error",
		"code":    string(utils.CodeValidation),
		"message": "Validation failed",
	}
	bodyBytes, _ := json.Marshal(body)

	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(bytes.NewReader(bodyBytes)),
		Status:     "400 Bad Request",
	}

	// Test handling error
	err := client.handleErrorResponse(resp)

	// Assertions
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Validation failed")

	// Check error type
	sdkErr, ok := err.(*utils.MidazError)
	assert.True(t, ok, "Error should be of type *utils.MidazError")
	assert.Equal(t, utils.CodeValidation, sdkErr.Code, "Error code should match validation code")
}
