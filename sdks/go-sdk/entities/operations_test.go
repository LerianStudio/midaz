package entities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/stretchr/testify/assert"
)

func TestOperationsEntity_ListOperations(t *testing.T) {
	// Test cases
	testCases := []struct {
		name       string
		orgID      string
		ledgerID   string
		accountID  string
		opts       *models.ListOptions
		statusCode int
		response   interface{}
		expectErr  bool
	}{
		{
			name:       "Valid request",
			orgID:      "org123",
			ledgerID:   "ledger123",
			accountID:  "account123",
			opts:       &models.ListOptions{Limit: 10, Offset: 0},
			statusCode: http.StatusOK,
			response: models.ListResponse[models.Operation]{
				Items: []models.Operation{
					{
						ID:        "op123",
						AccountID: "account123",
						Amount:    100,
						Type:      "credit",
						AssetCode: "USD",
						Scale:     2,
					},
				},
				Pagination: models.Pagination{
					Total:  1,
					Limit:  10,
					Offset: 0,
				},
			},
			expectErr: false,
		},
		{
			name:       "Missing organization ID",
			orgID:      "",
			ledgerID:   "ledger123",
			accountID:  "account123",
			opts:       nil,
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:       "Missing ledger ID",
			orgID:      "org123",
			ledgerID:   "",
			accountID:  "account123",
			opts:       nil,
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:       "Missing account ID",
			orgID:      "org123",
			ledgerID:   "ledger123",
			accountID:  "",
			opts:       nil,
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:       "Server error",
			orgID:      "org123",
			ledgerID:   "ledger123",
			accountID:  "account123",
			opts:       nil,
			statusCode: http.StatusInternalServerError,
			response: map[string]string{
				"error":   "internal_error",
				"message": "Internal server error",
			},
			expectErr: true,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check request method
				assert.Equal(t, http.MethodGet, r.Method)

				// Check path
				expectedPath := "/organizations/" + tc.orgID + "/ledgers/" + tc.ledgerID + "/accounts/" + tc.accountID + "/operations"
				assert.Equal(t, expectedPath, r.URL.Path)

				// Check query parameters if options are provided
				if tc.opts != nil {
					query := r.URL.Query()
					assert.Equal(t, "10", query.Get("limit"))
					assert.Equal(t, "0", query.Get("offset"))
				}

				// Return response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				if tc.response != nil {
					json.NewEncoder(w).Encode(tc.response)
				}
			}))
			defer server.Close()

			// Create operations entity
			entity := &operationsEntity{
				httpClient: http.DefaultClient,
				authToken:  "test-token",
				baseURLs: map[string]string{
					"transaction": server.URL,
				},
			}

			// Call ListOperations
			result, err := entity.ListOperations(context.Background(), tc.orgID, tc.ledgerID, tc.accountID, tc.opts)

			// Check error
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, 1, len(result.Items))
				assert.Equal(t, "op123", result.Items[0].ID)
				assert.Equal(t, "account123", result.Items[0].AccountID)
				assert.Equal(t, 100, result.Items[0].Amount)
				assert.Equal(t, "credit", result.Items[0].Type)
				assert.Equal(t, "USD", result.Items[0].AssetCode)
				assert.Equal(t, 2, result.Items[0].Scale)
			}
		})
	}
}

