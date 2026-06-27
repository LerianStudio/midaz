// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
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

// stubInstrumentLedgerAccountReader satisfies services.LedgerAccountReader for
// the handler-level CreateInstrument tests. CreateInstrument treats the reader
// as a hard dependency, so every case must inject one; the booleans drive the
// 422 referential branches at the wire layer.
type stubInstrumentLedgerAccountReader struct {
	ledgerExists  bool
	accountExists bool
}

func (s stubInstrumentLedgerAccountReader) LedgerExists(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return s.ledgerExists, nil
}

func (s stubInstrumentLedgerAccountReader) AccountExists(_ context.Context, _, _, _ uuid.UUID) (bool, error) {
	return s.accountExists, nil
}

// CountAccountsByHolder satisfies the holder-delete ownership leg of the port;
// the CreateInstrument handler tests never delete a holder, so it reports none.
func (s stubInstrumentLedgerAccountReader) CountAccountsByHolder(_ context.Context, _, _ uuid.UUID) (int64, error) {
	return 0, nil
}

func TestInstrumentHandler_CreateInstrument(t *testing.T) {
	tests := []struct {
		name           string
		jsonBody       string
		setupMocks     func(instrumentRepo *instrument.MockRepository, holderRepo *holder.MockRepository, orgID string, holderID uuid.UUID)
		ledgerAccounts services.LedgerAccountReader
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
						a.CreatedAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
						a.UpdatedAt = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
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
		{
			name: "ledger reference not found returns 422",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002"
			}`,
			ledgerAccounts: stubInstrumentLedgerAccountReader{ledgerExists: false, accountExists: true},
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
				// No instrumentRepo.Create expectation: the create must NOT run.
			},
			expectedStatus: 422,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrInstrumentLedgerReferenceNotFound.Error(), errResp["code"])
			},
		},
		{
			name: "account reference not found returns 422",
			jsonBody: `{
				"ledgerId": "00000000-0000-0000-0000-000000000001",
				"accountId": "00000000-0000-0000-0000-000000000002"
			}`,
			ledgerAccounts: stubInstrumentLedgerAccountReader{ledgerExists: true, accountExists: false},
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
				// No instrumentRepo.Create expectation: the create must NOT run.
			},
			expectedStatus: 422,
			validateBody: func(t *testing.T, body []byte) {
				var errResp map[string]any
				err := json.Unmarshal(body, &errResp)
				require.NoError(t, err)

				assert.Contains(t, errResp, "code", "error response should contain code")
				assert.Equal(t, cn.ErrInstrumentAccountReferenceNotFound.Error(), errResp["code"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			orgUUID := uuid.New()
			orgID := orgUUID.String()
			holderID := uuid.New()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			mockHolderRepo := holder.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, mockHolderRepo, orgID, holderID)

			ledgerAccounts := tt.ledgerAccounts
			if ledgerAccounts == nil {
				ledgerAccounts = stubInstrumentLedgerAccountReader{ledgerExists: true, accountExists: true}
			}

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
				HolderRepo:     mockHolderRepo,
				LedgerAccounts: ledgerAccounts,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/holders/:holder_id/instruments",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("organization_id", orgUUID)
					return c.Next()
				},
				http.WithBody(new(mmodel.CreateInstrumentInput), handler.CreateInstrument),
			)

			req := httptest.NewRequest("POST", "/v1/organizations/"+orgID+"/holders/"+holderID.String()+"/instruments", bytes.NewBufferString(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			require.NoError(t, err)

			defer resp.Body.Close()
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
						CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
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
						CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
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

			orgUUID := uuid.New()
			orgID := orgUUID.String()
			holderID := uuid.New()
			instrumentID := uuid.New()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, orgID, holderID, instrumentID)

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("instrument_id", instrumentID)
					c.Locals("organization_id", orgUUID)
					return c.Next()
				},
				handler.GetInstrumentByID,
			)

			url := "/v1/organizations/" + orgID + "/holders/" + holderID.String() + "/instruments/" + instrumentID.String()
			if tt.includeDeleted != "" {
				url += "?include_deleted=" + tt.includeDeleted
			}
			req := httptest.NewRequest("GET", url, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)

			defer resp.Body.Close()
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
						CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
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

			orgUUID := uuid.New()
			orgID := orgUUID.String()
			holderID := uuid.New()
			instrumentID := uuid.New()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, orgID, holderID, instrumentID)

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("instrument_id", instrumentID)
					c.Locals("organization_id", orgUUID)
					return c.Next()
				},
				http.WithBody(new(mmodel.UpdateInstrumentInput), handler.UpdateInstrument),
			)

			req := httptest.NewRequest("PATCH", "/v1/organizations/"+orgID+"/holders/"+holderID.String()+"/instruments/"+instrumentID.String(), bytes.NewBufferString(tt.jsonBody))
			req.Header.Set("Content-Type", "application/json")
			resp, err := app.Test(req)

			require.NoError(t, err)

			defer resp.Body.Close()
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

			orgUUID := uuid.New()
			orgID := orgUUID.String()
			holderID := uuid.New()
			instrumentID := uuid.New()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, orgID, holderID, instrumentID)

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("instrument_id", instrumentID)
					c.Locals("organization_id", orgUUID)
					return c.Next()
				},
				handler.DeleteInstrumentByID,
			)

			url := "/v1/organizations/" + orgID + "/holders/" + holderID.String() + "/instruments/" + instrumentID.String()
			if tt.hardDelete != "" {
				url += "?hard_delete=" + tt.hardDelete
			}
			req := httptest.NewRequest("DELETE", url, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)

			defer resp.Body.Close()
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

			orgUUID := uuid.New()
			orgID := orgUUID.String()
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
			app.Delete("/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id/related-parties/:related_party_id",
				func(c *fiber.Ctx) error {
					c.Locals("holder_id", holderID)
					c.Locals("instrument_id", instrumentID)
					c.Locals("related_party_id", relatedPartyID)
					c.Locals("organization_id", orgUUID)
					return c.Next()
				},
				handler.DeleteRelatedParty,
			)

			url := "/v1/organizations/" + orgID + "/holders/" + holderID.String() + "/instruments/" + instrumentID.String() + "/related-parties/" + relatedPartyID.String()
			req := httptest.NewRequest("DELETE", url, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)

			defer resp.Body.Close()
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
							CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
							UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						},
						{
							ID:        &instrument2ID,
							LedgerID:  &ledgerID,
							AccountID: &accountID,
							HolderID:  &holderID,
							Document:  &document,
							Type:      &holderType,
							CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
							UpdatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
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
			// ledger_id and holder_id remain query-string list filters (they did
			// not move to the path). Assert the parsed holder UUID and the ledger
			// filter flow through to the service.
			name:        "holder_id and ledger_id query filters flow to the service",
			queryParams: "?holder_id=11111111-1111-1111-1111-111111111111&ledger_id=22222222-2222-2222-2222-222222222222",
			setupMocks: func(instrumentRepo *instrument.MockRepository, orgID string) {
				filterHolderID := uuid.MustParse("11111111-1111-1111-1111-111111111111")

				instrumentRepo.EXPECT().
					FindAll(gomock.Any(), orgID, filterHolderID, gomock.Cond(func(x any) bool {
						filter, ok := x.(http.QueryHeader)
						if !ok {
							return false
						}

						return filter.LedgerID != nil && *filter.LedgerID == "22222222-2222-2222-2222-222222222222" &&
							filter.HolderID != nil && *filter.HolderID == "11111111-1111-1111-1111-111111111111"
					}), false).
					Return([]*mmodel.Instrument{}, nil).
					Times(1)
			},
			expectedStatus: 200,
			validateBody: func(t *testing.T, body []byte) {
				var result map[string]any
				err := json.Unmarshal(body, &result)
				require.NoError(t, err)

				assert.Contains(t, result, "items", "response should contain items")
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

			orgUUID := uuid.New()
			orgID := orgUUID.String()

			mockInstrumentRepo := instrument.NewMockRepository(ctrl)
			tt.setupMocks(mockInstrumentRepo, orgID)

			uc := &services.UseCase{
				InstrumentRepo: mockInstrumentRepo,
			}
			handler := &InstrumentHandler{Service: uc}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/instruments",
				func(c *fiber.Ctx) error {
					c.Locals("organization_id", orgUUID)
					return c.Next()
				},
				handler.GetAllInstruments,
			)

			req := httptest.NewRequest("GET", "/v1/organizations/"+orgID+"/instruments"+tt.queryParams, nil)
			resp, err := app.Test(req)

			require.NoError(t, err)

			defer resp.Body.Close()
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validateBody != nil {
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				tt.validateBody(t, body)
			}
		})
	}
}
