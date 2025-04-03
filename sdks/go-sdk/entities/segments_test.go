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

func TestSegmentsEntity_ListSegments(t *testing.T) {
	// Test cases
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		portfolioID    string
		opts           *models.ListOptions
		statusCode     int
		response       interface{}
		expectErr      bool
	}{
		{
			name:           "Valid request",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			opts:           &models.ListOptions{Limit: 10, Offset: 0},
			statusCode:     http.StatusOK,
			response: models.ListResponse[models.Segment]{
				Items: []models.Segment{
					{
						ID:             "segment123",
						Name:           "Test Segment",
						LedgerID:       "ledger123",
						OrganizationID: "org123",
						Status:         models.Status{Code: "active"},
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
			name:           "Missing organization ID",
			organizationID: "",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			opts:           nil,
			statusCode:     http.StatusOK,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Missing ledger ID",
			organizationID: "org123",
			ledgerID:       "",
			portfolioID:    "portfolio123",
			opts:           nil,
			statusCode:     http.StatusOK,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Missing portfolio ID",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "",
			opts:           nil,
			statusCode:     http.StatusOK,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Server error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			opts:           nil,
			statusCode:     http.StatusInternalServerError,
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
				expectedPath := "/organizations/" + tc.organizationID + "/ledgers/" + tc.ledgerID + "/portfolios/" + tc.portfolioID + "/segments"
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

			// Create segments entity
			entity := &segmentsEntity{
				httpClient: http.DefaultClient,
				authToken:  "test-token",
				baseURLs: map[string]string{
					"onboarding": server.URL,
				},
			}

			// Call ListSegments
			result, err := entity.ListSegments(context.Background(), tc.organizationID, tc.ledgerID, tc.portfolioID, tc.opts)

			// Check error
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, 1, len(result.Items))
				assert.Equal(t, "segment123", result.Items[0].ID)
				assert.Equal(t, "Test Segment", result.Items[0].Name)
				assert.Equal(t, "ledger123", result.Items[0].LedgerID)
				assert.Equal(t, "org123", result.Items[0].OrganizationID)
				assert.Equal(t, "active", result.Items[0].Status.Code)
			}
		})
	}
}

func TestSegmentsEntity_GetSegment(t *testing.T) {
	// Test cases
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		portfolioID    string
		segmentID      string
		statusCode     int
		response       interface{}
		expectErr      bool
	}{
		{
			name:           "Valid request",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			statusCode:     http.StatusOK,
			response: models.Segment{
				ID:             "segment123",
				Name:           "Test Segment",
				LedgerID:       "ledger123",
				OrganizationID: "org123",
				Status:         models.Status{Code: "active"},
			},
			expectErr: false,
		},
		{
			name:           "Missing organization ID",
			organizationID: "",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			statusCode:     http.StatusOK,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Missing ledger ID",
			organizationID: "org123",
			ledgerID:       "",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			statusCode:     http.StatusOK,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Missing portfolio ID",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "",
			segmentID:      "segment123",
			statusCode:     http.StatusOK,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Missing segment ID",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "",
			statusCode:     http.StatusOK,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Server error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			statusCode:     http.StatusInternalServerError,
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
				expectedPath := "/organizations/" + tc.organizationID + "/ledgers/" + tc.ledgerID + "/portfolios/" + tc.portfolioID + "/segments/" + tc.segmentID
				assert.Equal(t, expectedPath, r.URL.Path)

				// Return response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				if tc.response != nil {
					json.NewEncoder(w).Encode(tc.response)
				}
			}))
			defer server.Close()

			// Create segments entity
			entity := &segmentsEntity{
				httpClient: http.DefaultClient,
				authToken:  "test-token",
				baseURLs: map[string]string{
					"onboarding": server.URL,
				},
			}

			// Call GetSegment
			result, err := entity.GetSegment(context.Background(), tc.organizationID, tc.ledgerID, tc.portfolioID, tc.segmentID)

			// Check error
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "segment123", result.ID)
				assert.Equal(t, "Test Segment", result.Name)
				assert.Equal(t, "ledger123", result.LedgerID)
				assert.Equal(t, "org123", result.OrganizationID)
				assert.Equal(t, "active", result.Status.Code)
			}
		})
	}
}

