package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBaseError(t *testing.T) {
	tests := []struct {
		name     string
		baseErr  BaseError
		expected string
	}{
		{
			name: "with code",
			baseErr: BaseError{
				ErrorType: ErrorTypeAPI,
				Message:   "Something went wrong",
				Code:      "ERR123",
			},
			expected: "ERR123 - Something went wrong",
		},
		{
			name: "without code",
			baseErr: BaseError{
				ErrorType: ErrorTypeAPI,
				Message:   "Something went wrong",
			},
			expected: "Something went wrong",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.baseErr.Error() != tc.expected {
				t.Errorf("Expected error message '%s', got '%s'", tc.expected, tc.baseErr.Error())
			}

			if tc.baseErr.Type() != tc.baseErr.ErrorType {
				t.Errorf("Expected error type '%s', got '%s'", tc.baseErr.ErrorType, tc.baseErr.Type())
			}
		})
	}
}

func TestAPIError(t *testing.T) {
	tests := []struct {
		name       string
		apiErr     *APIError
		expected   string
		statusCode int
		requestID  string
	}{
		{
			name: "with request ID",
			apiErr: NewAPIError(
				http.StatusBadRequest,
				"Invalid input",
				"req-123",
				"INVALID_INPUT",
				nil,
			),
			expected:   "API error (status: 400, request_id: req-123): INVALID_INPUT - Invalid input",
			statusCode: http.StatusBadRequest,
			requestID:  "req-123",
		},
		{
			name: "without request ID",
			apiErr: NewAPIError(
				http.StatusInternalServerError,
				"Server error",
				"",
				"SERVER_ERROR",
				nil,
			),
			expected:   "API error (status: 500): SERVER_ERROR - Server error",
			statusCode: http.StatusInternalServerError,
			requestID:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.apiErr.Error() != tc.expected {
				t.Errorf("Expected error message '%s', got '%s'", tc.expected, tc.apiErr.Error())
			}

			if tc.apiErr.StatusCode != tc.statusCode {
				t.Errorf("Expected status code %d, got %d", tc.statusCode, tc.apiErr.StatusCode)
			}

			if tc.apiErr.RequestID != tc.requestID {
				t.Errorf("Expected request ID '%s', got '%s'", tc.requestID, tc.apiErr.RequestID)
			}

			if tc.apiErr.Type() != ErrorTypeAPI {
				t.Errorf("Expected error type '%s', got '%s'", ErrorTypeAPI, tc.apiErr.Type())
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	tests := []struct {
		name       string
		validErr   *ValidationError
		expected   string
		fields     map[string]string
		entityType string
	}{
		{
			name: "with fields",
			validErr: NewValidationError(
				"Validation failed",
				"VALIDATION_ERROR",
				"User",
				map[string]string{"name": "required", "email": "invalid format"},
				nil,
			),
			expected:   "Validation error: VALIDATION_ERROR - Validation failed (Invalid fields: map[email:invalid format name:required])",
			fields:     map[string]string{"name": "required", "email": "invalid format"},
			entityType: "User",
		},
		{
			name: "without fields",
			validErr: NewValidationError(
				"Validation failed",
				"VALIDATION_ERROR",
				"User",
				nil,
				nil,
			),
			expected:   "Validation error: VALIDATION_ERROR - Validation failed",
			fields:     nil,
			entityType: "User",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.validErr.Error() != tc.expected {
				t.Errorf("Expected error message '%s', got '%s'", tc.expected, tc.validErr.Error())
			}

			if tc.validErr.EntityType != tc.entityType {
				t.Errorf("Expected entity type '%s', got '%s'", tc.entityType, tc.validErr.EntityType)
			}

			if len(tc.validErr.Fields) != len(tc.fields) {
				t.Errorf("Expected %d fields, got %d", len(tc.fields), len(tc.validErr.Fields))
			}

			if tc.validErr.Type() != ErrorTypeValidation {
				t.Errorf("Expected error type '%s', got '%s'", ErrorTypeValidation, tc.validErr.Type())
			}
		})
	}
}

func TestEntityNotFoundError(t *testing.T) {
	tests := []struct {
		name          string
		notFoundErr   *EntityNotFoundError
		expected      string
		entityType    string
		expectedTitle string
	}{
		{
			name: "with entity type",
			notFoundErr: NewEntityNotFoundError(
				"User not found",
				"USER_NOT_FOUND",
				"User",
			),
			expected:      "USER_NOT_FOUND - User not found",
			entityType:    "User",
			expectedTitle: "User Not Found",
		},
		{
			name: "without entity type",
			notFoundErr: NewEntityNotFoundError(
				"Entity not found",
				"ENTITY_NOT_FOUND",
				"",
			),
			expected:      "ENTITY_NOT_FOUND - Entity not found",
			entityType:    "",
			expectedTitle: "Entity Not Found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.notFoundErr.Error() != tc.expected {
				t.Errorf("Expected error message '%s', got '%s'", tc.expected, tc.notFoundErr.Error())
			}

			if tc.notFoundErr.EntityType != tc.entityType {
				t.Errorf("Expected entity type '%s', got '%s'", tc.entityType, tc.notFoundErr.EntityType)
			}

			if tc.notFoundErr.Title != tc.expectedTitle {
				t.Errorf("Expected title '%s', got '%s'", tc.expectedTitle, tc.notFoundErr.Title)
			}

			if tc.notFoundErr.Type() != ErrorTypeEntityNotFound {
				t.Errorf("Expected error type '%s', got '%s'", ErrorTypeEntityNotFound, tc.notFoundErr.Type())
			}
		})
	}
}

