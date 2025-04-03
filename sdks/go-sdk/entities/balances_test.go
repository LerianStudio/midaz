package entities

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/entities/mocks"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// TestListBalances tests the mock interface for ListBalances
func TestListBalances(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock balances service
	mockService := mocks.NewMockBalancesService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	now := time.Now()

	// Create test balances list response
	balancesList := &models.ListResponse[models.Balance]{
		Items: []models.Balance{
			{
				ID:             "bal-123",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "acc-1",
				Alias:          "user-account",
				AssetCode:      "USD",
				Available:      1000000,
				OnHold:         0,
				Scale:          100,
				Version:        1,
				AccountType:    "LIABILITY",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			{
				ID:             "bal-456",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      "acc-2",
				Alias:          "platform-account",
				AssetCode:      "USD",
				Available:      5000000,
				OnHold:         200000,
				Scale:          100,
				Version:        2,
				AccountType:    "ASSET",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		Pagination: models.Pagination{
			Total:  2,
			Limit:  10,
			Offset: 0,
		},
	}

	// Setup expectations for default options
	mockService.EXPECT().
		ListBalances(gomock.Any(), orgID, ledgerID, gomock.Nil()).
		Return(balancesList, nil)

	// Test listing balances with default options
	result, err := mockService.ListBalances(ctx, orgID, ledgerID, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "bal-123", result.Items[0].ID)
	assert.Equal(t, "acc-1", result.Items[0].AccountID)
	assert.Equal(t, "user-account", result.Items[0].Alias)
	assert.Equal(t, "USD", result.Items[0].AssetCode)
	assert.Equal(t, int64(1000000), result.Items[0].Available)
	assert.Equal(t, int64(0), result.Items[0].OnHold)
	assert.Equal(t, int64(100), result.Items[0].Scale)
	assert.Equal(t, "LIABILITY", result.Items[0].AccountType)
	assert.Equal(t, true, result.Items[0].AllowSending)
	assert.Equal(t, true, result.Items[0].AllowReceiving)

	// Test with options
	opts := &models.ListOptions{
		Limit:          5,
		Offset:         0,
		OrderBy:        "created_at",
		OrderDirection: "desc",
	}

	// Setup expectations with options
	mockService.EXPECT().
		ListBalances(gomock.Any(), orgID, ledgerID, opts).
		Return(balancesList, nil)

	// Test listing balances with options
	result, err = mockService.ListBalances(ctx, orgID, ledgerID, opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
}

// TestListAccountBalances tests the mock interface for ListAccountBalances
func TestListAccountBalances(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock balances service
	mockService := mocks.NewMockBalancesService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	accountID := "acc-1"
	now := time.Now()

	// Create test balances list response
	balancesList := &models.ListResponse[models.Balance]{
		Items: []models.Balance{
			{
				ID:             "bal-123",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "user-account",
				AssetCode:      "USD",
				Available:      1000000,
				OnHold:         0,
				Scale:          100,
				Version:        1,
				AccountType:    "LIABILITY",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
			{
				ID:             "bal-456",
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				AccountID:      accountID,
				Alias:          "user-account",
				AssetCode:      "EUR",
				Available:      500000,
				OnHold:         0,
				Scale:          100,
				Version:        1,
				AccountType:    "LIABILITY",
				AllowSending:   true,
				AllowReceiving: true,
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		},
		Pagination: models.Pagination{
			Total:  2,
			Limit:  10,
			Offset: 0,
		},
	}

	// Setup expectations for default options
	mockService.EXPECT().
		ListAccountBalances(gomock.Any(), orgID, ledgerID, accountID, gomock.Nil()).
		Return(balancesList, nil)

	// Test listing account balances with default options
	result, err := mockService.ListAccountBalances(ctx, orgID, ledgerID, accountID, nil)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
	assert.Equal(t, "bal-123", result.Items[0].ID)
	assert.Equal(t, accountID, result.Items[0].AccountID)
	assert.Equal(t, "user-account", result.Items[0].Alias)
	assert.Equal(t, "USD", result.Items[0].AssetCode)
	assert.Equal(t, "bal-456", result.Items[1].ID)
	assert.Equal(t, "EUR", result.Items[1].AssetCode)

	// Test with options
	opts := &models.ListOptions{
		Limit:          5,
		Offset:         0,
		OrderBy:        "created_at",
		OrderDirection: "desc",
	}

	// Setup expectations with options
	mockService.EXPECT().
		ListAccountBalances(gomock.Any(), orgID, ledgerID, accountID, opts).
		Return(balancesList, nil)

	// Test listing account balances with options
	result, err = mockService.ListAccountBalances(ctx, orgID, ledgerID, accountID, opts)
	assert.NoError(t, err)
	assert.Equal(t, 2, result.Pagination.Total)
	assert.Len(t, result.Items, 2)
}

// TestGetBalanceByID tests the mock interface for GetBalance
func TestGetBalanceByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock balances service
	mockService := mocks.NewMockBalancesService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	balanceID := "bal-123"
	now := time.Now()

	// Create test balance
	balance := &models.Balance{
		ID:             balanceID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      "acc-1",
		Alias:          "user-account",
		AssetCode:      "USD",
		Available:      1000000,
		OnHold:         0,
		Scale:          100,
		Version:        1,
		AccountType:    "LIABILITY",
		AllowSending:   true,
		AllowReceiving: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Setup expectations
	mockService.EXPECT().
		GetBalance(gomock.Any(), orgID, ledgerID, balanceID).
		Return(balance, nil)

	// Test getting balance by ID
	result, err := mockService.GetBalance(ctx, orgID, ledgerID, balanceID)
	assert.NoError(t, err)
	assert.Equal(t, balanceID, result.ID)
	assert.Equal(t, orgID, result.OrganizationID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "acc-1", result.AccountID)
	assert.Equal(t, "user-account", result.Alias)
	assert.Equal(t, "USD", result.AssetCode)
	assert.Equal(t, int64(1000000), result.Available)
	assert.Equal(t, int64(0), result.OnHold)
	assert.Equal(t, int64(100), result.Scale)
	assert.Equal(t, int64(1), result.Version)
	assert.Equal(t, "LIABILITY", result.AccountType)
	assert.Equal(t, true, result.AllowSending)
	assert.Equal(t, true, result.AllowReceiving)
	assert.Equal(t, now, result.CreatedAt)
	assert.Equal(t, now, result.UpdatedAt)
}

// TestUpdateBalance tests the mock interface for UpdateBalance
func TestUpdateBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock balances service
	mockService := mocks.NewMockBalancesService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	balanceID := "bal-123"
	now := time.Now()

	// Create test balance
	balance := &models.Balance{
		ID:             balanceID,
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		AccountID:      "acc-1",
		Alias:          "user-account",
		AssetCode:      "USD",
		Available:      2000000, // Updated value
		OnHold:         100000,  // Updated value
		Scale:          100,
		Version:        2, // Updated version
		AccountType:    "LIABILITY",
		AllowSending:   true,
		AllowReceiving: false, // Updated value
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Create update input
	allowReceiving := false
	input := &models.UpdateBalanceInput{
		AllowReceiving: &allowReceiving,
	}

	// Setup expectations
	mockService.EXPECT().
		UpdateBalance(gomock.Any(), orgID, ledgerID, balanceID, input).
		Return(balance, nil)

	// Test updating balance
	result, err := mockService.UpdateBalance(ctx, orgID, ledgerID, balanceID, input)
	assert.NoError(t, err)
	assert.Equal(t, balanceID, result.ID)
	assert.Equal(t, int64(2000000), result.Available)
	assert.Equal(t, int64(100000), result.OnHold)
	assert.Equal(t, int64(2), result.Version)
	assert.Equal(t, true, result.AllowSending)
	assert.Equal(t, false, result.AllowReceiving)
}

// TestDeleteBalance tests the mock interface for DeleteBalance
func TestDeleteBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock balances service
	mockService := mocks.NewMockBalancesService(ctrl)

	// Test data
	ctx := context.Background()
	orgID := "org-123"
	ledgerID := "ledger-123"
	balanceID := "bal-123"

	// Setup expectations
	mockService.EXPECT().
		DeleteBalance(gomock.Any(), orgID, ledgerID, balanceID).
		Return(nil)

	// Test deleting balance
	err := mockService.DeleteBalance(ctx, orgID, ledgerID, balanceID)
	assert.NoError(t, err)
}

// TestBalancesEntityImplementation tests the actual implementation of the balancesEntity
func TestBalancesEntityImplementation(t *testing.T) {
	t.Run("ListBalances", func(t *testing.T) {
		testCases := []struct {
			name           string
			orgID          string
			ledgerID       string
			opts           *models.ListOptions
			mockResponse   interface{}
			mockStatusCode int
			expectError    bool
			errorContains  string
			skipServer     bool
		}{
			{
				name:          "Missing organization ID",
				orgID:         "",
				ledgerID:      "ledger-123",
				expectError:   true,
				errorContains: "organization ID is required",
				skipServer:    true,
			},
			{
				name:          "Missing ledger ID",
				orgID:         "org-123",
				ledgerID:      "",
				expectError:   true,
				errorContains: "ledger ID is required",
				skipServer:    true,
			},
			{
				name:           "Server error",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				mockStatusCode: http.StatusInternalServerError,
				mockResponse: map[string]interface{}{
					"message": "Internal server error",
				},
				expectError:   true,
				errorContains: "Internal server error",
			},
			{
				name:           "Successful response",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				mockStatusCode: http.StatusOK,
				mockResponse: models.ListResponse[models.Balance]{
					Items: []models.Balance{
						{
							ID:             "bal-123",
							OrganizationID: "org-123",
							LedgerID:       "ledger-123",
							AccountID:      "acc-1",
							AssetCode:      "USD",
							Available:      1000000,
							Scale:          100,
						},
					},
					Pagination: models.Pagination{
						Total:  1,
						Limit:  10,
						Offset: 0,
					},
				},
				expectError: false,
			},
			{
				name:     "Successful response with options",
				orgID:    "org-123",
				ledgerID: "ledger-123",
				opts: &models.ListOptions{
					Limit:          5,
					Offset:         10,
					OrderBy:        "created_at",
					OrderDirection: "desc",
				},
				mockStatusCode: http.StatusOK,
				mockResponse: models.ListResponse[models.Balance]{
					Items: []models.Balance{
						{
							ID:             "bal-123",
							OrganizationID: "org-123",
							LedgerID:       "ledger-123",
							AccountID:      "acc-1",
							AssetCode:      "USD",
							Available:      1000000,
							Scale:          100,
						},
					},
					Pagination: models.Pagination{
						Total:  1,
						Limit:  5,
						Offset: 10,
					},
				},
				expectError: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.skipServer {
					// For validation cases that don't need a server
					balancesEntity := NewBalancesEntity(http.DefaultClient, "test-token", map[string]string{
						"onboarding": "http://example.com",
					})

					// Call ListBalances
					result, err := balancesEntity.ListBalances(context.Background(), tc.orgID, tc.ledgerID, tc.opts)

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

					// Check path
					expectedPath := fmt.Sprintf("/organizations/%s/ledgers/%s/balances", tc.orgID, tc.ledgerID)
					assert.Equal(t, expectedPath, r.URL.Path)

					// Check query parameters if options are provided
					if tc.opts != nil {
						query := r.URL.Query()
						for key, value := range tc.opts.ToQueryParams() {
							assert.Equal(t, value, query.Get(key))
						}
					}

					// Set response status code
					w.WriteHeader(tc.mockStatusCode)

					// Write response
					if tc.mockResponse != nil {
						// For error cases, just write invalid JSON to trigger a decode error
						if tc.mockStatusCode >= 400 {
							jsonBytes, err := json.Marshal(tc.mockResponse)
							assert.NoError(t, err)
							w.Write(jsonBytes)
						} else {
							err := json.NewEncoder(w).Encode(tc.mockResponse)
							assert.NoError(t, err)
						}
					}
				}))
				defer server.Close()

				// Create balances entity with proper base URL
				balancesEntity := NewBalancesEntity(server.Client(), "test-token", map[string]string{
					"onboarding": server.URL,
				})

				// Call ListBalances
				result, err := balancesEntity.ListBalances(context.Background(), tc.orgID, tc.ledgerID, tc.opts)

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

					// Check response
					if mockResponse, ok := tc.mockResponse.(models.ListResponse[models.Balance]); ok {
						assert.Equal(t, mockResponse.Pagination.Total, result.Pagination.Total)
						assert.Equal(t, len(mockResponse.Items), len(result.Items))
						if len(mockResponse.Items) > 0 {
							assert.Equal(t, mockResponse.Items[0].ID, result.Items[0].ID)
						}
					}
				}
			})
		}
	})

	t.Run("ListAccountBalances", func(t *testing.T) {
		testCases := []struct {
			name           string
			orgID          string
			ledgerID       string
			accountID      string
			opts           *models.ListOptions
			mockResponse   interface{}
			mockStatusCode int
			expectError    bool
			errorContains  string
			skipServer     bool
		}{
			{
				name:          "Missing organization ID",
				orgID:         "",
				ledgerID:      "ledger-123",
				accountID:     "acc-123",
				expectError:   true,
				errorContains: "organization ID is required",
				skipServer:    true,
			},
			{
				name:          "Missing ledger ID",
				orgID:         "org-123",
				ledgerID:      "",
				accountID:     "acc-123",
				expectError:   true,
				errorContains: "ledger ID is required",
				skipServer:    true,
			},
			{
				name:          "Missing account ID",
				orgID:         "org-123",
				ledgerID:      "ledger-123",
				accountID:     "",
				expectError:   true,
				errorContains: "account ID is required",
				skipServer:    true,
			},
			{
				name:           "Server error",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				accountID:      "acc-123",
				mockStatusCode: http.StatusInternalServerError,
				mockResponse: map[string]interface{}{
					"message": "Internal server error",
				},
				expectError:   true,
				errorContains: "Internal server error",
			},
			{
				name:           "Successful response",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				accountID:      "acc-123",
				mockStatusCode: http.StatusOK,
				mockResponse: models.ListResponse[models.Balance]{
					Items: []models.Balance{
						{
							ID:             "bal-123",
							OrganizationID: "org-123",
							LedgerID:       "ledger-123",
							AccountID:      "acc-123",
							AssetCode:      "USD",
							Available:      1000000,
							Scale:          100,
						},
					},
					Pagination: models.Pagination{
						Total:  1,
						Limit:  10,
						Offset: 0,
					},
				},
				expectError: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.skipServer {
					// For validation cases that don't need a server
					balancesEntity := NewBalancesEntity(http.DefaultClient, "test-token", map[string]string{
						"onboarding": "http://example.com",
					})

					// Call ListAccountBalances
					result, err := balancesEntity.ListAccountBalances(context.Background(), tc.orgID, tc.ledgerID, tc.accountID, tc.opts)

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

					// Check path
					expectedPath := fmt.Sprintf("/organizations/%s/ledgers/%s/accounts/%s/balances", tc.orgID, tc.ledgerID, tc.accountID)
					assert.Equal(t, expectedPath, r.URL.Path)

					// Check query parameters if options are provided
					if tc.opts != nil {
						query := r.URL.Query()
						for key, value := range tc.opts.ToQueryParams() {
							assert.Equal(t, value, query.Get(key))
						}
					}

					// Set response status code
					w.WriteHeader(tc.mockStatusCode)

					// Write response
					if tc.mockResponse != nil {
						// For error cases, just write invalid JSON to trigger a decode error
						if tc.mockStatusCode >= 400 {
							jsonBytes, err := json.Marshal(tc.mockResponse)
							assert.NoError(t, err)
							w.Write(jsonBytes)
						} else {
							err := json.NewEncoder(w).Encode(tc.mockResponse)
							assert.NoError(t, err)
						}
					}
				}))
				defer server.Close()

				// Create balances entity with proper base URL
				balancesEntity := NewBalancesEntity(server.Client(), "test-token", map[string]string{
					"onboarding": server.URL,
				})

				// Call ListAccountBalances
				result, err := balancesEntity.ListAccountBalances(context.Background(), tc.orgID, tc.ledgerID, tc.accountID, tc.opts)

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

					// Check response
					if mockResponse, ok := tc.mockResponse.(models.ListResponse[models.Balance]); ok {
						assert.Equal(t, mockResponse.Pagination.Total, result.Pagination.Total)
						assert.Equal(t, len(mockResponse.Items), len(result.Items))
						if len(mockResponse.Items) > 0 {
							assert.Equal(t, mockResponse.Items[0].ID, result.Items[0].ID)
							assert.Equal(t, mockResponse.Items[0].AccountID, result.Items[0].AccountID)
						}
					}
				}
			})
		}
	})

	t.Run("GetBalance", func(t *testing.T) {
		testCases := []struct {
			name           string
			orgID          string
			ledgerID       string
			balanceID      string
			mockResponse   interface{}
			mockStatusCode int
			expectError    bool
			errorContains  string
			skipServer     bool
		}{
			{
				name:          "Missing organization ID",
				orgID:         "",
				ledgerID:      "ledger-123",
				balanceID:     "bal-123",
				expectError:   true,
				errorContains: "organization ID is required",
				skipServer:    true,
			},
			{
				name:          "Missing ledger ID",
				orgID:         "org-123",
				ledgerID:      "",
				balanceID:     "bal-123",
				expectError:   true,
				errorContains: "ledger ID is required",
				skipServer:    true,
			},
			{
				name:          "Missing balance ID",
				orgID:         "org-123",
				ledgerID:      "ledger-123",
				balanceID:     "",
				expectError:   true,
				errorContains: "balance ID is required",
				skipServer:    true,
			},
			{
				name:           "Balance not found",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				balanceID:      "bal-not-found",
				mockStatusCode: http.StatusNotFound,
				mockResponse: map[string]interface{}{
					"message":    "Balance not found",
					"entityType": "balance",
				},
				expectError:   true,
				errorContains: "Balance not found",
			},
			{
				name:           "Server error",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				balanceID:      "bal-123",
				mockStatusCode: http.StatusInternalServerError,
				mockResponse: map[string]interface{}{
					"message": "Internal server error",
				},
				expectError:   true,
				errorContains: "Internal server error",
			},
			{
				name:           "Successful response",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				balanceID:      "bal-123",
				mockStatusCode: http.StatusOK,
				mockResponse: models.Balance{
					ID:             "bal-123",
					OrganizationID: "org-123",
					LedgerID:       "ledger-123",
					AccountID:      "acc-123",
					AssetCode:      "USD",
					Available:      1000000,
					Scale:          100,
					Version:        1,
				},
				expectError: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.skipServer {
					// For validation cases that don't need a server
					balancesEntity := NewBalancesEntity(http.DefaultClient, "test-token", map[string]string{
						"onboarding": "http://example.com",
					})

					// Call GetBalance
					result, err := balancesEntity.GetBalance(context.Background(), tc.orgID, tc.ledgerID, tc.balanceID)

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

					// Check path
					expectedPath := fmt.Sprintf("/organizations/%s/ledgers/%s/balances/%s", tc.orgID, tc.ledgerID, tc.balanceID)
					assert.Equal(t, expectedPath, r.URL.Path)

					// Set response status code
					w.WriteHeader(tc.mockStatusCode)

					// Write response
					if tc.mockResponse != nil {
						// For error cases, just write invalid JSON to trigger a decode error
						if tc.mockStatusCode >= 400 {
							jsonBytes, err := json.Marshal(tc.mockResponse)
							assert.NoError(t, err)
							w.Write(jsonBytes)
						} else {
							err := json.NewEncoder(w).Encode(tc.mockResponse)
							assert.NoError(t, err)
						}
					}
				}))
				defer server.Close()

				// Create balances entity with proper base URL
				balancesEntity := NewBalancesEntity(server.Client(), "test-token", map[string]string{
					"onboarding": server.URL,
				})

				// Call GetBalance
				result, err := balancesEntity.GetBalance(context.Background(), tc.orgID, tc.ledgerID, tc.balanceID)

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

					// Check response
					if mockResponse, ok := tc.mockResponse.(models.Balance); ok {
						assert.Equal(t, mockResponse.ID, result.ID)
						assert.Equal(t, mockResponse.OrganizationID, result.OrganizationID)
						assert.Equal(t, mockResponse.LedgerID, result.LedgerID)
						assert.Equal(t, mockResponse.AccountID, result.AccountID)
						assert.Equal(t, mockResponse.AssetCode, result.AssetCode)
						assert.Equal(t, mockResponse.Available, result.Available)
						assert.Equal(t, mockResponse.Scale, result.Scale)
						assert.Equal(t, mockResponse.Version, result.Version)
					}
				}
			})
		}
	})

	t.Run("UpdateBalance", func(t *testing.T) {
		testCases := []struct {
			name           string
			orgID          string
			ledgerID       string
			balanceID      string
			input          *models.UpdateBalanceInput
			mockResponse   interface{}
			mockStatusCode int
			expectError    bool
			errorContains  string
			skipServer     bool
		}{
			{
				name:          "Missing organization ID",
				orgID:         "",
				ledgerID:      "ledger-123",
				balanceID:     "bal-123",
				input:         &models.UpdateBalanceInput{},
				expectError:   true,
				errorContains: "organization ID is required",
				skipServer:    true,
			},
			{
				name:          "Missing ledger ID",
				orgID:         "org-123",
				ledgerID:      "",
				balanceID:     "bal-123",
				input:         &models.UpdateBalanceInput{},
				expectError:   true,
				errorContains: "ledger ID is required",
				skipServer:    true,
			},
			{
				name:          "Missing balance ID",
				orgID:         "org-123",
				ledgerID:      "ledger-123",
				balanceID:     "",
				input:         &models.UpdateBalanceInput{},
				expectError:   true,
				errorContains: "balance ID is required",
				skipServer:    true,
			},
			{
				name:          "Nil input",
				orgID:         "org-123",
				ledgerID:      "ledger-123",
				balanceID:     "bal-123",
				input:         nil,
				expectError:   true,
				errorContains: "input cannot be nil",
				skipServer:    true,
			},
			{
				name:           "Validation error",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				balanceID:      "bal-123",
				input:          &models.UpdateBalanceInput{},
				mockStatusCode: http.StatusBadRequest,
				mockResponse: map[string]interface{}{
					"message":    "Validation failed",
					"entityType": "balance",
					"fields": map[string]string{
						"amount": "Amount must be greater than zero",
					},
				},
				expectError:   true,
				errorContains: "invalid balance update input",
			},
			{
				name:           "Balance not found",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				balanceID:      "bal-not-found",
				input:          models.NewUpdateBalanceInput().WithAllowSending(true),
				mockStatusCode: http.StatusNotFound,
				mockResponse: map[string]interface{}{
					"message":    "Balance not found",
					"entityType": "balance",
				},
				expectError:   true,
				errorContains: "Balance not found",
			},
			{
				name:           "Successful update",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				balanceID:      "bal-123",
				input:          models.NewUpdateBalanceInput().WithAllowSending(true).WithAllowReceiving(false),
				mockStatusCode: http.StatusOK,
				mockResponse: models.Balance{
					ID:             "bal-123",
					OrganizationID: "org-123",
					LedgerID:       "ledger-123",
					AccountID:      "acc-123",
					AssetCode:      "USD",
					Available:      1000000,
					Scale:          100,
					Version:        2,
					AllowSending:   true,
					AllowReceiving: false,
				},
				expectError: false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.skipServer {
					// For validation cases that don't need a server
					balancesEntity := NewBalancesEntity(http.DefaultClient, "test-token", map[string]string{
						"onboarding": "http://example.com",
					})

					// Call UpdateBalance
					result, err := balancesEntity.UpdateBalance(context.Background(), tc.orgID, tc.ledgerID, tc.balanceID, tc.input)

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
					assert.Equal(t, http.MethodPatch, r.Method)

					// Check path
					expectedPath := fmt.Sprintf("/organizations/%s/ledgers/%s/balances/%s", tc.orgID, tc.ledgerID, tc.balanceID)
					assert.Equal(t, expectedPath, r.URL.Path)

					// Check request body if input is not nil
					if tc.input != nil {
						var requestBody map[string]interface{}
						err := json.NewDecoder(r.Body).Decode(&requestBody)
						assert.NoError(t, err)

						// Check if the request body contains the expected fields
						if tc.input.AllowSending != nil {
							assert.Equal(t, *tc.input.AllowSending, requestBody["allowSending"])
						}
						if tc.input.AllowReceiving != nil {
							assert.Equal(t, *tc.input.AllowReceiving, requestBody["allowReceiving"])
						}
					}

					// Set response status code
					w.WriteHeader(tc.mockStatusCode)

					// Write response
					if tc.mockResponse != nil {
						// For error cases, just write invalid JSON to trigger a decode error
						if tc.mockStatusCode >= 400 {
							jsonBytes, err := json.Marshal(tc.mockResponse)
							assert.NoError(t, err)
							w.Write(jsonBytes)
						} else {
							err := json.NewEncoder(w).Encode(tc.mockResponse)
							assert.NoError(t, err)
						}
					}
				}))
				defer server.Close()

				// Create balances entity with proper base URL
				balancesEntity := NewBalancesEntity(server.Client(), "test-token", map[string]string{
					"onboarding": server.URL,
				})

				// Call UpdateBalance
				result, err := balancesEntity.UpdateBalance(context.Background(), tc.orgID, tc.ledgerID, tc.balanceID, tc.input)

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

					// Check response
					if mockResponse, ok := tc.mockResponse.(models.Balance); ok {
						assert.Equal(t, mockResponse.ID, result.ID)
						assert.Equal(t, mockResponse.Version, result.Version)
						assert.Equal(t, mockResponse.AllowSending, result.AllowSending)
						assert.Equal(t, mockResponse.AllowReceiving, result.AllowReceiving)
					}
				}
			})
		}
	})

	t.Run("DeleteBalance", func(t *testing.T) {
		testCases := []struct {
			name           string
			orgID          string
			ledgerID       string
			balanceID      string
			mockStatusCode int
			mockResponse   interface{}
			expectError    bool
			errorContains  string
			skipServer     bool
		}{
			{
				name:          "Missing organization ID",
				orgID:         "",
				ledgerID:      "ledger-123",
				balanceID:     "bal-123",
				expectError:   true,
				errorContains: "organization ID is required",
				skipServer:    true,
			},
			{
				name:          "Missing ledger ID",
				orgID:         "org-123",
				ledgerID:      "",
				balanceID:     "bal-123",
				expectError:   true,
				errorContains: "ledger ID is required",
				skipServer:    true,
			},
			{
				name:          "Missing balance ID",
				orgID:         "org-123",
				ledgerID:      "ledger-123",
				balanceID:     "",
				expectError:   true,
				errorContains: "balance ID is required",
				skipServer:    true,
			},
			{
				name:           "Balance not found",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				balanceID:      "bal-not-found",
				mockStatusCode: http.StatusNotFound,
				mockResponse: map[string]interface{}{
					"message":    "Balance not found",
					"entityType": "balance",
				},
				expectError:   true,
				errorContains: "Balance not found",
			},
			{
				name:           "Server error",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				balanceID:      "bal-123",
				mockStatusCode: http.StatusInternalServerError,
				mockResponse: map[string]interface{}{
					"message": "Internal server error",
				},
				expectError:   true,
				errorContains: "Internal server error",
			},
			{
				name:           "Successful deletion",
				orgID:          "org-123",
				ledgerID:       "ledger-123",
				balanceID:      "bal-123",
				mockStatusCode: http.StatusNoContent,
				expectError:    false,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				if tc.skipServer {
					// For validation cases that don't need a server
					balancesEntity := NewBalancesEntity(http.DefaultClient, "test-token", map[string]string{
						"onboarding": "http://example.com",
					})

					// Call DeleteBalance
					err := balancesEntity.DeleteBalance(context.Background(), tc.orgID, tc.ledgerID, tc.balanceID)

					// Check error
					assert.Error(t, err)
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
					return
				}

				// Create a test server
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Check request method
					assert.Equal(t, http.MethodDelete, r.Method)

					// Check path
					expectedPath := fmt.Sprintf("/organizations/%s/ledgers/%s/balances/%s", tc.orgID, tc.ledgerID, tc.balanceID)
					assert.Equal(t, expectedPath, r.URL.Path)

					// Set response status code
					w.WriteHeader(tc.mockStatusCode)

					// Write response
					if tc.mockResponse != nil {
						// For error cases, just write invalid JSON to trigger a decode error
						if tc.mockStatusCode >= 400 {
							jsonBytes, err := json.Marshal(tc.mockResponse)
							assert.NoError(t, err)
							w.Write(jsonBytes)
						} else {
							err := json.NewEncoder(w).Encode(tc.mockResponse)
							assert.NoError(t, err)
						}
					}
				}))
				defer server.Close()

				// Create balances entity with proper base URL
				balancesEntity := NewBalancesEntity(server.Client(), "test-token", map[string]string{
					"onboarding": server.URL,
				})

				// Call DeleteBalance
				err := balancesEntity.DeleteBalance(context.Background(), tc.orgID, tc.ledgerID, tc.balanceID)

				// Check error
				if tc.expectError {
					assert.Error(t, err)
					if tc.errorContains != "" {
						assert.Contains(t, err.Error(), tc.errorContains)
					}
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}
