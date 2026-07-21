// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readBody drains and unmarshals a JSON response body into a generic map.
func readBody(t *testing.T, resp *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &out))

	return out
}

// decodeJSON drains and unmarshals a real *http.Response body (from app.Test).
func decodeJSON(t *testing.T, body io.ReadCloser) map[string]any {
	t.Helper()

	raw, err := io.ReadAll(body)
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out))

	return out
}

// ============================================================================
// BillingPackageHandler — stub + tests
// ============================================================================

// stubBillingPackageService is a hand-written stub for BillingPackageUseCase.
// It records the arguments the handler forwards and returns canned values so
// each test asserts both the wire response and the handler→service contract.
type stubBillingPackageService struct {
	createResult *model.BillingPackage
	createErr    error

	getByIDResult *model.BillingPackage
	getByIDErr    error

	getAllResult []*model.BillingPackage
	getAllTotal  int64
	getAllErr    error

	updateResult *model.BillingPackage
	updateErr    error

	deleteErr error

	// captured args
	gotCreate        *model.BillingPackage
	gotGetByIDID     uuid.UUID
	gotGetByIDOrg    uuid.UUID
	gotGetAllOrg     uuid.UUID
	gotGetAllLedger  *uuid.UUID
	gotGetAllType    string
	gotGetAllLimit   int
	gotGetAllPage    int
	gotUpdateID      uuid.UUID
	gotUpdateOrg     uuid.UUID
	gotUpdateUpdates map[string]any
	gotDeleteID      uuid.UUID
	gotDeleteOrg     uuid.UUID
	createCalled     bool
	updateCalled     bool
	deleteCalled     bool
}

func (s *stubBillingPackageService) CreateBillingPackage(_ context.Context, bp *model.BillingPackage) (*model.BillingPackage, error) {
	s.createCalled = true
	s.gotCreate = bp

	return s.createResult, s.createErr
}

func (s *stubBillingPackageService) GetBillingPackageByID(_ context.Context, id, organizationID uuid.UUID) (*model.BillingPackage, error) {
	s.gotGetByIDID = id
	s.gotGetByIDOrg = organizationID

	return s.getByIDResult, s.getByIDErr
}

func (s *stubBillingPackageService) GetAllBillingPackages(_ context.Context, organizationID uuid.UUID, ledgerID *uuid.UUID, billingType string, limit, page int) ([]*model.BillingPackage, int64, error) {
	s.gotGetAllOrg = organizationID
	s.gotGetAllLedger = ledgerID
	s.gotGetAllType = billingType
	s.gotGetAllLimit = limit
	s.gotGetAllPage = page

	return s.getAllResult, s.getAllTotal, s.getAllErr
}

func (s *stubBillingPackageService) UpdateBillingPackage(_ context.Context, id, organizationID uuid.UUID, updates map[string]any) (*model.BillingPackage, error) {
	s.updateCalled = true
	s.gotUpdateID = id
	s.gotUpdateOrg = organizationID
	s.gotUpdateUpdates = updates

	return s.updateResult, s.updateErr
}

func (s *stubBillingPackageService) DeleteBillingPackage(_ context.Context, id, organizationID uuid.UUID) error {
	s.deleteCalled = true
	s.gotDeleteID = id
	s.gotDeleteOrg = organizationID

	return s.deleteErr
}

func TestBillingPackageHandler_CreateBillingPackage(t *testing.T) {
	orgUUID := uuid.New()

	tests := []struct {
		name           string
		payload        any
		stub           *stubBillingPackageService
		expectedStatus int
		validate       func(t *testing.T, resp *httptest.ResponseRecorder, stub *stubBillingPackageService)
	}{
		{
			name:    "success returns 201 and stamps org id onto payload",
			payload: &model.BillingPackage{Label: "Monthly Volume", Type: "volume", LedgerID: uuid.NewString()},
			stub: &stubBillingPackageService{
				createResult: &model.BillingPackage{ID: uuid.NewString(), Label: "Monthly Volume", Type: "volume"},
			},
			expectedStatus: fiber.StatusCreated,
			validate: func(t *testing.T, resp *httptest.ResponseRecorder, stub *stubBillingPackageService) {
				require.True(t, stub.createCalled, "service.CreateBillingPackage must be invoked")
				assert.Equal(t, orgUUID.String(), stub.gotCreate.OrganizationID,
					"handler must overwrite payload OrganizationID with the path org")

				body := readBody(t, resp)
				assert.Equal(t, "Monthly Volume", body["label"])
				assert.Equal(t, "volume", body["type"])
			},
		},
		{
			name:           "wrong payload type returns 500 internal error",
			payload:        &model.BillingPackageUpdate{}, // not *model.BillingPackage
			stub:           &stubBillingPackageService{},
			expectedStatus: fiber.StatusInternalServerError,
			validate: func(t *testing.T, _ *httptest.ResponseRecorder, stub *stubBillingPackageService) {
				assert.False(t, stub.createCalled, "service must not be called on a type-assertion failure")
			},
		},
		{
			name:    "conflict from service maps to 409",
			payload: &model.BillingPackage{Label: "Dup", Type: "volume", LedgerID: uuid.NewString()},
			stub: &stubBillingPackageService{
				createErr: pkg.ValidateBusinessError(cn.ErrBillingRouteOverlap, cn.EntityBillingPackage),
			},
			expectedStatus: fiber.StatusConflict,
			validate: func(t *testing.T, resp *httptest.ResponseRecorder, _ *stubBillingPackageService) {
				assert.Equal(t, cn.ErrBillingRouteOverlap.Error(), readBody(t, resp)["code"])
			},
		},
		{
			name:           "nil result with nil error returns 500",
			payload:        &model.BillingPackage{Label: "X", Type: "volume", LedgerID: uuid.NewString()},
			stub:           &stubBillingPackageService{createResult: nil, createErr: nil},
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &BillingPackageHandler{Service: tt.stub}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/billing-packages", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)

				return handler.CreateBillingPackage(tt.payload, c)
			})

			req := httptest.NewRequest(fiber.MethodPost, "/v1/organizations/"+orgUUID.String()+"/billing-packages", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			rec := httptest.NewRecorder()
			body, _ := io.ReadAll(resp.Body)
			rec.Body.Write(body)

			if tt.validate != nil {
				tt.validate(t, rec, tt.stub)
			}
		})
	}
}