func TestEntityConflictError(t *testing.T) {
	tests := []struct {
		name          string
		conflictErr   *EntityConflictError
		expected      string
		entityType    string
		expectedTitle string
	}{
		{
			name: "with entity type",
			conflictErr: NewEntityConflictError(
				"User already exists",
				"USER_CONFLICT",
				"User",
			),
			expected:      "USER_CONFLICT - User already exists",
			entityType:    "User",
			expectedTitle: "User Conflict",
		},
		{
			name: "without entity type",
			conflictErr: NewEntityConflictError(
				"Entity conflict",
				"ENTITY_CONFLICT",
				"",
			),
			expected:      "ENTITY_CONFLICT - Entity conflict",
			entityType:    "",
			expectedTitle: "Entity Conflict",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.conflictErr.Error() != tc.expected {
				t.Errorf("Expected error message '%s', got '%s'", tc.expected, tc.conflictErr.Error())
			}

			if tc.conflictErr.EntityType != tc.entityType {
				t.Errorf("Expected entity type '%s', got '%s'", tc.entityType, tc.conflictErr.EntityType)
			}

			if tc.conflictErr.Title != tc.expectedTitle {
				t.Errorf("Expected title '%s', got '%s'", tc.expectedTitle, tc.conflictErr.Title)
			}

			if tc.conflictErr.Type() != ErrorTypeEntityConflict {
				t.Errorf("Expected error type '%s', got '%s'", ErrorTypeEntityConflict, tc.conflictErr.Type())
			}
		})
	}
}

func TestAuthenticationError(t *testing.T) {
	authErr := NewAuthenticationError(
		"Invalid credentials",
		"INVALID_CREDENTIALS",
	)

	expected := "INVALID_CREDENTIALS - Invalid credentials"
	if authErr.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, authErr.Error())
	}

	if authErr.Type() != ErrorTypeAuthentication {
		t.Errorf("Expected error type '%s', got '%s'", ErrorTypeAuthentication, authErr.Type())
	}

	if authErr.Title != "Authentication Error" {
		t.Errorf("Expected title 'Authentication Error', got '%s'", authErr.Title)
	}
}

func TestAuthorizationError(t *testing.T) {
	authzErr := NewAuthorizationError(
		"Insufficient permissions",
		"INSUFFICIENT_PERMISSIONS",
	)

	expected := "INSUFFICIENT_PERMISSIONS - Insufficient permissions"
	if authzErr.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, authzErr.Error())
	}

	if authzErr.Type() != ErrorTypeAuthorization {
		t.Errorf("Expected error type '%s', got '%s'", ErrorTypeAuthorization, authzErr.Type())
	}

	if authzErr.Title != "Authorization Error" {
		t.Errorf("Expected title 'Authorization Error', got '%s'", authzErr.Title)
	}
}

func TestUnprocessableOperationError(t *testing.T) {
	unprocessableErr := NewUnprocessableOperationError(
		"Cannot process this operation",
		"UNPROCESSABLE_OPERATION",
		"Transaction",
	)

	expected := "UNPROCESSABLE_OPERATION - Cannot process this operation"
	if unprocessableErr.Error() != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, unprocessableErr.Error())
	}

	if unprocessableErr.Type() != ErrorTypeUnprocessableOperation {
		t.Errorf("Expected error type '%s', got '%s'", ErrorTypeUnprocessableOperation, unprocessableErr.Type())
	}

	if unprocessableErr.Title != "Unprocessable Operation" {
		t.Errorf("Expected title 'Unprocessable Operation', got '%s'", unprocessableErr.Title)
	}

	if unprocessableErr.EntityType != "Transaction" {
		t.Errorf("Expected entity type 'Transaction', got '%s'", unprocessableErr.EntityType)
	}
}

