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

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

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
// BillingPackageHandler — stub
// ============================================================================

// stubBillingPackageService is a hand-written stub for BillingPackageUseCase.
// It records the arguments the handler forwards and returns canned values so
// each test asserts both the wire response and the handler→service contract.
// The billing-package tests that drive it live in billing_huma_test.go.
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

	// Ledger argument each by-ID call received, so tests can pin the scope the
	// handler asks for.
	gotGetByIDLedger uuid.UUID
	gotUpdateLedger  uuid.UUID
	gotDeleteLedger  uuid.UUID
}

func (s *stubBillingPackageService) CreateBillingPackage(_ context.Context, bp *model.BillingPackage) (*model.BillingPackage, error) {
	s.createCalled = true
	s.gotCreate = bp

	return s.createResult, s.createErr
}

func (s *stubBillingPackageService) GetBillingPackageByID(_ context.Context, id, organizationID, ledgerID uuid.UUID) (*model.BillingPackage, error) {
	s.gotGetByIDID = id
	s.gotGetByIDOrg = organizationID
	s.gotGetByIDLedger = ledgerID

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

func (s *stubBillingPackageService) UpdateBillingPackage(_ context.Context, id, organizationID, ledgerID uuid.UUID, updates map[string]any) (*model.BillingPackage, error) {
	s.updateCalled = true
	s.gotUpdateID = id
	s.gotUpdateOrg = organizationID
	s.gotUpdateLedger = ledgerID
	s.gotUpdateUpdates = updates

	return s.updateResult, s.updateErr
}

func (s *stubBillingPackageService) DeleteBillingPackage(_ context.Context, id, organizationID, ledgerID uuid.UUID) error {
	s.deleteCalled = true
	s.gotDeleteID = id
	s.gotDeleteOrg = organizationID
	s.gotDeleteLedger = ledgerID

	return s.deleteErr
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
			app.Post("/v1/organizations/:organization_id/billing/calculate", func(c fiber.Ctx) error {
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
			app.Post("/v1/organizations/:organization_id/estimates", func(c fiber.Ctx) error {
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

	// Ledger argument each by-ID call received, so tests can pin the scope the
	// handler asks for.
	gotGetByIDLedger uuid.UUID
	gotUpdateLedger  uuid.UUID
	gotDeleteLedger  uuid.UUID
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

func (s *stubPackageService) GetPackageByID(_ context.Context, id, organizationID, ledgerID uuid.UUID) (*pack.Package, error) {
	s.gotGetByIDID = id
	s.gotGetByIDLedger = ledgerID

	return s.getByIDResult, s.getByIDErr
}

func (s *stubPackageService) UpdatePackageByID(_ context.Context, id, organizationID, ledgerID uuid.UUID, up *model.UpdatePackageInput) error {
	s.updateCalled = true
	s.gotUpdate = up
	s.gotUpdateLedger = ledgerID

	return s.updateErr
}

func (s *stubPackageService) DeletePackageByID(_ context.Context, id, organizationID, ledgerID uuid.UUID) error {
	s.deleteCalled = true
	s.gotDeleteID = id
	s.gotDeleteLedger = ledgerID

	return s.deleteErr
}