func TestBillingPackageHandler_GetAllBillingPackages(t *testing.T) {
	orgUUID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name           string
		query          string
		stub           *stubBillingPackageService
		expectedStatus int
		validate       func(t *testing.T, body map[string]any, stub *stubBillingPackageService)
	}{
		{
			name:  "success forwards defaults and filters, returns paginated items",
			query: "?ledgerId=" + ledgerID.String() + "&type=volume&limit=25&page=2",
			stub: &stubBillingPackageService{
				getAllResult: []*model.BillingPackage{{ID: uuid.NewString(), Label: "P1"}},
				getAllTotal:  1,
			},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, body map[string]any, stub *stubBillingPackageService) {
				assert.Equal(t, orgUUID, stub.gotGetAllOrg)
				require.NotNil(t, stub.gotGetAllLedger, "ledgerId must be forwarded as a non-nil pointer when present")
				assert.Equal(t, ledgerID, *stub.gotGetAllLedger)
				assert.Equal(t, "volume", stub.gotGetAllType)
				assert.Equal(t, 25, stub.gotGetAllLimit)
				assert.Equal(t, 2, stub.gotGetAllPage)

				items, ok := body["items"].([]any)
				require.True(t, ok, "response must carry an items array")
				assert.Len(t, items, 1)
			},
		},
		{
			name:           "missing limit/page defaults to 10/1",
			query:          "",
			stub:           &stubBillingPackageService{getAllResult: []*model.BillingPackage{}},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, _ map[string]any, stub *stubBillingPackageService) {
				assert.Equal(t, 10, stub.gotGetAllLimit)
				assert.Equal(t, 1, stub.gotGetAllPage)
			},
		},
		{
			name:           "non-uuid ledgerId rejected with 400 before service call",
			query:          "?ledgerId=not-a-uuid",
			stub:           &stubBillingPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, stub *stubBillingPackageService) {
				assert.Equal(t, uuid.Nil, stub.gotGetAllOrg, "service must not be called when ledgerId is malformed")
				assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), body["code"])
			},
		},
		{
			name:           "non-numeric limit rejected with 400",
			query:          "?limit=abc",
			stub:           &stubBillingPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, _ *stubBillingPackageService) {
				assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), body["code"])
			},
		},
		{
			name:           "limit below 1 rejected with 400",
			query:          "?limit=0",
			stub:           &stubBillingPackageService{},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:           "limit above 100 rejected with pagination-limit-exceeded",
			query:          "?limit=101",
			stub:           &stubBillingPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, _ *stubBillingPackageService) {
				assert.Equal(t, cn.ErrPaginationLimitExceeded.Error(), body["code"])
			},
		},
		{
			name:           "non-numeric page rejected with 400",
			query:          "?page=xyz",
			stub:           &stubBillingPackageService{},
			expectedStatus: fiber.StatusBadRequest,
		},
		{
			name:           "service error maps to 500",
			query:          "",
			stub:           &stubBillingPackageService{getAllErr: pkg.ValidateBusinessError(cn.ErrBillingCalculationFailed, cn.EntityBillingPackage, "boom")},
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &BillingPackageHandler{Service: tt.stub}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/billing-packages", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)

				return handler.GetAllBillingPackages(c)
			})

			req := httptest.NewRequest(fiber.MethodGet, "/v1/organizations/"+orgUUID.String()+"/billing-packages"+tt.query, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validate != nil {
				tt.validate(t, decodeJSON(t, resp.Body), tt.stub)
			}
		})
	}
}

func TestBillingPackageHandler_GetBillingPackageByID(t *testing.T) {
	orgUUID := uuid.New()
	id := uuid.New()

	tests := []struct {
		name           string
		stub           *stubBillingPackageService
		expectedStatus int
		validate       func(t *testing.T, body map[string]any, stub *stubBillingPackageService)
	}{
		{
			name: "success returns 200 and forwards id+org",
			stub: &stubBillingPackageService{
				getByIDResult: &model.BillingPackage{ID: id.String(), Label: "Found"},
			},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, body map[string]any, stub *stubBillingPackageService) {
				assert.Equal(t, id, stub.gotGetByIDID)
				assert.Equal(t, orgUUID, stub.gotGetByIDOrg)
				assert.Equal(t, "Found", body["label"])
			},
		},
		{
			name:           "not found maps to 404",
			stub:           &stubBillingPackageService{getByIDErr: pkg.ValidateBusinessError(cn.ErrBillingPackageNotFound, cn.EntityBillingPackage, id.String())},
			expectedStatus: fiber.StatusNotFound,
			validate: func(t *testing.T, body map[string]any, _ *stubBillingPackageService) {
				assert.Equal(t, cn.ErrBillingPackageNotFound.Error(), body["code"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &BillingPackageHandler{Service: tt.stub}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/billing-packages/:id", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)
				c.Locals("id", id)

				return handler.GetBillingPackageByID(c)
			})

			req := httptest.NewRequest(fiber.MethodGet, "/v1/organizations/"+orgUUID.String()+"/billing-packages/"+id.String(), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validate != nil {
				tt.validate(t, decodeJSON(t, resp.Body), tt.stub)
			}
		})
	}
}

