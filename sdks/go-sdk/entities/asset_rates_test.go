package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/stretchr/testify/assert"
)

func TestAssetRatesEntityImplementation(t *testing.T) {
	t.Run("GetAssetRate", func(t *testing.T) {
		testCases := []struct {
			name                 string
			orgID                string
			ledgerID             string
			sourceAssetCode      string
			destinationAssetCode string
			skipServer           bool
			mockStatusCode       int
			mockResponse         map[string]interface{}
			expectError          bool
			errorContains        string
		}{
			{
				name:                 "Missing organization ID",
				orgID:                "",
				ledgerID:             "ledger-123",
				sourceAssetCode:      "USD",
				destinationAssetCode: "EUR",
				skipServer:           true,
				expectError:          true,
				errorContains:        "organization ID is required",
			},
			{
				name:                 "Missing ledger ID",
				orgID:                "org-123",
				ledgerID:             "",
				sourceAssetCode:      "USD",
				destinationAssetCode: "EUR",
				skipServer:           true,
				expectError:          true,
				errorContains:        "ledger ID is required",
			},
			{
				name:                 "Missing source asset code",
				orgID:                "org-123",
				ledgerID:             "ledger-123",
				sourceAssetCode:      "",
				destinationAssetCode: "EUR",
				skipServer:           true,
				expectError:          true,
				errorContains:        "source asset code is required",
			},
			{
				name:                 "Missing destination asset code",
				orgID:                "org-123",
				ledgerID:             "ledger-123",
				sourceAssetCode:      "USD",
				destinationAssetCode: "",
				skipServer:           true,
				expectError:          true,
				errorContains:        "destination asset code is required",
			},
			{
				name:                 "Server error",
				orgID:                "org-123",
				ledgerID:             "ledger-123",
				sourceAssetCode:      "USD",
				destinationAssetCode: "EUR",
				mockStatusCode:       http.StatusInternalServerError,
				mockResponse: map[string]interface{}{
					"message": "Internal server error",
				},
				expectError:   true,
				errorContains: "Internal server error",
			},
			{
				name:                 "Rate not found",
				orgID:                "org-123",
				ledgerID:             "ledger-123",
				sourceAssetCode:      "USD",
				destinationAssetCode: "UNKNOWN",
				mockStatusCode:       http.StatusNotFound,
				mockResponse: map[string]interface{}{
					"message":    "Asset rate not found",
					"entityType": "assetRate",
				},
				expectError:   true,
				errorContains: "Asset rate not found",
			},
			{
				name:                 "Successful response",
				orgID:                "org-123",
				ledgerID:             "ledger-123",
				sourceAssetCode:      "USD",
				destinationAssetCode: "EUR",
				mockStatusCode:       http.StatusOK,
				mockResponse: map[string]interface{}{
					"id":           "rate-123",
					"fromAsset":    "USD",
					"toAsset":      "EUR",
					"rate":         0.85,
					"createdAt":    time.Now().Format(time.RFC3339),
					"updatedAt":    time.Now().Format(time.RFC3339),
					"effectiveAt":  time.Now().Format(time.RFC3339),
					"expirationAt": time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				},
				expectError: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.skipServer {
					// For validation cases that don't need a server
					assetRatesEntity := NewAssetRatesEntity(http.DefaultClient, "test-token", map[string]string{
						"onboarding": "http://example.com",
					})

					// Call GetAssetRate
					result, err := assetRatesEntity.GetAssetRate(context.Background(), tc.orgID, tc.ledgerID, tc.sourceAssetCode, tc.destinationAssetCode)

					// Check error
					assert.Error(t, err)
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
					assert.Nil(t, result)
					return
				}

				// Create a test server
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Check request method
					assert.Equal(t, http.MethodGet, r.Method)

					// Check path and query parameters
					expectedPath := fmt.Sprintf("/organizations/%s/ledgers/%s/assets/rates", tc.orgID, tc.ledgerID)
					assert.Equal(t, expectedPath, r.URL.Path)

					// Check query parameters
					query := r.URL.Query()
					assert.Equal(t, tc.sourceAssetCode, query.Get("source"))
					assert.Equal(t, tc.destinationAssetCode, query.Get("destination"))

					// Set response status code
					w.WriteHeader(tc.mockStatusCode)

					// Write response
					if tc.mockResponse != nil {
						jsonBytes, err := json.Marshal(tc.mockResponse)
						assert.NoError(t, err)
						w.Write(jsonBytes)
					}
				}))
				defer server.Close()

				// Create asset rates entity with proper base URL
				assetRatesEntity := NewAssetRatesEntity(server.Client(), "test-token", map[string]string{
					"onboarding": server.URL,
				})

				// Call GetAssetRate
				result, err := assetRatesEntity.GetAssetRate(context.Background(), tc.orgID, tc.ledgerID, tc.sourceAssetCode, tc.destinationAssetCode)

				// Check error
				if tc.expectError {
					assert.Error(t, err)
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
					assert.Nil(t, result)
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, result)

					// Check response fields
					assert.Equal(t, tc.mockResponse["id"], result.ID)
					assert.Equal(t, tc.mockResponse["fromAsset"], result.FromAsset)
					assert.Equal(t, tc.mockResponse["toAsset"], result.ToAsset)
					assert.Equal(t, tc.mockResponse["rate"], result.Rate)
				}
			})
		}
	})

	t.Run("CreateOrUpdateAssetRate", func(t *testing.T) {
		now := time.Now()
		expiration := now.Add(24 * time.Hour)

		testCases := []struct {
			name           string
			orgID          string
			ledgerID       string
			input          *models.UpdateAssetRateInput
			skipServer     bool
			mockStatusCode int
			mockResponse   map[string]interface{}
			expectError    bool
			errorContains  string
		}{
			{
				name:          "Missing organization ID",
				orgID:         "",
				ledgerID:      "ledger-123",
				input:         models.NewUpdateAssetRateInput("USD", "EUR", 0.85, now, expiration),
				skipServer:    true,
				expectError:   true,
				errorContains: "organization ID is required",
			},
			{
				name:          "Missing ledger ID",
				orgID:         "org-123",
				ledgerID:      "",
				input:         models.NewUpdateAssetRateInput("USD", "EUR", 0.85, now, expiration),
				skipServer:    true,
				expectError:   true,
				errorContains: "ledger ID is required",
			},
			{
				name:          "Nil input",
				orgID:         "org-123",
				ledgerID:      "ledger-123",
				input:         nil,
				skipServer:    true,
				expectError:   true,
				errorContains: "asset rate input cannot be nil",
			},
			{
				name:          "Invalid input - missing source asset",
				orgID:         "org-123",
				ledgerID:      "ledger-123",
				input:         models.NewUpdateAssetRateInput("", "EUR", 0.85, now, expiration),
				skipServer:    true,
				expectError:   true,
				errorContains: "invalid asset rate input",
			},
			{
				name:          "Invalid input - missing destination asset",
				orgID:         "org-123",
				ledgerID:      "ledger-123",
				input:         models.NewUpdateAssetRateInput("USD", "", 0.85, now, expiration),
				skipServer:    true,
				expectError:   true,
				errorContains: "invalid asset rate input",
			},
			{
				name:          "Invalid input - negative rate",
				orgID:         "org-123",
				ledgerID:      "ledger-123",
				input:         models.NewUpdateAssetRateInput("USD", "EUR", -0.85, now, expiration),
				skipServer:    true,
				expectError:   true,
				errorContains: "invalid asset rate input",
			},
			{
				name:           "Server error",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				input:          models.NewUpdateAssetRateInput("USD", "EUR", 0.85, now, expiration),
				mockStatusCode: http.StatusInternalServerError,
				mockResponse: map[string]interface{}{
					"message": "Internal server error",
				},
				expectError:   true,
				errorContains: "Internal server error",
			},
			{
				name:           "Validation error",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				input:          models.NewUpdateAssetRateInput("USD", "EUR", 0.85, now, expiration),
				mockStatusCode: http.StatusBadRequest,
				mockResponse: map[string]interface{}{
					"message":    "Validation failed",
					"entityType": "assetRate",
					"fields": map[string]string{
						"rate": "Rate must be positive",
					},
				},
				expectError:   true,
				errorContains: "Validation failed",
			},
			{
				name:           "Successful creation",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				input:          models.NewUpdateAssetRateInput("USD", "EUR", 0.85, now, expiration),
				mockStatusCode: http.StatusCreated,
				mockResponse: map[string]interface{}{
					"id":           "rate-123",
					"fromAsset":    "USD",
					"toAsset":      "EUR",
					"rate":         0.85,
					"createdAt":    now.Format(time.RFC3339),
					"updatedAt":    now.Format(time.RFC3339),
					"effectiveAt":  now.Format(time.RFC3339),
					"expirationAt": expiration.Format(time.RFC3339),
				},
				expectError: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.skipServer {
					// For validation cases that don't need a server
					assetRatesEntity := NewAssetRatesEntity(http.DefaultClient, "test-token", map[string]string{
						"onboarding": "http://example.com",
					})

					// Call CreateOrUpdateAssetRate
					result, err := assetRatesEntity.CreateOrUpdateAssetRate(context.Background(), tc.orgID, tc.ledgerID, tc.input)

					// Check error
					assert.Error(t, err)
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
					assert.Nil(t, result)
					return
				}

				// Create a test server
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Check request method
					assert.Equal(t, http.MethodPost, r.Method)

					// Check path
					expectedPath := fmt.Sprintf("/organizations/%s/ledgers/%s/assets/rates", tc.orgID, tc.ledgerID)
					assert.Equal(t, expectedPath, r.URL.Path)

					// Check request body
					var requestBody models.UpdateAssetRateInput
					err := json.NewDecoder(r.Body).Decode(&requestBody)
					assert.NoError(t, err)
					assert.Equal(t, tc.input.FromAsset, requestBody.FromAsset)
					assert.Equal(t, tc.input.ToAsset, requestBody.ToAsset)
					assert.Equal(t, tc.input.Rate, requestBody.Rate)

					// Set response status code
					w.WriteHeader(tc.mockStatusCode)

					// Write response
					if tc.mockResponse != nil {
						jsonBytes, err := json.Marshal(tc.mockResponse)
						assert.NoError(t, err)
						w.Write(jsonBytes)
					}
				}))
				defer server.Close()

				// Create asset rates entity with proper base URL
				assetRatesEntity := NewAssetRatesEntity(server.Client(), "test-token", map[string]string{
					"onboarding": server.URL,
				})

				// Call CreateOrUpdateAssetRate
				result, err := assetRatesEntity.CreateOrUpdateAssetRate(context.Background(), tc.orgID, tc.ledgerID, tc.input)

				// Check error
				if tc.expectError {
					assert.Error(t, err)
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
					assert.Nil(t, result)
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, result)

					// Check response fields
					assert.Equal(t, tc.mockResponse["id"], result.ID)
					assert.Equal(t, tc.mockResponse["fromAsset"], result.FromAsset)
					assert.Equal(t, tc.mockResponse["toAsset"], result.ToAsset)
					assert.Equal(t, tc.mockResponse["rate"], result.Rate)
				}
			})
		}
	})
}
