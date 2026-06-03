// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	midaz "github.com/LerianStudio/midaz/v3/components/ledger/internal/services/fees/midaz"
	billing_package "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/billing_package"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// newTestBillingCalculateService creates a BillingCalculateService with mock dependencies for testing.
func newTestBillingCalculateService(
	t *testing.T,
) (*BillingCalculateService, *billing_package.MockRepository, *midaz.MockTransactionCounter, *midaz.MockAccountResolver) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockRepo := billing_package.NewMockRepository(ctrl)
	mockCounter := midaz.NewMockTransactionCounter(ctrl)
	mockResolver := midaz.NewMockAccountResolver(ctrl)

	svc, err := NewBillingCalculateService(mockRepo, mockCounter, mockResolver)
	assert.NoError(t, err)
	assert.NotNil(t, svc)

	return svc, mockRepo, mockCounter, mockResolver
}

// volumePackageForCalc returns a volume billing package with tiered pricing for calculation tests.
func volumePackageForCalc(orgID, ledgerID string) *model.BillingPackage {
	pricingModel := model.PricingModelTiered
	countMode := model.CountModePerRoute
	assetCode := "BRL"
	debitAlias := "billing-debit@account"
	creditAlias := "billing-credit@account"
	routeID := uuid.New()
	freeQuota := 10

	max100 := int64(100)

	return &model.BillingPackage{
		ID:             uuid.New().String(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Label:          "Volume Tiered Package",
		Type:           model.BillingPackageTypeVolume,
		Enable:         boolPtr(true),
		EventFilter: &model.EventFilter{
			TransactionRoute: routeID.String(),
			Status:           "completed",
		},
		PricingModel: &pricingModel,
		Tiers: []model.PricingTier{
			{
				MinQuantity: 0,
				MaxQuantity: &max100,
				UnitPrice:   decimal.NewFromFloat(0.50),
			},
			{
				MinQuantity: 101,
				UnitPrice:   decimal.NewFromFloat(0.30),
			},
		},
		FreeQuota: &freeQuota,
		DiscountTiers: []model.DiscountTier{
			{
				MinQuantity:        50,
				DiscountPercentage: decimal.NewFromInt(10),
			},
		},
		CountMode:          &countMode,
		AssetCode:          &assetCode,
		DebitAccountAlias:  &debitAlias,
		CreditAccountAlias: &creditAlias,
	}
}

// maintenancePackageForCalc returns a maintenance billing package for calculation tests.
func maintenancePackageForCalc(orgID, ledgerID string) *model.BillingPackage {
	feeAmount := decimal.NewFromInt(25)
	assetCode := "BRL"
	creditAccount := "maint-credit@account"
	segmentID := uuid.New()

	return &model.BillingPackage{
		ID:                       uuid.New().String(),
		OrganizationID:           orgID,
		LedgerID:                 ledgerID,
		Label:                    "Maintenance Segment Package",
		Type:                     model.BillingPackageTypeMaintenance,
		Enable:                   boolPtr(true),
		FeeAmount:                &feeAmount,
		AssetCode:                &assetCode,
		MaintenanceCreditAccount: &creditAccount,
		AccountTarget: &model.AccountTarget{
			SegmentID: &segmentID,
		},
	}
}

func TestBillingCalculateService_VolumeHappyPath(t *testing.T) {
	t.Parallel()

	svc, mockRepo, mockCounter, _ := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	volPkg := volumePackageForCalc(orgID, ledgerID)

	tests := []struct {
		name      string
		request   model.BillingCalculateRequest
		mockSetup func()
	}{
		{
			name: "Success - Volume calculation with tiered pricing, free quota, and discount",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeVolume,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeVolume).
					Return([]*model.BillingPackage{volPkg}, nil)

				// 80 total events; 80 - 10 freeQuota = 70 billable; falls in tier [0,100] at 0.50
				// gross = 70 * 0.50 = 35.00; discount 10% at minQty=50 => discount=3.50 => net=31.50
				mockCounter.EXPECT().
					CountByRoute(gomock.Any(), gomock.Any()).
					Return(int64(80), nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Len(t, resp.Results, 1)

			result := resp.Results[0]
			assert.Equal(t, volPkg.ID, result.BillingPackageID)
			assert.Equal(t, volPkg.Label, result.BillingPackageLabel)
			assert.Equal(t, model.BillingPackageTypeVolume, result.BillingType)
			assert.Equal(t, "2026-01", result.Period)

			// 70 billable * 0.50 = 35.00; 10% discount on 35.00 = 3.50; net = 31.50
			expectedNet := decimal.NewFromFloat(31.50)
			assert.True(t, expectedNet.Equal(result.TotalNetAmount),
				"expected net=%s, got=%s", expectedNet.String(), result.TotalNetAmount.String())

			// Transaction payload should exist and be non-empty
			assert.NotEmpty(t, result.TransactionPayload)
			assert.NotEqual(t, "{}", string(result.TransactionPayload))

			// Summary
			assert.Equal(t, 1, resp.Summary.TotalResults)
			assert.Equal(t, 1, resp.Summary.TotalVolume)
			assert.Equal(t, 0, resp.Summary.TotalMaintenance)
			assert.True(t, expectedNet.Equal(resp.Summary.TotalNetAmount))
		})
	}
}