func TestBillingPackageHandler_UpdateBillingPackage(t *testing.T) {
	orgUUID := uuid.New()
	id := uuid.New()

	tests := []struct {
		name           string
		payload        any
		stub           *stubBillingPackageService
		expectedStatus int
		validate       func(t *testing.T, body map[string]any, stub *stubBillingPackageService)
	}{
		{
			name:    "success returns 200 and forwards only provided fields",
			payload: &model.BillingPackageUpdate{Label: ptr("New Label"), Enable: ptr(false)},
			stub: &stubBillingPackageService{
				updateResult: &model.BillingPackage{ID: id.String(), Label: "New Label"},
			},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, body map[string]any, stub *stubBillingPackageService) {
				require.True(t, stub.updateCalled)
				assert.Equal(t, id, stub.gotUpdateID)
				assert.Equal(t, orgUUID, stub.gotUpdateOrg)
				// Only label + enable were set; description must be absent from the map.
				assert.Equal(t, "New Label", stub.gotUpdateUpdates["label"])
				assert.Equal(t, false, stub.gotUpdateUpdates["enable"])
				assert.NotContains(t, stub.gotUpdateUpdates, "description",
					"nil PATCH fields must not appear in the update map")
				assert.Equal(t, "New Label", body["label"])
			},
		},
		{
			name:    "empty-string description clears value (PATCH != nil semantics)",
			payload: &model.BillingPackageUpdate{Description: ptr("")},
			stub: &stubBillingPackageService{
				updateResult: &model.BillingPackage{ID: id.String()},
			},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, _ map[string]any, stub *stubBillingPackageService) {
				val, present := stub.gotUpdateUpdates["description"]
				require.True(t, present, "an explicit empty-string description must be forwarded to clear the value")
				assert.Equal(t, "", val)
			},
		},
		{
			name:           "wrong payload type returns 500",
			payload:        &model.BillingPackage{},
			stub:           &stubBillingPackageService{},
			expectedStatus: fiber.StatusInternalServerError,
			validate: func(t *testing.T, _ map[string]any, stub *stubBillingPackageService) {
				assert.False(t, stub.updateCalled)
			},
		},
		{
			name:           "blank label fails validation with 400 before service call",
			payload:        &model.BillingPackageUpdate{Label: ptr("   ")},
			stub:           &stubBillingPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, stub *stubBillingPackageService) {
				assert.False(t, stub.updateCalled, "validation must short-circuit before the service")
				assert.Equal(t, cn.ErrMissingFieldsInRequest.Error(), body["code"])
			},
		},
		{
			name:           "empty update payload returns nothing-to-update 400",
			payload:        &model.BillingPackageUpdate{},
			stub:           &stubBillingPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, stub *stubBillingPackageService) {
				assert.False(t, stub.updateCalled)
				assert.Equal(t, cn.ErrNothingToUpdate.Error(), body["code"])
			},
		},
		{
			name:           "not found from service maps to 404",
			payload:        &model.BillingPackageUpdate{Label: ptr("X")},
			stub:           &stubBillingPackageService{updateErr: pkg.ValidateBusinessError(cn.ErrBillingPackageNotFound, cn.EntityBillingPackage, id.String())},
			expectedStatus: fiber.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &BillingPackageHandler{Service: tt.stub}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/billing-packages/:id", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)
				c.Locals("id", id)

				return handler.UpdateBillingPackage(tt.payload, c)
			})

			req := httptest.NewRequest(fiber.MethodPatch, "/v1/organizations/"+orgUUID.String()+"/billing-packages/"+id.String(), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validate != nil {
				tt.validate(t, decodeJSON(t, resp.Body), tt.stub)
			}
		})
	}
}

func TestBillingPackageHandler_DeleteBillingPackage(t *testing.T) {
	orgUUID := uuid.New()
	id := uuid.New()

	tests := []struct {
		name           string
		stub           *stubBillingPackageService
		expectedStatus int
		validate       func(t *testing.T, stub *stubBillingPackageService, body []byte)
	}{
		{
			name:           "success returns 204 with empty body and forwards id+org",
			stub:           &stubBillingPackageService{},
			expectedStatus: fiber.StatusNoContent,
			validate: func(t *testing.T, stub *stubBillingPackageService, body []byte) {
				require.True(t, stub.deleteCalled)
				assert.Equal(t, id, stub.gotDeleteID)
				assert.Equal(t, orgUUID, stub.gotDeleteOrg)
				assert.Empty(t, body, "204 must carry no body")
			},
		},
		{
			name:           "not found maps to 404",
			stub:           &stubBillingPackageService{deleteErr: pkg.ValidateBusinessError(cn.ErrBillingPackageNotFound, cn.EntityBillingPackage, id.String())},
			expectedStatus: fiber.StatusNotFound,
			validate: func(t *testing.T, _ *stubBillingPackageService, body []byte) {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(body, &errResp))
				assert.Equal(t, cn.ErrBillingPackageNotFound.Error(), errResp["code"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &BillingPackageHandler{Service: tt.stub}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/billing-packages/:id", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)
				c.Locals("id", id)

				return handler.DeleteBillingPackage(c)
			})

			req := httptest.NewRequest(fiber.MethodDelete, "/v1/organizations/"+orgUUID.String()+"/billing-packages/"+id.String(), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.validate != nil {
				tt.validate(t, tt.stub, body)
			}
		})
	}
}