func TestSegmentsEntity_CreateSegment(t *testing.T) {
	// Test cases
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		portfolioID    string
		input          *models.CreateSegmentInput
		statusCode     int
		response       interface{}
		expectErr      bool
	}{
		{
			name:           "Valid request",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			input: &models.CreateSegmentInput{
				Name: "New Segment",
			},
			statusCode: http.StatusCreated,
			response: models.Segment{
				ID:             "segment123",
				Name:           "New Segment",
				LedgerID:       "ledger123",
				OrganizationID: "org123",
				Status:         models.Status{Code: "active"},
			},
			expectErr: false,
		},
		{
			name:           "Missing organization ID",
			organizationID: "",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			input: &models.CreateSegmentInput{
				Name: "New Segment",
			},
			statusCode: http.StatusCreated,
			response:   nil,
			expectErr:  true,
		},
		{
			name:           "Missing ledger ID",
			organizationID: "org123",
			ledgerID:       "",
			portfolioID:    "portfolio123",
			input: &models.CreateSegmentInput{
				Name: "New Segment",
			},
			statusCode: http.StatusCreated,
			response:   nil,
			expectErr:  true,
		},
		{
			name:           "Missing portfolio ID",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "",
			input: &models.CreateSegmentInput{
				Name: "New Segment",
			},
			statusCode: http.StatusCreated,
			response:   nil,
			expectErr:  true,
		},
		{
			name:           "Nil input",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			input:          nil,
			statusCode:     http.StatusCreated,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Server error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			input: &models.CreateSegmentInput{
				Name: "New Segment",
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
				assert.Equal(t, http.MethodPost, r.Method)

				// Check path
				expectedPath := "/organizations/" + tc.organizationID + "/ledgers/" + tc.ledgerID + "/portfolios/" + tc.portfolioID + "/segments"
				assert.Equal(t, expectedPath, r.URL.Path)

				// Check request body if input is provided
				if tc.input != nil {
					var requestBody models.CreateSegmentInput
					json.NewDecoder(r.Body).Decode(&requestBody)
					assert.Equal(t, tc.input.Name, requestBody.Name)
				}

				// Return response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				if tc.response != nil {
					json.NewEncoder(w).Encode(tc.response)
				}
			}))
			defer server.Close()

			// Create segments entity
			entity := &segmentsEntity{
				httpClient: http.DefaultClient,
				authToken:  "test-token",
				baseURLs: map[string]string{
					"onboarding": server.URL,
				},
			}

			// Call CreateSegment
			result, err := entity.CreateSegment(context.Background(), tc.organizationID, tc.ledgerID, tc.portfolioID, tc.input)

			// Check error
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "segment123", result.ID)
				assert.Equal(t, "New Segment", result.Name)
				assert.Equal(t, "ledger123", result.LedgerID)
				assert.Equal(t, "org123", result.OrganizationID)
				assert.Equal(t, "active", result.Status.Code)
			}
		})
	}
}

func TestSegmentsEntity_UpdateSegment(t *testing.T) {
	// Test cases
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		portfolioID    string
		segmentID      string
		input          *models.UpdateSegmentInput
		statusCode     int
		response       interface{}
		expectErr      bool
	}{
		{
			name:           "Valid request",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			input: &models.UpdateSegmentInput{
				Name: "Updated Segment",
			},
			statusCode: http.StatusOK,
			response: models.Segment{
				ID:             "segment123",
				Name:           "Updated Segment",
				LedgerID:       "ledger123",
				OrganizationID: "org123",
				Status:         models.Status{Code: "active"},
			},
			expectErr: false,
		},
		{
			name:           "Missing organization ID",
			organizationID: "",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			input: &models.UpdateSegmentInput{
				Name: "Updated Segment",
			},
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:           "Missing ledger ID",
			organizationID: "org123",
			ledgerID:       "",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			input: &models.UpdateSegmentInput{
				Name: "Updated Segment",
			},
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:           "Missing portfolio ID",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "",
			segmentID:      "segment123",
			input: &models.UpdateSegmentInput{
				Name: "Updated Segment",
			},
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:           "Missing segment ID",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "",
			input: &models.UpdateSegmentInput{
				Name: "Updated Segment",
			},
			statusCode: http.StatusOK,
			response:   nil,
			expectErr:  true,
		},
		{
			name:           "Nil input",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			input:          nil,
			statusCode:     http.StatusOK,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Server error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			input: &models.UpdateSegmentInput{
				Name: "Updated Segment",
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
				expectedPath := "/organizations/" + tc.organizationID + "/ledgers/" + tc.ledgerID + "/portfolios/" + tc.portfolioID + "/segments/" + tc.segmentID
				assert.Equal(t, expectedPath, r.URL.Path)

				// Check request body if input is provided
				if tc.input != nil {
					var requestBody models.UpdateSegmentInput
					json.NewDecoder(r.Body).Decode(&requestBody)
					assert.Equal(t, tc.input.Name, requestBody.Name)
				}

				// Return response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				if tc.response != nil {
					json.NewEncoder(w).Encode(tc.response)
				}
			}))
			defer server.Close()

			// Create segments entity
			entity := &segmentsEntity{
				httpClient: http.DefaultClient,
				authToken:  "test-token",
				baseURLs: map[string]string{
					"onboarding": server.URL,
				},
			}

			// Call UpdateSegment
			result, err := entity.UpdateSegment(context.Background(), tc.organizationID, tc.ledgerID, tc.portfolioID, tc.segmentID, tc.input)

			// Check error
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, "segment123", result.ID)
				assert.Equal(t, "Updated Segment", result.Name)
				assert.Equal(t, "ledger123", result.LedgerID)
				assert.Equal(t, "org123", result.OrganizationID)
				assert.Equal(t, "active", result.Status.Code)
			}
		})
	}
}