func TestNetworkError(t *testing.T) {
	originalErr := errors.New("connection refused")
	networkErr := NewNetworkError(originalErr)

	if networkErr.Message != "Network error: connection refused" {
		t.Errorf("Expected message 'Network error: connection refused', got '%s'", networkErr.Message)
	}

	if !errors.Is(networkErr.Err, originalErr) {
		t.Errorf("Expected original error to be preserved")
	}

	if networkErr.Type() != ErrorTypeNetwork {
		t.Errorf("Expected error type '%s', got '%s'", ErrorTypeNetwork, networkErr.Type())
	}

	if networkErr.Title != "Network Error" {
		t.Errorf("Expected title 'Network Error', got '%s'", networkErr.Title)
	}

	if !errors.Is(networkErr.Err, originalErr) {
		t.Errorf("Expected wrapped error to be the original error")
	}
}

func TestTimeoutError(t *testing.T) {
	timeoutErr := NewTimeoutError("30s")

	expected := "Request timed out after 30s"
	if timeoutErr.Message != expected {
		t.Errorf("Expected error message '%s', got '%s'", expected, timeoutErr.Message)
	}

	if timeoutErr.Type() != ErrorTypeTimeout {
		t.Errorf("Expected error type '%s', got '%s'", ErrorTypeTimeout, timeoutErr.Type())
	}

	if timeoutErr.Title != "Timeout Error" {
		t.Errorf("Expected title 'Timeout Error', got '%s'", timeoutErr.Title)
	}

	if timeoutErr.Duration != "30s" {
		t.Errorf("Expected duration '30s', got '%s'", timeoutErr.Duration)
	}
}

func TestInternalError(t *testing.T) {
	originalErr := errors.New("something went wrong internally")
	internalErr := NewInternalError(originalErr)

	if internalErr.Message != "Internal error: something went wrong internally" {
		t.Errorf("Expected message 'Internal error: something went wrong internally', got '%s'", internalErr.Message)
	}

	if !errors.Is(internalErr.Err, originalErr) {
		t.Errorf("Expected original error to be preserved")
	}

	if internalErr.Type() != ErrorTypeInternal {
		t.Errorf("Expected error type '%s', got '%s'", ErrorTypeInternal, internalErr.Type())
	}

	if internalErr.Title != "Internal Error" {
		t.Errorf("Expected title 'Internal Error', got '%s'", internalErr.Title)
	}

	if !errors.Is(internalErr.Err, originalErr) {
		t.Errorf("Expected wrapped error to be the original error")
	}
}

func TestErrorFromResponse(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		requestID      string
		expectedType   ErrorType
		expectedFields map[string]string
	}{
		{
			name:         "unauthorized error",
			statusCode:   http.StatusUnauthorized,
			responseBody: `{"message":"Invalid credentials","code":"INVALID_CREDENTIALS"}`,
			requestID:    "req-123",
			expectedType: ErrorTypeAuthentication,
		},
		{
			name:         "forbidden error",
			statusCode:   http.StatusForbidden,
			responseBody: `{"message":"Insufficient permissions","code":"INSUFFICIENT_PERMISSIONS"}`,
			requestID:    "req-123",
			expectedType: ErrorTypeAuthorization,
		},
		{
			name:         "not found error",
			statusCode:   http.StatusNotFound,
			responseBody: `{"message":"User not found","code":"USER_NOT_FOUND","entityType":"User"}`,
			requestID:    "req-123",
			expectedType: ErrorTypeEntityNotFound,
		},
		{
			name:         "conflict error",
			statusCode:   http.StatusConflict,
			responseBody: `{"message":"User already exists","code":"USER_CONFLICT","entityType":"User"}`,
			requestID:    "req-123",
			expectedType: ErrorTypeEntityConflict,
		},
		{
			name:         "bad request error",
			statusCode:   http.StatusBadRequest,
			responseBody: `{"message":"Validation failed","code":"VALIDATION_ERROR","entityType":"User","fields":{"name":"required","email":"invalid format"}}`,
			requestID:    "req-123",
			expectedType: ErrorTypeValidation,
			expectedFields: map[string]string{
				"name":  "required",
				"email": "invalid format",
			},
		},
		{
			name:         "unprocessable entity error",
			statusCode:   http.StatusUnprocessableEntity,
			responseBody: `{"message":"Cannot process this operation","code":"UNPROCESSABLE_OPERATION","entityType":"Transaction"}`,
			requestID:    "req-123",
			expectedType: ErrorTypeUnprocessableOperation,
		},
		{
			name:         "internal server error",
			statusCode:   http.StatusInternalServerError,
			responseBody: `{"message":"Internal server error","code":"INTERNAL_ERROR"}`,
			requestID:    "req-123",
			expectedType: ErrorTypeAPI,
		},
		{
			name:         "unparseable response",
			statusCode:   http.StatusInternalServerError,
			responseBody: `not valid json`,
			requestID:    "req-123",
			expectedType: ErrorTypeAPI,
		},
		{
			name:         "empty response",
			statusCode:   http.StatusInternalServerError,
			responseBody: ``,
			requestID:    "req-123",
			expectedType: ErrorTypeAPI,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock response
			resp := &http.Response{
				StatusCode: tc.statusCode,
				Header:     http.Header{},
			}
			resp.Header.Set("X-Request-Id", tc.requestID)

			// Call the function
			err := ErrorFromResponse(resp, []byte(tc.responseBody))

			// Check the error type
			if err.Type() != tc.expectedType {
				t.Errorf("Expected error type '%s', got '%s'", tc.expectedType, err.Type())
			}

			// Check fields for validation errors
			if tc.expectedType == ErrorTypeValidation {
				validErr, ok := err.(*ValidationError)
				if !ok {
					t.Fatalf("Expected ValidationError, got %T", err)
				}

				if len(validErr.Fields) != len(tc.expectedFields) {
					t.Errorf("Expected %d fields, got %d", len(tc.expectedFields), len(validErr.Fields))
				}

				for k, v := range tc.expectedFields {
					if validErr.Fields[k] != v {
						t.Errorf("Expected field '%s' to have value '%s', got '%s'", k, v, validErr.Fields[k])
					}
				}
			}
		})
	}
}