// ============================================================================
// BillingCalculateHandler — stub + tests
// ============================================================================

type stubBillingCalculateService struct {
	result *model.BillingCalculateResponse
	err    error

	got    model.BillingCalculateRequest
	called bool
}

func (s *stubBillingCalculateService) Calculate(_ context.Context, request model.BillingCalculateRequest) (*model.BillingCalculateResponse, error) {
	s.called = true
	s.got = request

	return s.result, s.err
}

func TestBillingCalculateHandler_CalculateBilling(t *testing.T) {
	orgUUID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name           string
		payload        *model.BillingCalculateRequest
		stub           *stubBillingCalculateService
		expectedStatus int
		validate       func(t *testing.T, body map[string]any, stub *stubBillingCalculateService)
	}{
		{
			name:    "success returns 200 and stamps org id onto request",
			payload: &model.BillingCalculateRequest{LedgerID: ledgerID.String(), Period: "2026-01", Type: "volume"},
			stub: &stubBillingCalculateService{
				result: &model.BillingCalculateResponse{},
			},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, body map[string]any, stub *stubBillingCalculateService) {
				require.True(t, stub.called)
				assert.Equal(t, orgUUID.String(), stub.got.OrganizationID,
					"handler must stamp the path org onto the request")
				assert.Equal(t, ledgerID.String(), stub.got.LedgerID)
				assert.Equal(t, "2026-01", stub.got.Period)
				assert.Contains(t, body, "results")
			},
		},
		{
			name:           "weekly period is accepted",
			payload:        &model.BillingCalculateRequest{LedgerID: ledgerID.String(), Period: "2026-W13"},
			stub:           &stubBillingCalculateService{result: &model.BillingCalculateResponse{}},
			expectedStatus: fiber.StatusOK,
		},
		{
			name:           "daily period is accepted",
			payload:        &model.BillingCalculateRequest{LedgerID: ledgerID.String(), Period: "2026-01-15"},
			stub:           &stubBillingCalculateService{result: &model.BillingCalculateResponse{}},
			expectedStatus: fiber.StatusOK,
		},
		{
			name:           "missing ledgerId fails validation with 400 before service call",
			payload:        &model.BillingCalculateRequest{Period: "2026-01"},
			stub:           &stubBillingCalculateService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, stub *stubBillingCalculateService) {
				assert.False(t, stub.called)
				assert.Equal(t, cn.ErrInvalidLedgerID.Error(), body["code"])
			},
		},
		{
			name:           "non-uuid ledgerId fails validation with 400",
			payload:        &model.BillingCalculateRequest{LedgerID: "not-a-uuid", Period: "2026-01"},
			stub:           &stubBillingCalculateService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, _ *stubBillingCalculateService) {
				assert.Equal(t, cn.ErrInvalidLedgerID.Error(), body["code"])
			},
		},
		{
			name:           "missing period fails validation with 400",
			payload:        &model.BillingCalculateRequest{LedgerID: ledgerID.String()},
			stub:           &stubBillingCalculateService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, stub *stubBillingCalculateService) {
				assert.False(t, stub.called)
				assert.Equal(t, cn.ErrInvalidBillingPeriod.Error(), body["code"])
			},
		},
		{
			name:           "malformed period fails validation with 400",
			payload:        &model.BillingCalculateRequest{LedgerID: ledgerID.String(), Period: "January"},
			stub:           &stubBillingCalculateService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, _ *stubBillingCalculateService) {
				assert.Equal(t, cn.ErrInvalidBillingPeriod.Error(), body["code"])
			},
		},
		{
			name:           "invalid type fails validation with 400",
			payload:        &model.BillingCalculateRequest{LedgerID: ledgerID.String(), Period: "2026-01", Type: "bogus"},
			stub:           &stubBillingCalculateService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, stub *stubBillingCalculateService) {
				assert.False(t, stub.called)
				assert.Equal(t, cn.ErrInvalidBillingPackageType.Error(), body["code"])
			},
		},
		{
			name:           "service failure maps to 500",
			payload:        &model.BillingCalculateRequest{LedgerID: ledgerID.String(), Period: "2026-01"},
			stub:           &stubBillingCalculateService{err: pkg.ValidateBusinessError(cn.ErrBillingCalculationFailed, cn.EntityBillingPackage, "boom")},
			expectedStatus: fiber.StatusInternalServerError,
		},
		{
			name:           "nil result with nil error returns 500",
			payload:        &model.BillingCalculateRequest{LedgerID: ledgerID.String(), Period: "2026-01"},
			stub:           &stubBillingCalculateService{result: nil, err: nil},
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &BillingCalculateHandler{Service: tt.stub}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/billing/calculate", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)

				return handler.CalculateBilling(tt.payload, c)
			})

			req := httptest.NewRequest(fiber.MethodPost, "/v1/organizations/"+orgUUID.String()+"/billing/calculate", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validate != nil {
				tt.validate(t, decodeJSON(t, resp.Body), tt.stub)
			}
		})
	}
}