func TestBillingCalculateService_MaintenanceHappyPath(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _, mockResolver := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	maintPkg := maintenancePackageForCalc(orgID, ledgerID)
	orgUUID := uuid.MustParse(orgID)
	ledgerUUID := uuid.MustParse(ledgerID)

	activeStatus := &pkg.AccountStatus{Code: "active", Description: "Active"}

	tests := []struct {
		name      string
		request   model.BillingCalculateRequest
		mockSetup func()
	}{
		{
			name: "Success - Maintenance calculation with 3 accounts",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeMaintenance,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeMaintenance).
					Return([]*model.BillingPackage{maintPkg}, nil)

				mockResolver.EXPECT().
					ResolveAccounts(gomock.Any(), orgUUID, ledgerUUID, *maintPkg.AccountTarget).
					Return([]pkg.Account{
						{ID: uuid.New().String(), Alias: "acc1@wallet", Status: activeStatus},
						{ID: uuid.New().String(), Alias: "acc2@wallet", Status: activeStatus},
						{ID: uuid.New().String(), Alias: "acc3@wallet", Status: activeStatus},
					}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Len(t, resp.Results, 1)

			result := resp.Results[0]
			assert.Equal(t, maintPkg.ID, result.BillingPackageID)
			assert.Equal(t, maintPkg.Label, result.BillingPackageLabel)
			assert.Equal(t, model.BillingPackageTypeMaintenance, result.BillingType)
			assert.Equal(t, 3, result.TotalAccounts)
			assert.Equal(t, 3, result.TotalCharged)

			// 3 accounts * 25 fee = 75
			expectedNet := decimal.NewFromInt(75)
			assert.True(t, expectedNet.Equal(result.TotalNetAmount),
				"expected net=%s, got=%s", expectedNet.String(), result.TotalNetAmount.String())

			// Transaction payload should exist and be non-empty
			assert.NotEmpty(t, result.TransactionPayload)
			assert.NotEqual(t, "{}", string(result.TransactionPayload))

			// Summary
			assert.Equal(t, 1, resp.Summary.TotalResults)
			assert.Equal(t, 0, resp.Summary.TotalVolume)
			assert.Equal(t, 1, resp.Summary.TotalMaintenance)
			assert.True(t, expectedNet.Equal(resp.Summary.TotalNetAmount))
		})
	}
}

func TestBillingCalculateService_MixedBothTypes(t *testing.T) {
	t.Parallel()

	svc, mockRepo, mockCounter, mockResolver := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	volPkg := volumePackageForCalc(orgID, ledgerID)
	maintPkg := maintenancePackageForCalc(orgID, ledgerID)
	orgUUID := uuid.MustParse(orgID)
	ledgerUUID := uuid.MustParse(ledgerID)

	activeStatus := &pkg.AccountStatus{Code: "active", Description: "Active"}

	tests := []struct {
		name      string
		request   model.BillingCalculateRequest
		mockSetup func()
	}{
		{
			name: "Success - Both volume and maintenance packages",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           "", // Both types
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeVolume).
					Return([]*model.BillingPackage{volPkg}, nil)

				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeMaintenance).
					Return([]*model.BillingPackage{maintPkg}, nil)

				mockCounter.EXPECT().
					CountByRoute(gomock.Any(), gomock.Any()).
					Return(int64(80), nil)

				mockResolver.EXPECT().
					ResolveAccounts(gomock.Any(), orgUUID, ledgerUUID, *maintPkg.AccountTarget).
					Return([]pkg.Account{
						{ID: uuid.New().String(), Alias: "acc1@wallet", Status: activeStatus},
					}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Len(t, resp.Results, 2)

			// Summary totals
			assert.Equal(t, 2, resp.Summary.TotalResults)
			assert.Equal(t, 1, resp.Summary.TotalVolume)
			assert.Equal(t, 1, resp.Summary.TotalMaintenance)
		})
	}
}