func TestSegmentsEntity_DeleteSegment(t *testing.T) {
	// Test cases
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		portfolioID    string
		segmentID      string
		statusCode     int
		response       interface{}
		expectErr      bool
	}{
		{
			name:           "Valid request",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			statusCode:     http.StatusNoContent,
			response:       nil,
			expectErr:      false,
		},
		{
			name:           "Missing organization ID",
			organizationID: "",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			statusCode:     http.StatusNoContent,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Missing ledger ID",
			organizationID: "org123",
			ledgerID:       "",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			statusCode:     http.StatusNoContent,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Missing portfolio ID",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "",
			segmentID:      "segment123",
			statusCode:     http.StatusNoContent,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Missing segment ID",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "",
			statusCode:     http.StatusNoContent,
			response:       nil,
			expectErr:      true,
		},
		{
			name:           "Server error",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			statusCode:     http.StatusInternalServerError,
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
				assert.Equal(t, http.MethodDelete, r.Method)

				// Check path
				expectedPath := "/organizations/" + tc.organizationID + "/ledgers/" + tc.ledgerID + "/portfolios/" + tc.portfolioID + "/segments/" + tc.segmentID
				assert.Equal(t, expectedPath, r.URL.Path)

				// Return response
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				if tc.response != nil {
					json.NewEncoder(w).Encode(tc.response)
				}
			}))
			defer server.Close()

			// Create segments entity
			entity := &segmentsEntity{
				httpClient: http.DefaultClient,
				authToken:  "test-token",
				baseURLs: map[string]string{
					"onboarding": server.URL,
				},
			}

			// Call DeleteSegment
			err := entity.DeleteSegment(context.Background(), tc.organizationID, tc.ledgerID, tc.portfolioID, tc.segmentID)

			// Check error
			if tc.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSegmentsEntity_BuildURL(t *testing.T) {
	// Create segments entity
	entity := &segmentsEntity{
		httpClient: http.DefaultClient,
		authToken:  "test-token",
		baseURLs: map[string]string{
			"onboarding": "https://api.example.com",
		},
	}

	// Test cases
	testCases := []struct {
		name           string
		organizationID string
		ledgerID       string
		portfolioID    string
		segmentID      string
		expected       string
	}{
		{
			name:           "With segment ID",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "segment123",
			expected:       "https://api.example.com/organizations/org123/ledgers/ledger123/portfolios/portfolio123/segments/segment123",
		},
		{
			name:           "Without segment ID",
			organizationID: "org123",
			ledgerID:       "ledger123",
			portfolioID:    "portfolio123",
			segmentID:      "",
			expected:       "https://api.example.com/organizations/org123/ledgers/ledger123/portfolios/portfolio123/segments",
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := entity.buildURL(tc.organizationID, tc.ledgerID, tc.portfolioID, tc.segmentID)
			assert.Equal(t, tc.expected, url)
		})
	}
}

func TestSegmentsEntity_SetCommonHeaders(t *testing.T) {
	// Create segments entity
	entity := &segmentsEntity{
		httpClient: http.DefaultClient,
		authToken:  "test-token",
		baseURLs: map[string]string{
			"onboarding": "https://api.example.com",
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