// ============================================================================
// FeeHandler — stub + tests
// ============================================================================

type stubFeeService struct {
	result *model.FeeEstimateResult
	err    error

	gotEstimate *model.FeeEstimate
	gotOrg      uuid.UUID
	called      bool
}

func (s *stubFeeService) EstimateFeeCalculation(_ context.Context, cf *model.FeeEstimate, organizationID uuid.UUID) (*model.FeeEstimateResult, error) {
	s.called = true
	s.gotEstimate = cf
	s.gotOrg = organizationID

	return s.result, s.err
}

func TestFeeHandler_EstimateFeeCalculation(t *testing.T) {
	orgUUID := uuid.New()
	packageID := uuid.New()
	ledgerID := uuid.New()

	resultWithFees := &model.FeeEstimateResult{
		LedgerID: ledgerID,
		Transaction: model.FeeAdjustedTransaction{
			Metadata: map[string]any{"packageAppliedID": packageID.String()},
		},
	}
	resultNoFees := &model.FeeEstimateResult{
		LedgerID:    ledgerID,
		Transaction: model.FeeAdjustedTransaction{Metadata: map[string]any{}},
	}

	tests := []struct {
		name           string
		payload        *model.FeeEstimate
		stub           *stubFeeService
		expectedStatus int
		validate       func(t *testing.T, body map[string]any, stub *stubFeeService)
	}{
		{
			name:           "success with applied fees returns 200 and forwards org+payload",
			payload:        &model.FeeEstimate{PackageID: packageID, LedgerID: ledgerID, Transaction: transaction.Transaction{}},
			stub:           &stubFeeService{result: resultWithFees},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, body map[string]any, stub *stubFeeService) {
				require.True(t, stub.called)
				assert.Equal(t, orgUUID, stub.gotOrg)
				assert.Equal(t, packageID, stub.gotEstimate.PackageID)
				assert.Equal(t, "Successfully estimated fee.", body["message"])
				assert.NotNil(t, body["feesApplied"], "applied fees must be returned in the envelope")
			},
		},
		{
			name:           "no matching rules returns 200 with no-fees message and nil feesApplied",
			payload:        &model.FeeEstimate{PackageID: packageID, LedgerID: ledgerID},
			stub:           &stubFeeService{result: resultNoFees},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, body map[string]any, _ *stubFeeService) {
				assert.Equal(t, "No fee or gratuity rules were found for the given parameters.", body["message"])
				assert.Nil(t, body["feesApplied"])
			},
		},
		{
			name:           "business error maps to 422",
			payload:        &model.FeeEstimate{PackageID: packageID, LedgerID: ledgerID},
			stub:           &stubFeeService{err: pkg.ValidateBusinessError(cn.ErrMinAmountGreaterThanMaxAmount, cn.EntityPackage)},
			expectedStatus: fiber.StatusUnprocessableEntity,
			validate: func(t *testing.T, body map[string]any, _ *stubFeeService) {
				assert.Equal(t, cn.ErrMinAmountGreaterThanMaxAmount.Error(), body["code"])
			},
		},
		{
			name:           "calculate-fee technical error maps to 500",
			payload:        &model.FeeEstimate{PackageID: packageID, LedgerID: ledgerID},
			stub:           &stubFeeService{err: pkg.ValidateBusinessError(cn.ErrCalculateFee, cn.EntityFeeCalculation)},
			expectedStatus: fiber.StatusInternalServerError,
		},
		{
			name:           "nil result with nil error returns 500",
			payload:        &model.FeeEstimate{PackageID: packageID, LedgerID: ledgerID},
			stub:           &stubFeeService{result: nil, err: nil},
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &FeeHandler{Service: tt.stub}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/estimates", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)

				return handler.EstimateFeeCalculation(tt.payload, c)
			})

			req := httptest.NewRequest(fiber.MethodPost, "/v1/organizations/"+orgUUID.String()+"/estimates", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validate != nil {
				tt.validate(t, decodeJSON(t, resp.Body), tt.stub)
			}
		})
	}
}

// ============================================================================
// PackageHandler — stub + tests
// ============================================================================

type stubPackageService struct {
	createResult *pack.Package
	createErr    error

	getAllResult []*pack.Package
	getAllErr    error

	getByIDResult *pack.Package
	getByIDErr    error

	updateErr error

	deleteErr error

	gotCreate       *model.CreatePackageInput
	gotCreateLedger uuid.UUID
	gotCreateSeg    uuid.UUID
	gotGetAllOrg    uuid.UUID
	gotGetAllFilter feehttp.QueryHeader
	gotGetByIDID    uuid.UUID
	gotUpdate       *model.UpdatePackageInput
	gotDeleteID     uuid.UUID
	createCalled    bool
	updateCalled    bool
	deleteCalled    bool
}

func (s *stubPackageService) CreatePackage(_ context.Context, cpi *model.CreatePackageInput, organizationID, ledgerID, segmentID uuid.UUID) (*pack.Package, error) {
	s.createCalled = true
	s.gotCreate = cpi
	s.gotCreateLedger = ledgerID
	s.gotCreateSeg = segmentID

	return s.createResult, s.createErr
}

func (s *stubPackageService) GetAllPackages(_ context.Context, filters feehttp.QueryHeader, organizationID uuid.UUID) ([]*pack.Package, error) {
	s.gotGetAllOrg = organizationID
	s.gotGetAllFilter = filters

	return s.getAllResult, s.getAllErr
}