func TestBillingCalculateService_NoActivePackages(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _, _ := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	tests := []struct {
		name      string
		request   model.BillingCalculateRequest
		mockSetup func()
	}{
		{
			name: "Success - No active packages returns empty results, not error",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeVolume,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeVolume).
					Return([]*model.BillingPackage{}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Empty(t, resp.Results)
			assert.Equal(t, 0, resp.Summary.TotalResults)
			assert.True(t, decimal.Zero.Equal(resp.Summary.TotalNetAmount))
		})
	}
}

func TestBillingCalculateService_TransactionCounterError(t *testing.T) {
	t.Parallel()

	svc, mockRepo, mockCounter, _ := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	volPkg := volumePackageForCalc(orgID, ledgerID)

	tests := []struct {
		name        string
		request     model.BillingCalculateRequest
		mockSetup   func()
		errContains string
	}{
		{
			name: "Error - TransactionCounter fails with package context",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeVolume,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeVolume).
					Return([]*model.BillingPackage{volPkg}, nil)

				mockCounter.EXPECT().
					CountByRoute(gomock.Any(), gomock.Any()).
					Return(int64(0), errors.New("midaz connection refused"))
			},
			errContains: volPkg.ID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestBillingCalculateService_AccountResolverError(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _, mockResolver := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	maintPkg := maintenancePackageForCalc(orgID, ledgerID)
	orgUUID := uuid.MustParse(orgID)
	ledgerUUID := uuid.MustParse(ledgerID)

	tests := []struct {
		name        string
		request     model.BillingCalculateRequest
		mockSetup   func()
		errContains string
	}{
		{
			name: "Error - AccountResolver fails with package context",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeMaintenance,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeMaintenance).
					Return([]*model.BillingPackage{maintPkg}, nil)

				mockResolver.EXPECT().
					ResolveAccounts(gomock.Any(), orgUUID, ledgerUUID, *maintPkg.AccountTarget).
					Return(nil, errors.New("segment not found"))
			},
			errContains: maintPkg.ID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestNewBillingCalculateService_NilDependencies(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	validRepo := billing_package.NewMockRepository(ctrl)
	validCounter := midaz.NewMockTransactionCounter(ctrl)
	validResolver := midaz.NewMockAccountResolver(ctrl)

	tests := []struct {
		name        string
		repo        billing_package.Repository
		counter     midaz.TransactionCounter
		resolver    midaz.AccountResolver
		expectErr   bool
		errContains string
	}{
		{
			name:        "Error - nil repository",
			repo:        nil,
			counter:     validCounter,
			resolver:    validResolver,
			expectErr:   true,
			errContains: "repository is required",
		},
		{
			name:        "Error - nil TransactionCounter",
			repo:        validRepo,
			counter:     nil,
			resolver:    validResolver,
			expectErr:   true,
			errContains: "TransactionCounter is required",
		},
		{
			name:        "Error - nil AccountResolver",
			repo:        validRepo,
			counter:     validCounter,
			resolver:    nil,
			expectErr:   true,
			errContains: "AccountResolver is required",
		},
		{
			name:     "Success - all dependencies provided",
			repo:     validRepo,
			counter:  validCounter,
			resolver: validResolver,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewBillingCalculateService(tt.repo, tt.counter, tt.resolver)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, svc)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, svc)
			}
		})
	}
}

func TestParsePeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		period      string
		wantStart   string
		wantEnd     string
		wantErr     bool
		errContains string
	}{
		{
			name:      "Monthly - valid YYYY-MM",
			period:    "2026-01",
			wantStart: "2026-01-01T00:00:00Z",
			wantEnd:   "2026-02-01T00:00:00Z",
		},
		{
			name:      "Monthly - December rolls to next year",
			period:    "2026-12",
			wantStart: "2026-12-01T00:00:00Z",
			wantEnd:   "2027-01-01T00:00:00Z",
		},
		{
			name:      "Daily - valid YYYY-MM-DD",
			period:    "2026-03-11",
			wantStart: "2026-03-11T00:00:00Z",
			wantEnd:   "2026-03-12T00:00:00Z",
		},
		{
			name:      "Daily - end of month rolls to next month",
			period:    "2026-01-31",
			wantStart: "2026-01-31T00:00:00Z",
			wantEnd:   "2026-02-01T00:00:00Z",
		},
		{
			name:      "Daily - leap year Feb 29",
			period:    "2024-02-29",
			wantStart: "2024-02-29T00:00:00Z",
			wantEnd:   "2024-03-01T00:00:00Z",
		},
		{
			name:      "Weekly - valid YYYY-Www (W01)",
			period:    "2026-W01",
			wantStart: "2025-12-29T00:00:00Z",
			wantEnd:   "2026-01-05T00:00:00Z",
		},
		{
			name:      "Weekly - mid year (W13)",
			period:    "2026-W13",
			wantStart: "2026-03-23T00:00:00Z",
			wantEnd:   "2026-03-30T00:00:00Z",
		},
		{
			name:      "Weekly - end of year (W52)",
			period:    "2026-W52",
			wantStart: "2026-12-21T00:00:00Z",
			wantEnd:   "2026-12-28T00:00:00Z",
		},
		{
			name:      "Weekly - year with 53 weeks (2020-W53)",
			period:    "2020-W53",
			wantStart: "2020-12-28T00:00:00Z",
			wantEnd:   "2021-01-04T00:00:00Z",
		},
		{
			name:        "Error - invalid week number W00",
			period:      "2026-W00",
			wantErr:     true,
			errContains: "FEE-0063",
		},
		{
			name:        "Error - invalid week number W54",
			period:      "2026-W54",
			wantErr:     true,
			errContains: "FEE-0063",
		},
		{
			name:      "Weekly - year with 53 weeks (2026-W53)",
			period:    "2026-W53",
			wantStart: "2026-12-28T00:00:00Z",
			wantEnd:   "2027-01-04T00:00:00Z",
		},
		{
			// 2025-W53 has the correct YYYY-Www format but the week does not exist in 2025
			// (only 52 ISO weeks). parsePeriod uses LooksLikeWeeklyPeriod to detect this
			// case and emits a specific internal message; the public error code is FEE-0063.
			name:        "Error - week 53 on year without 53 weeks (valid format, invalid ISO week)",
			period:      "2025-W53",
			wantErr:     true,
			errContains: "FEE-0063",
		},
		{
			// 2026-W1 fails the strict two-digit enforcement (ISO 8601 requires W01..W53).
			// This is a format error, distinct from the non-existent-week case above.
			// Both map to FEE-0063; the distinction is in the detail logged via telemetry.
			name:        "Error - single digit week (not ISO 8601)",
			period:      "2026-W1",
			wantErr:     true,
			errContains: "FEE-0063",
		},
		{
			name:        "Error - empty string",
			period:      "",
			wantErr:     true,
			errContains: "FEE-0063",
		},
		{
			name:        "Error - invalid format",
			period:      "2026-13",
			wantErr:     true,
			errContains: "FEE-0063",
		},
		{
			name:        "Error - garbage input",
			period:      "not-a-date",
			wantErr:     true,
			errContains: "FEE-0063",
		},
		{
			name:        "Error - invalid day",
			period:      "2026-02-30",
			wantErr:     true,
			errContains: "FEE-0063",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			start, end, err := parsePeriod(tt.period)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)

				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.wantStart, start.Format("2006-01-02T15:04:05Z"))
			assert.Equal(t, tt.wantEnd, end.Format("2006-01-02T15:04:05Z"))
		})
	}
}