func TestOperationsEntity_GetOperation(t *testing.T) {
	// Test cases
	testCases := []struct {
		name        string
		orgID       string
		ledgerID    string
		accountID   string
		operationID string
		statusCode  int
		response    interface{}
		expectErr   bool
	}{
		{
			name:        "Valid request",
			orgID:       "org123",
			ledgerID:    "ledger123",
			accountID:   "account123",
			operationID: "op123",
			statusCode:  http.StatusOK,
			response: models.Operation{
				ID:        "op123",
				AccountID: "account123",
				Amount:    100,
				Type:      "credit",
				AssetCode: "USD",
				Scale:     2,
			},
			expectErr: false,
		},
		{
			name:        "Missing organization ID",
			orgID:       "",
			ledgerID:    "ledger123",
			accountID:   "account123",
			operationID: "op123",
			statusCode:  http.StatusOK,
			response:    nil,
			expectErr:   true,
		},
		{
			name:        "Missing ledger ID",
			orgID:       "org123",
			ledgerID:    "",
			accountID:   "account123",
			operationID: "op123",
			statusCode:  http.StatusOK,
			response:    nil,
			expectErr:   true,
		},
		{
			name:        "Missing account ID",
			orgID:       "org123",
			ledgerID:    "ledger123",
			accountID:   "",
			operationID: "op123",
			statusCode:  http.StatusOK,
			response:    nil,
			expectErr:   true,
		},
		{
			name:        "Missing operation ID",
			orgID:       "org123",
			ledgerID:    "ledger123",
			accountID:   "account123",
			operationID: "",
			statusCode:  http.StatusOK,
			response:    nil,
			expectErr:   true,
		},
		{
			name:        "Server error",
			orgID:       "org123",
			ledgerID:    "ledger123",
			accountID:   "account123",
			operationID: "op123",
			statusCode:  http.StatusInternalServerError,
			response: map[string]string{
				"error":   "internal_error",
				"message": "Internal server error",
			},
			expectErr: true,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check request method
				assert.Equal(t, http.MethodGet, r.Method)

				// Check path
				expectedPath := "/organizations/" + tc.orgID + "/ledgers/" + tc.ledgerID + "/accounts/" + tc.accountID + "/operations/" + tc.operationID
				assert.Equal(t, expectedPath, r.URL.Path)

				// Return response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				if tc.response != nil {
					json.NewEncoder(w).Encode(tc.response)
				}
			}))
			defer server.Close()

			// Create operations entity
			entity := &operationsEntity{
				httpClient: http.DefaultClient,
				authToken:  "test-token",
				baseURLs: map[string]string{
					"transaction": server.URL,
				},
			}

			// Call GetOperation
			result, err := entity.GetOperation(context.Background(), tc.orgID, tc.ledgerID, tc.accountID, tc.operationID)

			// Check error
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "op123", result.ID)
				assert.Equal(t, "account123", result.AccountID)
				assert.Equal(t, 100, result.Amount)
				assert.Equal(t, "credit", result.Type)
				assert.Equal(t, "USD", result.AssetCode)
				assert.Equal(t, 2, result.Scale)
			}
		})
	}
}