func (s *stubPackageService) GetPackageByID(_ context.Context, id, organizationID uuid.UUID) (*pack.Package, error) {
	s.gotGetByIDID = id

	return s.getByIDResult, s.getByIDErr
}

func (s *stubPackageService) UpdatePackageByID(_ context.Context, id, organizationID uuid.UUID, up *model.UpdatePackageInput) error {
	s.updateCalled = true
	s.gotUpdate = up

	return s.updateErr
}

func (s *stubPackageService) DeletePackageByID(_ context.Context, id, organizationID uuid.UUID) error {
	s.deleteCalled = true
	s.gotDeleteID = id

	return s.deleteErr
}

// validCreatePackageInput builds a minimal CreatePackageInput that passes all
// in-handler validation (min<=max, no duplicate priorities, valid fee map).
func validCreatePackageInput(ledgerID uuid.UUID) *model.CreatePackageInput {
	return &model.CreatePackageInput{
		FeeGroupLabel: "Standard",
		LedgerID:      ledgerID.String(),
		MinAmount:     "100.00",
		MaxAmount:     "1000.00",
		Enable:        ptr(true),
		Fee:           map[string]model.Fee{},
	}
}

func TestPackageHandler_CreatePackage(t *testing.T) {
	orgUUID := uuid.New()
	ledgerID := uuid.New()
	segmentID := uuid.New()

	tests := []struct {
		name           string
		payload        *model.CreatePackageInput
		stub           *stubPackageService
		expectedStatus int
		validate       func(t *testing.T, body map[string]any, stub *stubPackageService)
	}{
		{
			name: "success returns 201 and parses ledger+segment ids",
			payload: func() *model.CreatePackageInput {
				p := validCreatePackageInput(ledgerID)
				p.SegmentID = ptr(segmentID.String())

				return p
			}(),
			stub: &stubPackageService{
				createResult: &pack.Package{ID: uuid.New(), FeeGroupLabel: "Standard", LedgerID: ledgerID},
			},
			expectedStatus: fiber.StatusCreated,
			validate: func(t *testing.T, body map[string]any, stub *stubPackageService) {
				require.True(t, stub.createCalled)
				assert.Equal(t, ledgerID, stub.gotCreateLedger, "ledgerId must be parsed and forwarded")
				assert.Equal(t, segmentID, stub.gotCreateSeg, "segmentId must be parsed and forwarded")
				assert.Equal(t, "Standard", body["feeGroupLabel"])
			},
		},
		{
			name:    "success without segment id forwards uuid.Nil segment",
			payload: validCreatePackageInput(ledgerID),
			stub: &stubPackageService{
				createResult: &pack.Package{ID: uuid.New(), LedgerID: ledgerID},
			},
			expectedStatus: fiber.StatusCreated,
			validate: func(t *testing.T, _ map[string]any, stub *stubPackageService) {
				assert.Equal(t, uuid.Nil, stub.gotCreateSeg)
			},
		},
		{
			name: "malformed segment id rejected with 400 before service call",
			payload: func() *model.CreatePackageInput {
				p := validCreatePackageInput(ledgerID)
				p.SegmentID = ptr("not-a-uuid")

				return p
			}(),
			stub:           &stubPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, stub *stubPackageService) {
				assert.False(t, stub.createCalled)
				assert.Equal(t, cn.ErrInvalidSegmentID.Error(), body["code"])
			},
		},
		{
			name: "malformed ledger id rejected with 400",
			payload: func() *model.CreatePackageInput {
				p := validCreatePackageInput(ledgerID)
				p.LedgerID = "not-a-uuid"

				return p
			}(),
			stub:           &stubPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, _ *stubPackageService) {
				assert.Equal(t, cn.ErrInvalidLedgerID.Error(), body["code"])
			},
		},
		{
			name: "min greater than max rejected with 422",
			payload: func() *model.CreatePackageInput {
				p := validCreatePackageInput(ledgerID)
				p.MinAmount = "1000.00"
				p.MaxAmount = "1.00"

				return p
			}(),
			stub:           &stubPackageService{},
			expectedStatus: fiber.StatusUnprocessableEntity,
			validate: func(t *testing.T, body map[string]any, stub *stubPackageService) {
				assert.False(t, stub.createCalled)
				assert.Equal(t, cn.ErrMinAmountGreaterThanMaxAmount.Error(), body["code"])
			},
		},
		{
			name: "duplicate fee priorities rejected with 400",
			payload: func() *model.CreatePackageInput {
				// Both fees carry a valid CalculationModel so ValidateFees passes,
				// letting the handler reach its duplicate-priority guard. Priority 2
				// (not 1) avoids the priority-one referenceAmount rule.
				validFee := func() model.Fee {
					return model.Fee{
						Priority:        2,
						ReferenceAmount: model.OriginalAmount,
						CalculationModel: &model.CalculationModel{
							ApplicationRule: model.FlatFee,
							Calculations:    []model.Calculation{{Type: model.Flat, Value: "10.00"}},
						},
					}
				}
				p := validCreatePackageInput(ledgerID)
				p.Fee = map[string]model.Fee{"a": validFee(), "b": validFee()}

				return p
			}(),
			stub:           &stubPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, stub *stubPackageService) {
				assert.False(t, stub.createCalled)
				assert.Equal(t, cn.ErrPriorityInvalid.Error(), body["code"])
			},
		},
		{
			name:           "conflict from service maps to 409",
			payload:        validCreatePackageInput(ledgerID),
			stub:           &stubPackageService{createErr: pkg.ValidateBusinessError(cn.ErrDuplicatePackage, cn.EntityPackage)},
			expectedStatus: fiber.StatusConflict,
			validate: func(t *testing.T, body map[string]any, _ *stubPackageService) {
				assert.Equal(t, cn.ErrDuplicatePackage.Error(), body["code"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &PackageHandler{Service: tt.stub}

			app := fiber.New()
			app.Post("/v1/organizations/:organization_id/packages", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)

				return handler.CreatePackage(tt.payload, c)
			})

			req := httptest.NewRequest(fiber.MethodPost, "/v1/organizations/"+orgUUID.String()+"/packages", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validate != nil {
				tt.validate(t, decodeJSON(t, resp.Body), tt.stub)
			}
		})
	}
}