func TestErrorFromResponseWithServer(t *testing.T) {
	tests := []struct {
		name         string
		handler      http.HandlerFunc
		expectedType ErrorType
	}{
		{
			name: "unauthorized error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Id", "req-123")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{
					"message": "Invalid credentials",
					"code":    "INVALID_CREDENTIALS",
				})
			},
			expectedType: ErrorTypeAuthentication,
		},
		{
			name: "forbidden error",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Id", "req-123")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{
					"message": "Insufficient permissions",
					"code":    "INSUFFICIENT_PERMISSIONS",
				})
			},
			expectedType: ErrorTypeAuthorization,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test server
			server := httptest.NewServer(tc.handler)
			defer server.Close()

			// Make a request to the test server
			resp, err := http.Get(server.URL)
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			// Read the response body
			var body []byte
			resp.Body.Read(body)

			// Call the function
			apiErr := ErrorFromResponse(resp, body)

			// Check the error type
			if apiErr.Type() != tc.expectedType {
				t.Errorf("Expected error type '%s', got '%s'", tc.expectedType, apiErr.Type())
			}
		})
	}
}

func TestErrorTypeConstants(t *testing.T) {
	// Test that all error type constants are defined
	errorTypes := []struct {
		name     string
		errType  ErrorType
		expected string
	}{
		{"API", ErrorTypeAPI, "api_error"},
		{"Validation", ErrorTypeValidation, "validation_error"},
		{"Authentication", ErrorTypeAuthentication, "authentication_error"},
		{"Authorization", ErrorTypeAuthorization, "authorization_error"},
		{"EntityNotFound", ErrorTypeEntityNotFound, "entity_not_found_error"},
		{"EntityConflict", ErrorTypeEntityConflict, "entity_conflict_error"},
		{"UnprocessableOperation", ErrorTypeUnprocessableOperation, "unprocessable_operation_error"},
		{"Network", ErrorTypeNetwork, "network_error"},
		{"Timeout", ErrorTypeTimeout, "timeout_error"},
		{"Internal", ErrorTypeInternal, "internal_error"},
	}

	for _, tc := range errorTypes {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.errType) != tc.expected {
				t.Errorf("Expected error type '%s', got '%s'", tc.expected, tc.errType)
			}
		})
	}
}

func TestErrorInterface(t *testing.T) {
	// Test that all error types implement the Error interface
	var _ Error = &APIError{}
	var _ Error = &ValidationError{}
	var _ Error = &EntityNotFoundError{}
	var _ Error = &EntityConflictError{}
	var _ Error = &AuthenticationError{}
	var _ Error = &AuthorizationError{}
	var _ Error = &UnprocessableOperationError{}
	var _ Error = &NetworkError{}
	var _ Error = &TimeoutError{}
	var _ Error = &InternalError{}
}
