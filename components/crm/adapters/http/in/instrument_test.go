// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/components/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v3/components/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v3/components/crm/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// TestInstrumentEntityFieldContract locks the R43 contract: the typed business
// error built for the Instrument entity carries EntityType "Instrument" (the
// renamed value), not the former "Alias". This is the layer where the flip is
// observable — the CRM HTTP envelope (code/title/message) deliberately omits the
// entity field, so the typed error is the right place to pin it.
func TestInstrumentEntityFieldContract(t *testing.T) {
	err := pkg.ValidateBusinessError(cn.ErrInstrumentNotFound, reflect.TypeOf(mmodel.Instrument{}).Name())

	notFound, ok := err.(pkg.EntityNotFoundError)
	require.True(t, ok, "ErrInstrumentNotFound must map to EntityNotFoundError")
	assert.Equal(t, cn.EntityInstrument, notFound.EntityType,
		"entity field must reflect the renamed Instrument entity")
	assert.Equal(t, cn.ErrInstrumentNotFound.Error(), notFound.Code)
}

func TestInstrumentHandler_CreateInstrument(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 201 with created instrument",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002"
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				document := "12345678901"
				holderType := "individual"

				holderRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, false).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &document,
						Type:     &holderType,
					}, nil).
					Times(1)

				instrumentRepo.EXPECT().
					Create(gomock.Any(), orgID, gomock.Any()).
					DoAndReturn(func(ctx any, org string, a *mmodel.Instrument) (*mmodel.Instrument, error) {
						a.CreatedAt = time.Now()
						a.UpdatedAt = time.Now()
						return a, nil
					}).
					Times(1)
			},
			expectedStatus: 201,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "ledgerId", "response should contain ledgerId")
				assert.Equal(t, "00000000-0000-0000-0000-000000000001", result["ledgerId"])
			},
		},
		{
			name: "holder not found returns 404",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002"
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				holderRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, false).
					Return(nil, pkg.ValidateBusinessError(cn.ErrHolderNotFound, reflect.TypeOf(mmodel.Holder{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrHolderNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002"
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				document := "12345678901"
				holderType := "individual"

				holderRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, false).
					Return(&mmodel.Holder{
						ID:       &holderID,
						Document: &document,
						Type:     &holderType,
					}, nil).
					Times(1)

				instrumentRepo.EXPECT().
					Create(gomock.Any(), orgID, gomock.Any()).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name: "missing ledgerId returns 400",
			jsonBody: `{
				"accountId": "00000000-0000-0000-0000-000000000002"
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				// No mock calls expected - validation fails before service layer
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name: "missing accountId returns 400",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001"
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				// No mock calls expected - validation fails before service layer
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name: "invalid related party role returns 400",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002",
				"relatedParties": [{
					"document": "12345678900",
					"name": "John Smith",
					"role": "INVALID_ROLE",
					"startDate": "2025-01-01"
				}]
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				// No mock calls expected - validation fails before any repository call
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrInvalidRelatedPartyRole.Error(), errResp["code"])
			},
		},
		{
			name: "empty related party document returns 400",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002",
				"relatedParties": [{
					"document": "",
					"name": "John Smith",
					"role": "PRIMARY_HOLDER",
					"startDate": "2025-01-01"
				}]
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				// No mock calls expected - validation fails before any repository call
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrRelatedPartyDocumentRequired.Error(), errResp["code"])
			},
		},
		{
			name: "empty related party name returns 400",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002",
				"relatedParties": [{
					"document": "12345678900",
					"name": "",
					"role": "PRIMARY_HOLDER",
					"startDate": "2025-01-01"
				}]
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				// No mock calls expected - validation fails before any repository call
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrRelatedPartyNameRequired.Error(), errResp["code"])
			},
		},
		{
			name: "missing related party start date returns 400",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002",
				"relatedParties": [{
					"document": "12345678900",
					"name": "John Smith",
					"role": "PRIMARY_HOLDER"
				}]
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				// No mock calls expected - validation fails before any repository call
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrRelatedPartyStartDateRequired.Error(), errResp["code"])
			},
		},
		{
			name: "related party end date before start date returns 400",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002",
				"relatedParties": [{
					"document": "12345678900",
					"name": "John Smith",
					"role": "PRIMARY_HOLDER",
					"startDate": "2025-06-01",
					"endDate": "2025-01-01"
				}]
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID) {
				// No mock calls expected - validation fails before any repository call
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrRelatedPartyEndDateInvalid.Error(), errResp["code"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New().String()
			holderID := uuid.New()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			mockHolderRepo := holder.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, mockHolderRepo, orgID, holderID)

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
				HolderRepo:     mockHolderRepo,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Post("/v1/holders/:holder_id/instruments",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				http.WithBody(new(mmodel.CreateInstrumentInput), handler.CreateInstrument),
			)

			req := httptest.NewRequest("POST", "/v1/holders/"+holderID.String()+"/instruments", bytes.NewBufferString(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Organization-Id", orgID)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestInstrumentHandler_GetInstrumentByID(t *testing.T) {
	tests := []struct {
		name           string
		includeDeleted string
		setupMocks     func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:           "success returns 200 with instrument",
			includeDeleted: "",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				ledgerID := "00000000-0000-0000-0000-000000000001"
				accountID := "00000000-0000-0000-0000-000000000002"
				document := "12345678901"
				holderType := "individual"

				instrumentRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, instrumentID, false).
					Return(&mmodel.Instrument{
						ID:        &instrumentID,
						LedgerID:  &ledgerID,
						AccountID: &accountID,
						HolderID:  &holderID,
						Document:  &document,
						Type:      &holderType,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "ledgerId", "response should contain ledgerId")
				assert.Contains(t, result, "accountId", "response should contain accountId")
			},
		},
		{
			name:           "success with include_deleted returns 200 with instrument",
			includeDeleted: "true",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				ledgerID := "00000000-0000-0000-0000-000000000001"
				accountID := "00000000-0000-0000-0000-000000000002"
				document := "12345678901"
				holderType := "individual"

				instrumentRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, instrumentID, true).
					Return(&mmodel.Instrument{
						ID:        &instrumentID,
						LedgerID:  &ledgerID,
						AccountID: &accountID,
						HolderID:  &holderID,
						Document:  &document,
						Type:      &holderType,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "ledgerId", "response should contain ledgerId")
				assert.Contains(t, result, "accountId", "response should contain accountId")
			},
		},
		{
			name:           "not found returns 404",
			includeDeleted: "",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				instrumentRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, instrumentID, false).
					Return(nil, pkg.ValidateBusinessError(cn.ErrInstrumentNotFound, reflect.TypeOf(mmodel.Instrument{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrInstrumentNotFound.Error(), errResp["code"])
			},
		},
		{
			name:           "repository error returns 500",
			includeDeleted: "",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				instrumentRepo.EXPECT().
					Find(gomock.Any(), orgID, holderID, instrumentID, false).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New().String()
			holderID := uuid.New()
			instrumentID := uuid.New()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, orgID, holderID, instrumentID)

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Get("/v1/holders/:holder_id/instruments/:instrument_id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("instrument_id", instrumentID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.GetInstrumentByID,
			)

			url := "/v1/holders/" + holderID.String() + "/instruments/" + instrumentID.String()
			if tt.includeDeleted != "" {
				url += "?include_deleted=" + tt.includeDeleted
			}
			req := httptest.NewRequest("GET", url, nil)
			req.Header.Set("X-Organization-Id", orgID)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestInstrumentHandler_UpdateInstrument(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 200 with updated instrument",
			jsonBody: `{
				"metadata": {"key": "value"}
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				ledgerID := "00000000-0000-0000-0000-000000000001"
				accountID := "00000000-0000-0000-0000-000000000002"
				document := "12345678901"
				holderType := "individual"

				instrumentRepo.EXPECT().
					Update(gomock.Any(), orgID, holderID, instrumentID, gomock.Any(), gomock.Any()).
					Return(&mmodel.Instrument{
						ID:        &instrumentID,
						LedgerID:  &ledgerID,
						AccountID: &accountID,
						HolderID:  &holderID,
						Document:  &document,
						Type:      &holderType,
						Metadata:  map[string]any{"key": "value"},
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "id", "response should contain id")
				assert.Contains(t, result, "metadata", "response should contain metadata")
				metadata, ok := result["metadata"].(map[string]any)
				require.True(t, ok, "metadata should be an object")
				assert.Equal(t, "value", metadata["key"])
			},
		},
		{
			name: "not found returns 404",
			jsonBody: `{
				"metadata": {"key": "value"}
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				instrumentRepo.EXPECT().
					Update(gomock.Any(), orgID, holderID, instrumentID, gomock.Any(), gomock.Any()).
					Return(nil, pkg.ValidateBusinessError(cn.ErrInstrumentNotFound, reflect.TypeOf(mmodel.Instrument{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrInstrumentNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			jsonBody: `{
				"metadata": {"key": "value"}
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				instrumentRepo.EXPECT().
					Update(gomock.Any(), orgID, holderID, instrumentID, gomock.Any(), gomock.Any()).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name: "invalid related party role returns 400",
			jsonBody: `{
				"relatedParties": [{
					"document": "12345678900",
					"name": "John Smith",
					"role": "INVALID_ROLE",
					"startDate": "2025-01-01"
				}]
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				// No mock calls expected - validation fails before repository call
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrInvalidRelatedPartyRole.Error(), errResp["code"])
			},
		},
		{
			name: "empty related party document returns 400",
			jsonBody: `{
				"relatedParties": [{
					"document": "",
					"name": "John Smith",
					"role": "PRIMARY_HOLDER",
					"startDate": "2025-01-01"
				}]
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				// No mock calls expected - validation fails before repository call
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrRelatedPartyDocumentRequired.Error(), errResp["code"])
			},
		},
		{
			name: "empty related party name returns 400",
			jsonBody: `{
				"relatedParties": [{
					"document": "12345678900",
					"name": "",
					"role": "PRIMARY_HOLDER",
					"startDate": "2025-01-01"
				}]
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				// No mock calls expected - validation fails before repository call
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrRelatedPartyNameRequired.Error(), errResp["code"])
			},
		},
		{
			name: "missing related party start date returns 400",
			jsonBody: `{
				"relatedParties": [{
					"document": "12345678900",
					"name": "John Smith",
					"role": "PRIMARY_HOLDER"
				}]
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				// No mock calls expected - validation fails before repository call
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrRelatedPartyStartDateRequired.Error(), errResp["code"])
			},
		},
		{
			name: "related party end date before start date returns 400",
			jsonBody: `{
				"relatedParties": [{
					"document": "12345678900",
					"name": "John Smith",
					"role": "PRIMARY_HOLDER",
					"startDate": "2025-06-01",
					"endDate": "2025-01-01"
				}]
			}`,
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				// No mock calls expected - validation fails before repository call
			},
			expectedStatus: 400,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrRelatedPartyEndDateInvalid.Error(), errResp["code"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New().String()
			holderID := uuid.New()
			instrumentID := uuid.New()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, orgID, holderID, instrumentID)

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Patch("/v1/holders/:holder_id/instruments/:instrument_id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("instrument_id", instrumentID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				http.WithBody(new(mmodel.UpdateInstrumentInput), handler.UpdateInstrument),
			)

			req := httptest.NewRequest("PATCH", "/v1/holders/"+holderID.String()+"/instruments/"+instrumentID.String(), bytes.NewBufferString(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Organization-Id", orgID)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestInstrumentHandler_DeleteInstrumentByID(t *testing.T) {
	tests := []struct {
		name           string
		hardDelete     string
		setupMocks     func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:       "success returns 204 no content",
			hardDelete: "",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				instrumentRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, instrumentID, false).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil,
		},
		{
			name:       "success with hard delete returns 204",
			hardDelete: "true",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				instrumentRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, instrumentID, true).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil,
		},
		{
			name:       "not found returns 404",
			hardDelete: "",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				instrumentRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, instrumentID, false).
					Return(pkg.ValidateBusinessError(cn.ErrInstrumentNotFound, reflect.TypeOf(mmodel.Instrument{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrInstrumentNotFound.Error(), errResp["code"])
			},
		},
		{
			name:       "repository error returns 500",
			hardDelete: "",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID uuid.UUID) {
				instrumentRepo.EXPECT().
					Delete(gomock.Any(), orgID, holderID, instrumentID, false).
					Return(pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New().String()
			holderID := uuid.New()
			instrumentID := uuid.New()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, orgID, holderID, instrumentID)

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Delete("/v1/holders/:holder_id/instruments/:instrument_id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("instrument_id", instrumentID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.DeleteInstrumentByID,
			)

			url := "/v1/holders/" + holderID.String() + "/instruments/" + instrumentID.String()
			if tt.hardDelete != "" {
				url += "?hard_delete=" + tt.hardDelete
			}
			req := httptest.NewRequest("DELETE", url, nil)
			req.Header.Set("X-Organization-Id", orgID)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestInstrumentHandler_DeleteRelatedParty(t *testing.T) {
	tests := []struct {
		name           string
		setupMocks     func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID, relatedPartyID uuid.UUID)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name: "success returns 204 no content",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID, relatedPartyID uuid.UUID) {
				instrumentRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), orgID, holderID, instrumentID, relatedPartyID).
					Return(nil).
					Times(1)
			},
			expectedStatus: 204,
			validateBody:   nil,
		},
		{
			name: "not found returns 404",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID, relatedPartyID uuid.UUID) {
				instrumentRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), orgID, holderID, instrumentID, relatedPartyID).
					Return(pkg.ValidateBusinessError(cn.ErrInstrumentNotFound, reflect.TypeOf(mmodel.Instrument{}).Name())).
					Times(1)
			},
			expectedStatus: 404,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrInstrumentNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "repository error returns 500",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string, holderID, instrumentID, relatedPartyID uuid.UUID) {
				instrumentRepo.EXPECT().
					DeleteRelatedParty(gomock.Any(), orgID, holderID, instrumentID, relatedPartyID).
					Return(pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New().String()
			holderID := uuid.New()
			instrumentID := uuid.New()
			relatedPartyID := uuid.New()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, orgID, holderID, instrumentID, relatedPartyID)

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Delete("/v1/holders/:holder_id/instruments/:instrument_id/related-parties/:related_party_id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("instrument_id", instrumentID)
					c.Locals("related_party_id", relatedPartyID)
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.DeleteRelatedParty,
			)

			url := "/v1/holders/" + holderID.String() + "/instruments/" + instrumentID.String() + "/related-parties/" + relatedPartyID.String()
			req := httptest.NewRequest("DELETE", url, nil)
			req.Header.Set("X-Organization-Id", orgID)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func TestInstrumentHandler_GetAllInstruments(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMocks     func(instrumentRepo *instrument.MockRepository, orgID string)
		expectedStatus int
		validateBody   func(t *testing.T, body []byte)
	}{
		{
			name:        "empty list returns 200 with pagination structure",
			queryParams: "",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string) {
				instrumentRepo.EXPECT().
					FindAll(gomock.Any(), orgID, uuid.Nil, gomock.Any(), false).
					Return([]*mmodel.Instrument{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(10), limit)

				page, ok := result["page"].(float64)
				require.True(t, ok, "page should be a number")
				assert.Equal(t, float64(1), page)
			},
		},
		{
			name:        "success with items returns instruments",
			queryParams: "?limit=5&page=1",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string) {
				instrument1ID := uuid.New()
				instrument2ID := uuid.New()
				holderID := uuid.New()
				ledgerID := "00000000-0000-0000-0000-000000000001"
				accountID := "00000000-0000-0000-0000-000000000002"
				document := "12345678901"
				holderType := "individual"

				instrumentRepo.EXPECT().
					FindAll(gomock.Any(), orgID, uuid.Nil, gomock.Any(), false).
					Return([]*mmodel.Instrument{
						{
							ID:        &instrument1ID,
							LedgerID:  &ledgerID,
							AccountID: &accountID,
							HolderID:  &holderID,
							Document:  &document,
							Type:      &holderType,
							CreatedAt: time.Now(),
							UpdatedAt: time.Now(),
						},
						{
							ID:        &instrument2ID,
							LedgerID:  &ledgerID,
							AccountID: &accountID,
							HolderID:  &holderID,
							Document:  &document,
							Type:      &holderType,
							CreatedAt: time.Now(),
							UpdatedAt: time.Now(),
						},
					}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				items, ok := result["items"].([]any)
				require.True(t, ok, "items should be an array")
				assert.Len(t, items, 2, "should have two instruments")

				firstItem, ok := items[0].(map[string]any)
				require.True(t, ok, "item should be an object")
				assert.Contains(t, firstItem, "id", "instrument should have id field")
				assert.Contains(t, firstItem, "ledgerId", "instrument should have ledgerId field")

				limit, ok := result["limit"].(float64)
				require.True(t, ok, "limit should be a number")
				assert.Equal(t, float64(5), limit)
			},
		},
		{
			name:        "repository error returns 500",
			queryParams: "",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string) {
				instrumentRepo.EXPECT().
					FindAll(gomock.Any(), orgID, uuid.Nil, gomock.Any(), false).
					Return(nil, pkg.InternalServerError{
						Code:    "0046",
						Title:   "Internal Server Error",
						Message: "Database connection failed",
					}).
					Times(1)
			},
			expectedStatus: 500,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Contains(t, errResp, "message", "error response should contain message")
			},
		},
		{
			name:        "zero limit returns 400",
			queryParams: "?limit=0",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string) {
			},
			expectedStatus: 400,
			validateBody:   assertInvalidQueryParameterResponse,
		},
		{
			name:        "negative limit returns 400",
			queryParams: "?limit=-1",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string) {
			},
			expectedStatus: 400,
			validateBody:   assertInvalidQueryParameterResponse,
		},
		{
			name:        "non-numeric limit returns 400",
			queryParams: "?limit=abc",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string) {
			},
			expectedStatus: 400,
			validateBody:   assertInvalidQueryParameterResponse,
		},
		{
			name:        "zero page returns 400",
			queryParams: "?page=0",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string) {
			},
			expectedStatus: 400,
			validateBody:   assertInvalidQueryParameterResponse,
		},
		{
			name:        "negative page returns 400",
			queryParams: "?page=-1",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string) {
			},
			expectedStatus: 400,
			validateBody:   assertInvalidQueryParameterResponse,
		},
		{
			name:        "non-numeric page returns 400",
			queryParams: "?page=abc",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string) {
			},
			expectedStatus: 400,
			validateBody:   assertInvalidQueryParameterResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgID := uuid.New().String()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, orgID)

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Get("/v1/instruments",
				func(c *fiber.Ctx) error {
					c.Request().Header.Set("X-Organization-Id", orgID)
					return c.Next()
				},
				handler.GetAllInstruments,
			)

			req := httptest.NewRequest("GET", "/v1/instruments"+tt.queryParams, nil)
			req.Header.Set("X-Organization-Id", orgID)
			resp, err := app.Test(req)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}

func assertInvalidQueryParameterResponse(t *testing.T, body []byte) {
	t.Helper()

	var errResp map[string]any
	err := json.Unmarshal(body, &errResp)
	require.NoError(t, err)

	assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), errResp["code"])
	assert.Equal(t, "Invalid Query Parameter", errResp["title"])
	assert.Contains(t, errResp["message"], "query parameters")
}