func TestPackageHandler_GetAllPackages(t *testing.T) {
	orgUUID := uuid.New()
	ledgerID := uuid.New()

	tests := []struct {
		name           string
		query          string
		stub           *stubPackageService
		expectedStatus int
		validate       func(t *testing.T, body map[string]any, stub *stubPackageService)
	}{
		{
			name:  "success returns 200 with paginated items and parsed filters",
			query: "?ledgerId=" + ledgerID.String() + "&limit=20&page=3",
			stub: &stubPackageService{
				getAllResult: []*pack.Package{{ID: uuid.New(), LedgerID: ledgerID}},
			},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, body map[string]any, stub *stubPackageService) {
				assert.Equal(t, orgUUID, stub.gotGetAllOrg)
				assert.Equal(t, ledgerID, stub.gotGetAllFilter.LedgerID)
				assert.Equal(t, 20, stub.gotGetAllFilter.Limit)
				assert.Equal(t, 3, stub.gotGetAllFilter.Page)

				items, ok := body["items"].([]any)
				require.True(t, ok)
				assert.Len(t, items, 1)
			},
		},
		{
			name:           "malformed query param rejected with 400 before service call",
			query:          "?limit=abc",
			stub:           &stubPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, stub *stubPackageService) {
				assert.Equal(t, uuid.Nil, stub.gotGetAllOrg, "service must not be called when query params are invalid")
				assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), body["code"])
			},
		},
		{
			name:           "malformed segmentId query rejected with 400",
			query:          "?segmentId=not-a-uuid",
			stub:           &stubPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, _ *stubPackageService) {
				assert.Equal(t, cn.ErrInvalidQueryParameter.Error(), body["code"])
			},
		},
		{
			name:           "service error maps to 500",
			query:          "",
			stub:           &stubPackageService{getAllErr: pkg.ValidateBusinessError(cn.ErrCalculateFee, cn.EntityFeeCalculation)},
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &PackageHandler{Service: tt.stub}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/packages", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)

				return handler.GetAllPackages(c)
			})

			req := httptest.NewRequest(fiber.MethodGet, "/v1/organizations/"+orgUUID.String()+"/packages"+tt.query, nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validate != nil {
				tt.validate(t, decodeJSON(t, resp.Body), tt.stub)
			}
		})
	}
}

func TestPackageHandler_GetPackageByID(t *testing.T) {
	orgUUID := uuid.New()
	id := uuid.New()

	tests := []struct {
		name           string
		stub           *stubPackageService
		expectedStatus int
		validate       func(t *testing.T, body map[string]any, stub *stubPackageService)
	}{
		{
			name: "success returns 200 and forwards id",
			stub: &stubPackageService{
				getByIDResult: &pack.Package{ID: id, FeeGroupLabel: "Found"},
			},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, body map[string]any, stub *stubPackageService) {
				assert.Equal(t, id, stub.gotGetByIDID)
				assert.Equal(t, "Found", body["feeGroupLabel"])
			},
		},
		{
			name:           "not found maps to 404",
			stub:           &stubPackageService{getByIDErr: pkg.ValidateBusinessError(cn.ErrEntityNotFound, cn.EntityPackage)},
			expectedStatus: fiber.StatusNotFound,
			validate: func(t *testing.T, body map[string]any, _ *stubPackageService) {
				assert.Equal(t, cn.ErrEntityNotFound.Error(), body["code"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &PackageHandler{Service: tt.stub}

			app := fiber.New()
			app.Get("/v1/organizations/:organization_id/packages/:id", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)
				c.Locals("id", id)

				return handler.GetPackageByID(c)
			})

			req := httptest.NewRequest(fiber.MethodGet, "/v1/organizations/"+orgUUID.String()+"/packages/"+id.String(), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validate != nil {
				tt.validate(t, decodeJSON(t, resp.Body), tt.stub)
			}
		})
	}
}

