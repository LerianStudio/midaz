// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
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
	gotCreateLedger  uuid.UUID
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

func (s *stubBillingPackageService) CreateBillingPackage(_ context.Context, ledgerID uuid.UUID, bp *model.BillingPackage) (*model.BillingPackage, error) {
	s.createCalled = true
	s.gotCreate = bp
	s.gotCreateLedger = ledgerID

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

	got       model.BillingCalculateRequest
	gotLedger uuid.UUID
	called    bool
}

func (s *stubBillingCalculateService) Calculate(_ context.Context, ledgerID uuid.UUID, request model.BillingCalculateRequest) (*model.BillingCalculateResponse, error) {
	s.called = true
	s.got = request
	s.gotLedger = ledgerID

	return s.result, s.err
}

// ============================================================================
// FeeHandler — stub + tests
// ============================================================================

type stubFeeService struct {
	result *model.FeeEstimateResult
	err    error

	gotEstimate *model.FeeEstimate
	gotOrg      uuid.UUID
	gotLedger   uuid.UUID
	called      bool
}

func (s *stubFeeService) EstimateFeeCalculation(_ context.Context, cf *model.FeeEstimate, organizationID, ledgerID uuid.UUID) (*model.FeeEstimateResult, error) {
	s.called = true
	s.gotEstimate = cf
	s.gotOrg = organizationID
	s.gotLedger = ledgerID

	return s.result, s.err
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