func TestFetchPackagesByType_MaintenanceFetchError(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _, _ := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	tests := []struct {
		name        string
		request     model.BillingCalculateRequest
		mockSetup   func()
		errContains string
	}{
		{
			name: "Error - Maintenance type fetch fails",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeMaintenance,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeMaintenance).
					Return(nil, errors.New("db connection lost"))
			},
			errContains: "db connection lost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestFetchPackagesByType_DefaultMaintenanceFetchError(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _, _ := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	tests := []struct {
		name        string
		request     model.BillingCalculateRequest
		mockSetup   func()
		errContains string
	}{
		{
			name: "Error - Default case: volume succeeds but maintenance fetch fails",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           "", // default: fetch both
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeVolume).
					Return([]*model.BillingPackage{}, nil)

				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeMaintenance).
					Return(nil, errors.New("maintenance query timeout"))
			},
			errContains: "maintenance query timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestCalculateVolume_NilEventFilter(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _, _ := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	// Create a volume package with nil EventFilter.
	pricingModel := model.PricingModelTiered
	countMode := model.CountModePerRoute
	assetCode := "BRL"

	nilFilterPkg := &model.BillingPackage{
		ID:             uuid.New().String(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Label:          "Volume No Filter",
		Type:           model.BillingPackageTypeVolume,
		Enable:         boolPtr(true),
		EventFilter:    nil, // missing event filter
		PricingModel:   &pricingModel,
		Tiers: []model.PricingTier{
			{MinQuantity: 0, UnitPrice: decimal.NewFromFloat(0.50)},
		},
		CountMode: &countMode,
		AssetCode: &assetCode,
	}

	tests := []struct {
		name        string
		request     model.BillingCalculateRequest
		mockSetup   func()
		errContains string
	}{
		{
			name: "Error - Volume package with nil EventFilter",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeVolume,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeVolume).
					Return([]*model.BillingPackage{nilFilterPkg}, nil)
			},
			errContains: "missing event filter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestCalculateVolume_FixedPricingHappyPath(t *testing.T) {
	t.Parallel()

	svc, mockRepo, mockCounter, _ := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	pricingModel := model.PricingModelFixed
	countMode := model.CountModePerRoute
	assetCode := "BRL"
	debitAlias := "billing-debit@account"
	creditAlias := "billing-credit@account"
	routeID := uuid.New()

	fixedPkg := &model.BillingPackage{
		ID:             uuid.New().String(),
		OrganizationID: orgID,
		LedgerID:       ledgerID,
		Label:          "Volume Fixed Package",
		Type:           model.BillingPackageTypeVolume,
		Enable:         boolPtr(true),
		EventFilter: &model.EventFilter{
			TransactionRoute: routeID.String(),
			Status:           "completed",
		},
		PricingModel: &pricingModel,
		Tiers: []model.PricingTier{
			{MinQuantity: 0, UnitPrice: decimal.NewFromFloat(2.00)},
		},
		CountMode:          &countMode,
		AssetCode:          &assetCode,
		DebitAccountAlias:  &debitAlias,
		CreditAccountAlias: &creditAlias,
	}

	tests := []struct {
		name      string
		request   model.BillingCalculateRequest
		mockSetup func()
		wantNet   decimal.Decimal
	}{
		{
			name: "Success - Fixed pricing: 50 events * 2.00 = 100.00",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeVolume,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeVolume).
					Return([]*model.BillingPackage{fixedPkg}, nil)

				mockCounter.EXPECT().
					CountByRoute(gomock.Any(), gomock.Any()).
					Return(int64(50), nil)
			},
			wantNet: decimal.NewFromFloat(100.00),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Len(t, resp.Results, 1)

			result := resp.Results[0]
			assert.Equal(t, fixedPkg.ID, result.BillingPackageID)
			assert.Equal(t, model.BillingPackageTypeVolume, result.BillingType)
			assert.True(t, tt.wantNet.Equal(result.TotalNetAmount),
				"expected net=%s, got=%s", tt.wantNet.String(), result.TotalNetAmount.String())
			assert.NotEmpty(t, result.TransactionPayload)
		})
	}
}