func TestPackageHandler_UpdatePackageByID(t *testing.T) {
	orgUUID := uuid.New()
	id := uuid.New()

	tests := []struct {
		name           string
		payload        *model.UpdatePackageInput
		stub           *stubPackageService
		expectedStatus int
		validate       func(t *testing.T, body map[string]any, stub *stubPackageService)
	}{
		{
			name:    "success updates then re-reads, returns 200",
			payload: &model.UpdatePackageInput{FeeGroupLabel: "Renamed"},
			stub: &stubPackageService{
				getByIDResult: &pack.Package{ID: id, FeeGroupLabel: "Renamed"},
			},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, body map[string]any, stub *stubPackageService) {
				require.True(t, stub.updateCalled, "update must be called")
				assert.Equal(t, "Renamed", body["feeGroupLabel"],
					"response must reflect the re-read package after update")
			},
		},
		{
			name: "valid fee map passes validation and updates",
			payload: &model.UpdatePackageInput{
				Fee: map[string]model.Fee{"a": {FeeLabel: "Admin", Priority: 2, ReferenceAmount: model.OriginalAmount}},
			},
			stub: &stubPackageService{
				getByIDResult: &pack.Package{ID: id, FeeGroupLabel: "After"},
			},
			expectedStatus: fiber.StatusOK,
			validate: func(t *testing.T, _ map[string]any, stub *stubPackageService) {
				assert.True(t, stub.updateCalled, "a valid fee map must pass the fee guards and reach the service")
			},
		},
		{
			name: "duplicate fee priorities in update rejected with 400",
			payload: &model.UpdatePackageInput{
				Fee: map[string]model.Fee{
					"a": {FeeLabel: "F1", Priority: 3, ReferenceAmount: model.OriginalAmount},
					"b": {FeeLabel: "F2", Priority: 3, ReferenceAmount: model.OriginalAmount},
				},
			},
			stub:           &stubPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, body map[string]any, stub *stubPackageService) {
				assert.False(t, stub.updateCalled, "duplicate priorities must short-circuit before the service")
				assert.Equal(t, cn.ErrPriorityInvalid.Error(), body["code"])
			},
		},
		{
			name: "priority-one fee with wrong reference amount rejected with 400",
			payload: &model.UpdatePackageInput{
				Fee: map[string]model.Fee{"a": {FeeLabel: "F1", Priority: 1, ReferenceAmount: "afterFeesAmount"}},
			},
			stub:           &stubPackageService{},
			expectedStatus: fiber.StatusBadRequest,
			validate: func(t *testing.T, _ map[string]any, stub *stubPackageService) {
				assert.False(t, stub.updateCalled, "fee validation must short-circuit before the service")
			},
		},
		{
			name:           "min greater than max rejected with 422 before service call",
			payload:        &model.UpdatePackageInput{MinAmount: ptr("100"), MaxAmount: ptr("1")},
			stub:           &stubPackageService{},
			expectedStatus: fiber.StatusUnprocessableEntity,
			validate: func(t *testing.T, body map[string]any, stub *stubPackageService) {
				assert.False(t, stub.updateCalled)
				assert.Equal(t, cn.ErrMinAmountGreaterThanMaxAmount.Error(), body["code"])
			},
		},
		{
			name:           "update error maps to 404",
			payload:        &model.UpdatePackageInput{FeeGroupLabel: "X"},
			stub:           &stubPackageService{updateErr: pkg.ValidateBusinessError(cn.ErrEntityNotFound, cn.EntityPackage)},
			expectedStatus: fiber.StatusNotFound,
			validate: func(t *testing.T, _ map[string]any, stub *stubPackageService) {
				assert.True(t, stub.updateCalled)
			},
		},
		{
			name:           "re-read error after successful update maps to 500",
			payload:        &model.UpdatePackageInput{FeeGroupLabel: "X"},
			stub:           &stubPackageService{getByIDErr: pkg.ValidateBusinessError(cn.ErrCalculateFee, cn.EntityFeeCalculation)},
			expectedStatus: fiber.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &PackageHandler{Service: tt.stub}

			app := fiber.New()
			app.Patch("/v1/organizations/:organization_id/packages/:id", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)
				c.Locals("id", id)

				return handler.UpdatePackageByID(tt.payload, c)
			})

			req := httptest.NewRequest(fiber.MethodPatch, "/v1/organizations/"+orgUUID.String()+"/packages/"+id.String(), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			if tt.validate != nil {
				tt.validate(t, decodeJSON(t, resp.Body), tt.stub)
			}
		})
	}
}

func TestPackageHandler_DeletePackageByID(t *testing.T) {
	orgUUID := uuid.New()
	id := uuid.New()

	tests := []struct {
		name           string
		stub           *stubPackageService
		expectedStatus int
		validate       func(t *testing.T, stub *stubPackageService, body []byte)
	}{
		{
			name:           "success returns 204 and forwards id",
			stub:           &stubPackageService{},
			expectedStatus: fiber.StatusNoContent,
			validate: func(t *testing.T, stub *stubPackageService, body []byte) {
				require.True(t, stub.deleteCalled)
				assert.Equal(t, id, stub.gotDeleteID)
				assert.Empty(t, body)
			},
		},
		{
			name:           "not found maps to 404",
			stub:           &stubPackageService{deleteErr: pkg.ValidateBusinessError(cn.ErrEntityNotFound, cn.EntityPackage)},
			expectedStatus: fiber.StatusNotFound,
			validate: func(t *testing.T, _ *stubPackageService, body []byte) {
				var errResp map[string]any
				require.NoError(t, json.Unmarshal(body, &errResp))
				assert.Equal(t, cn.ErrEntityNotFound.Error(), errResp["code"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &PackageHandler{Service: tt.stub}

			app := fiber.New()
			app.Delete("/v1/organizations/:organization_id/packages/:id", func(c *fiber.Ctx) error {
				c.Locals("organization_id", orgUUID)
				c.Locals("id", id)

				return handler.DeletePackageByID(c)
			})

			req := httptest.NewRequest(fiber.MethodDelete, "/v1/organizations/"+orgUUID.String()+"/packages/"+id.String(), nil)
			resp, err := app.Test(req)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedStatus, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			if tt.validate != nil {
				tt.validate(t, tt.stub, body)
			}
		})
	}
}