func TestOperationsEntity_UpdateOperation(t *testing.T) {
	// Test cases
	testCases := []struct {
		name          string
		orgID         string
		ledgerID      string
		transactionID string
		operationID   string
		input         interface{}
		statusCode    int
		response      interface{}
		expectErr     bool
	}{
		{
			name:          "Valid request",
			orgID:         "org123",
			ledgerID:      "ledger123",
			transactionID: "tx123",
			operationID:   "op123",
			input: map[string]string{
				"description": "Updated operation",
			},
			statusCode: http.StatusOK,
			response: models.Operation{
				ID:        "op123",
				AccountID: "account123",
				Amount:    100,
				Type:      "credit",
				AssetCode: "USD",
				Scale:     2,
			},
			expectErr: false,
		},
		{
			name:          "Missing organization ID",
			orgID:         "",
			ledgerID:      "ledger123",
			transactionID: "tx123",
			operationID:   "op123",
			input: map[string]string{
				"description": "Updated operation",
			},
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:          "Missing ledger ID",
			orgID:         "org123",
			ledgerID:      "",
			transactionID: "tx123",
			operationID:   "op123",
			input: map[string]string{
				"description": "Updated operation",
			},
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:          "Missing transaction ID",
			orgID:         "org123",
			ledgerID:      "ledger123",
			transactionID: "",
			operationID:   "op123",
			input: map[string]string{
				"description": "Updated operation",
			},
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:          "Missing operation ID",
			orgID:         "org123",
			ledgerID:      "ledger123",
			transactionID: "tx123",
			operationID:   "",
			input: map[string]string{
				"description": "Updated operation",
			},
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:          "Nil input",
			orgID:         "org123",
			ledgerID:      "ledger123",
			transactionID: "tx123",
			operationID:   "op123",
			input:         nil,
			statusCode:    http.StatusOK,
			response:      nil,
			expectErr:     true,
		},
		{
			name:          "Server error",
			orgID:         "org123",
			ledgerID:      "ledger123",
			transactionID: "tx123",
			operationID:   "op123",
			input: map[string]string{
				"description": "Updated operation",
			},
			statusCode: http.StatusInternalServerError,
			response: map[string]string{
				"error":   "internal_error",
				"message": "Internal server error",
			},
			expectErr: true,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Check request method
				assert.Equal(t, http.MethodPut, r.Method)

				// Check path
				expectedPath := "/organizations/" + tc.orgID + "/ledgers/" + tc.ledgerID + "/transactions/" + tc.transactionID + "/operations/" + tc.operationID
				assert.Equal(t, expectedPath, r.URL.Path)

				// Check request body if input is provided
				if tc.input != nil {
					var requestBody map[string]string
					json.NewDecoder(r.Body).Decode(&requestBody)
					assert.Equal(t, "Updated operation", requestBody["description"])
				}

				// Return response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				if tc.response != nil {
					json.NewEncoder(w).Encode(tc.response)
				}
			}))
			defer server.Close()

			// Create operations entity
			entity := &operationsEntity{
				httpClient: http.DefaultClient,
				authToken:  "test-token",
				baseURLs: map[string]string{
					"transaction": server.URL,
				},
			}

			// Call UpdateOperation
			result, err := entity.UpdateOperation(context.Background(), tc.orgID, tc.ledgerID, tc.transactionID, tc.operationID, tc.input)

			// Check error
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "op123", result.ID)
				assert.Equal(t, "account123", result.AccountID)
				assert.Equal(t, 100, result.Amount)
				assert.Equal(t, "credit", result.Type)
				assert.Equal(t, "USD", result.AssetCode)
				assert.Equal(t, 2, result.Scale)
			}
		})
	}
}

func TestOperationsEntity_BuildURL(t *testing.T) {
	// Create operations entity
	entity := &operationsEntity{
		httpClient: http.DefaultClient,
		authToken:  "test-token",
		baseURLs: map[string]string{
			"transaction": "https://api.example.com",
		},
	}

	// Test cases
	testCases := []struct {
		name        string
		orgID       string
		ledgerID    string
		accountID   string
		operationID string
		expected    string
	}{
		{
			name:        "With operation ID",
			orgID:       "org123",
			ledgerID:    "ledger123",
			accountID:   "account123",
			operationID: "op123",
			expected:    "https://api.example.com/organizations/org123/ledgers/ledger123/accounts/account123/operations/op123",
		},
		{
			name:        "Without operation ID",
			orgID:       "org123",
			ledgerID:    "ledger123",
			accountID:   "account123",
			operationID: "",
			expected:    "https://api.example.com/organizations/org123/ledgers/ledger123/accounts/account123/operations",
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := entity.buildURL(tc.orgID, tc.ledgerID, tc.accountID, tc.operationID)
			assert.Equal(t, tc.expected, url)
		})
	}
}

func TestOperationsEntity_SetCommonHeaders(t *testing.T) {
	// Create operations entity
	entity := &operationsEntity{
		httpClient: http.DefaultClient,
		authToken:  "test-token",
		baseURLs: map[string]string{
			"transaction": "https://api.example.com",
		},
	}

	// Create request
	req, _ := http.NewRequest(http.MethodGet, "https://api.example.com", nil)

	// Set headers
	entity.setCommonHeaders(req)

	// Check headers
	assert.Equal(t, "application/json", req.Header.Get("Content-Type"))
	assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
}