func TestCalculateVolume_TransactionCountError(t *testing.T) {
	t.Parallel()

	svc, mockRepo, mockCounter, _ := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	volPkg := volumePackageForCalc(orgID, ledgerID)

	tests := []struct {
		name        string
		request     model.BillingCalculateRequest
		mockSetup   func()
		errContains string
	}{
		{
			name: "Error - CountByRoute returns error",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeVolume,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeVolume).
					Return([]*model.BillingPackage{volPkg}, nil)

				mockCounter.EXPECT().
					CountByRoute(gomock.Any(), gomock.Any()).
					Return(int64(0), errors.New("timeout counting transactions"))
			},
			errContains: "failed to count transactions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestCalculateMaintenance_NilAccountTarget(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _, _ := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	feeAmount := decimal.NewFromInt(25)
	assetCode := "BRL"
	creditAccount := "maint-credit@account"

	nilTargetPkg := &model.BillingPackage{
		ID:                       uuid.New().String(),
		OrganizationID:           orgID,
		LedgerID:                 ledgerID,
		Label:                    "Maintenance No Target",
		Type:                     model.BillingPackageTypeMaintenance,
		Enable:                   boolPtr(true),
		FeeAmount:                &feeAmount,
		AssetCode:                &assetCode,
		MaintenanceCreditAccount: &creditAccount,
		AccountTarget:            nil, // missing account target
	}

	tests := []struct {
		name        string
		request     model.BillingCalculateRequest
		mockSetup   func()
		errContains string
	}{
		{
			name: "Error - Maintenance package with nil AccountTarget",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeMaintenance,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeMaintenance).
					Return([]*model.BillingPackage{nilTargetPkg}, nil)
			},
			errContains: "missing account target",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestCalculateMaintenance_NilFeeAmount(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _, mockResolver := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	orgUUID := uuid.MustParse(orgID)
	ledgerUUID := uuid.MustParse(ledgerID)

	assetCode := "BRL"
	creditAccount := "maint-credit@account"
	segmentID := uuid.New()

	nilFeePkg := &model.BillingPackage{
		ID:                       uuid.New().String(),
		OrganizationID:           orgID,
		LedgerID:                 ledgerID,
		Label:                    "Maintenance Nil Fee",
		Type:                     model.BillingPackageTypeMaintenance,
		Enable:                   boolPtr(true),
		FeeAmount:                nil, // nil fee amount
		AssetCode:                &assetCode,
		MaintenanceCreditAccount: &creditAccount,
		AccountTarget: &model.AccountTarget{
			SegmentID: &segmentID,
		},
	}

	activeStatus := &pkg.AccountStatus{Code: "active", Description: "Active"}

	tests := []struct {
		name      string
		request   model.BillingCalculateRequest
		mockSetup func()
	}{
		{
			name: "Success - Nil FeeAmount defaults to zero, net amount is zero",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeMaintenance,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeMaintenance).
					Return([]*model.BillingPackage{nilFeePkg}, nil)

				mockResolver.EXPECT().
					ResolveAccounts(gomock.Any(), orgUUID, ledgerUUID, *nilFeePkg.AccountTarget).
					Return([]pkg.Account{
						{ID: uuid.New().String(), Alias: "acc1@wallet", Status: activeStatus},
						{ID: uuid.New().String(), Alias: "acc2@wallet", Status: activeStatus},
					}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Len(t, resp.Results, 1)

			result := resp.Results[0]
			assert.Equal(t, nilFeePkg.ID, result.BillingPackageID)
			assert.Equal(t, 2, result.TotalAccounts)
			// feeAmount defaults to 0, so 0 * 2 = 0
			assert.True(t, decimal.Zero.Equal(result.TotalNetAmount),
				"expected net=0, got=%s", result.TotalNetAmount.String())
		})
	}
}

func TestCalculateMaintenance_ZeroResolvedAccounts(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _, mockResolver := newTestBillingCalculateService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	maintPkg := maintenancePackageForCalc(orgID, ledgerID)
	orgUUID := uuid.MustParse(orgID)
	ledgerUUID := uuid.MustParse(ledgerID)

	tests := []struct {
		name      string
		request   model.BillingCalculateRequest
		mockSetup func()
	}{
		{
			name: "Success - Zero resolved accounts returns empty payload",
			request: model.BillingCalculateRequest{
				OrganizationID: orgID,
				LedgerID:       ledgerID,
				Period:         "2026-01",
				Type:           model.BillingPackageTypeMaintenance,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					FindActiveByType(gomock.Any(), orgID, ledgerID, model.BillingPackageTypeMaintenance).
					Return([]*model.BillingPackage{maintPkg}, nil)

				mockResolver.EXPECT().
					ResolveAccounts(gomock.Any(), orgUUID, ledgerUUID, *maintPkg.AccountTarget).
					Return([]pkg.Account{}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Len(t, resp.Results, 1)

			result := resp.Results[0]
			assert.Equal(t, maintPkg.ID, result.BillingPackageID)
			assert.Equal(t, 0, result.TotalAccounts)
			assert.Equal(t, 0, result.TotalCharged)
			assert.True(t, decimal.Zero.Equal(result.TotalNetAmount))
			assert.Equal(t, "{}", string(result.TransactionPayload))
		})
	}
}

func TestBillingCalculateService_InvalidPeriod(t *testing.T) {
	t.Parallel()

	svc, _, _, _ := newTestBillingCalculateService(t)

	tests := []struct {
		name        string
		request     model.BillingCalculateRequest
		errContains string
	}{
		{
			name: "Error - Invalid period format",
			request: model.BillingCalculateRequest{
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				Period:         "invalid-period",
			},
			errContains: "FEE-0063",
		},
		{
			name: "Error - Empty period",
			request: model.BillingCalculateRequest{
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				Period:         "",
			},
			errContains: "FEE-0063",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			resp, err := svc.Calculate(ctx, tt.request)

			assert.Error(t, err)
			assert.Nil(t, resp)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}
