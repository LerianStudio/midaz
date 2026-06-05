// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	billing_package "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/billing_package"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/mock/gomock"
)

// newTestBillingPackageService creates a BillingPackageService with mock dependencies for testing.
func newTestBillingPackageService(
	t *testing.T,
) (*BillingPackageService, *billing_package.MockRepository, *pkg.MockMidazResolver) {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(func() { ctrl.Finish() })

	mockRepo := billing_package.NewMockRepository(ctrl)
	mockMidaz := pkg.NewMockMidazResolver(ctrl)

	svc, err := NewBillingPackageService(mockRepo, mockMidaz)
	assert.NoError(t, err)
	assert.NotNil(t, svc)

	return svc, mockRepo, mockMidaz
}

// validVolumeBillingPackage returns a valid volume-type BillingPackage for testing.
func validVolumeBillingPackage() *model.BillingPackage {
	pricingModel := model.PricingModelTiered
	countMode := model.CountModePerRoute
	assetCode := "USD"
	debitAlias := "debit@account"
	creditAlias := "credit@account"
	routeID := uuid.New()

	return &model.BillingPackage{
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		Label:          "Volume Package Test",
		Type:           model.BillingPackageTypeVolume,
		EventFilter: &model.EventFilter{
			TransactionRoute: routeID.String(),
			Status:           "completed",
		},
		PricingModel: &pricingModel,
		Tiers: []model.PricingTier{
			{
				MinQuantity: 0,
				UnitPrice:   decimal.NewFromInt(10),
			},
		},
		CountMode:          &countMode,
		AssetCode:          &assetCode,
		DebitAccountAlias:  &debitAlias,
		CreditAccountAlias: &creditAlias,
	}
}

// validMaintenanceBillingPackage returns a valid maintenance-type BillingPackage for testing.
func validMaintenanceBillingPackage() *model.BillingPackage {
	feeAmount := decimal.NewFromInt(50)
	assetCode := "USD"
	creditAccount := "maint-credit@account"
	segmentID := uuid.New()

	return &model.BillingPackage{
		OrganizationID:           uuid.New().String(),
		LedgerID:                 uuid.New().String(),
		Label:                    "Maintenance Package Test",
		Type:                     model.BillingPackageTypeMaintenance,
		FeeAmount:                &feeAmount,
		AssetCode:                &assetCode,
		MaintenanceCreditAccount: &creditAccount,
		AccountTarget: &model.AccountTarget{
			SegmentID: &segmentID,
		},
	}
}

func TestCreateBillingPackage_Success_Volume(t *testing.T) {
	t.Parallel()

	svc, mockRepo, mockMidaz := newTestBillingPackageService(t)

	bp := validVolumeBillingPackage()

	tests := []struct {
		name      string
		input     *model.BillingPackage
		mockSetup func()
	}{
		{
			name:  "Success - Create volume billing package with default enable (nil → true)",
			input: bp,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindMatchingPackages(gomock.Any(), bp.OrganizationID, bp.LedgerID, bp.EventFilter.TransactionRoute).
					Return([]*model.BillingPackage{}, nil)

				mockMidaz.EXPECT().
					AccountExistsByAlias(gomock.Any(), uuid.MustParse(bp.OrganizationID), uuid.MustParse(bp.LedgerID), *bp.DebitAccountAlias).
					Return(nil)

				mockMidaz.EXPECT().
					AccountExistsByAlias(gomock.Any(), uuid.MustParse(bp.OrganizationID), uuid.MustParse(bp.LedgerID), *bp.CreditAccountAlias).
					Return(nil)

				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, input *model.BillingPackage) (*model.BillingPackage, error) {
						return input, nil
					})
			},
		},
		{
			// Regression: explicit enable=false must survive CreateBillingPackage.
			// Before the fix, the service hardcoded bp.Enable = true, overwriting any value.
			name: "Regression - Create volume billing package with explicit enable=false",
			input: func() *model.BillingPackage {
				disabled := validVolumeBillingPackage()
				disabled.Enable = boolPtr(false)
				bp = disabled // share the same instance with mockSetup below

				return disabled
			}(),
			mockSetup: func() {
				mockRepo.EXPECT().
					FindMatchingPackages(gomock.Any(), bp.OrganizationID, bp.LedgerID, bp.EventFilter.TransactionRoute).
					Return([]*model.BillingPackage{}, nil)

				mockMidaz.EXPECT().
					AccountExistsByAlias(gomock.Any(), uuid.MustParse(bp.OrganizationID), uuid.MustParse(bp.LedgerID), *bp.DebitAccountAlias).
					Return(nil)

				mockMidaz.EXPECT().
					AccountExistsByAlias(gomock.Any(), uuid.MustParse(bp.OrganizationID), uuid.MustParse(bp.LedgerID), *bp.CreditAccountAlias).
					Return(nil)

				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, input *model.BillingPackage) (*model.BillingPackage, error) {
						return input, nil
					})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.CreateBillingPackage(ctx, tt.input)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.NotEmpty(t, result.ID)
			assert.NotNil(t, result.Enable)

			if tt.input.Enable == nil {
				assert.True(t, *result.Enable, "nil enable must default to true")
			} else {
				assert.Equal(t, *tt.input.Enable, *result.Enable, "explicit enable must be preserved")
			}

			assert.NotEmpty(t, result.CreatedAt)
			assert.NotEmpty(t, result.UpdatedAt)
		})
	}
}

func TestCreateBillingPackage_Success_Maintenance(t *testing.T) {
	t.Parallel()

	svc, mockRepo, mockMidaz := newTestBillingPackageService(t)

	bp := validMaintenanceBillingPackage()

	tests := []struct {
		name      string
		input     *model.BillingPackage
		mockSetup func()
	}{
		{
			name:  "Success - Create maintenance billing package with default enable (nil → true)",
			input: bp,
			mockSetup: func() {
				mockMidaz.EXPECT().
					AccountExistsByAlias(gomock.Any(), uuid.MustParse(bp.OrganizationID), uuid.MustParse(bp.LedgerID), *bp.MaintenanceCreditAccount).
					Return(nil)

				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, input *model.BillingPackage) (*model.BillingPackage, error) {
						return input, nil
					})
			},
		},
		{
			// Regression: explicit enable=false must survive CreateBillingPackage.
			name: "Regression - Create maintenance billing package with explicit enable=false",
			input: func() *model.BillingPackage {
				disabled := validMaintenanceBillingPackage()
				disabled.Enable = boolPtr(false)
				bp = disabled // share the same instance with mockSetup below

				return disabled
			}(),
			mockSetup: func() {
				mockMidaz.EXPECT().
					AccountExistsByAlias(gomock.Any(), uuid.MustParse(bp.OrganizationID), uuid.MustParse(bp.LedgerID), *bp.MaintenanceCreditAccount).
					Return(nil)

				mockRepo.EXPECT().
					Create(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, input *model.BillingPackage) (*model.BillingPackage, error) {
						return input, nil
					})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.CreateBillingPackage(ctx, tt.input)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.NotEmpty(t, result.ID)
			assert.NotNil(t, result.Enable)

			if tt.input.Enable == nil {
				assert.True(t, *result.Enable, "nil enable must default to true")
			} else {
				assert.Equal(t, *tt.input.Enable, *result.Enable, "explicit enable must be preserved")
			}

			assert.Equal(t, model.BillingPackageTypeMaintenance, result.Type)
			assert.NotEmpty(t, result.CreatedAt)
			assert.NotEmpty(t, result.UpdatedAt)
		})
	}
}

func TestCreateBillingPackage_ValidationError(t *testing.T) {
	t.Parallel()

	svc, _, _ := newTestBillingPackageService(t)

	tests := []struct {
		name        string
		input       *model.BillingPackage
		errContains string
	}{
		{
			name: "Error - Invalid billing package type",
			input: &model.BillingPackage{
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				Label:          "Bad Type",
				Type:           "invalid-type",
			},
			errContains: "FEE-0053",
		},
		{
			name: "Error - Missing volume fields",
			input: &model.BillingPackage{
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
				Label:          "Missing Fields",
				Type:           model.BillingPackageTypeVolume,
			},
			errContains: "FEE-0054",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			result, err := svc.CreateBillingPackage(ctx, tt.input)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestCreateBillingPackage_RouteOverlap(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	bp := validVolumeBillingPackage()

	tests := []struct {
		name        string
		input       *model.BillingPackage
		mockSetup   func()
		errContains string
	}{
		{
			name:  "Error - Route overlap returns FEE-0058",
			input: bp,
			mockSetup: func() {
				existingPkg := &model.BillingPackage{
					ID:             uuid.New().String(),
					OrganizationID: bp.OrganizationID,
					LedgerID:       bp.LedgerID,
					Type:           model.BillingPackageTypeVolume,
				}

				mockRepo.EXPECT().
					FindMatchingPackages(gomock.Any(), bp.OrganizationID, bp.LedgerID, bp.EventFilter.TransactionRoute).
					Return([]*model.BillingPackage{existingPkg}, nil)
			},
			errContains: "A billing package already exists for this organization, ledger, and transaction route combination.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.CreateBillingPackage(ctx, tt.input)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestCreateBillingPackage_AccountValidationError(t *testing.T) {
	t.Parallel()

	svc, mockRepo, mockMidaz := newTestBillingPackageService(t)

	bp := validVolumeBillingPackage()

	tests := []struct {
		name        string
		input       *model.BillingPackage
		mockSetup   func()
		errContains string
	}{
		{
			name:  "Error - Credit account not found on Midaz",
			input: bp,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindMatchingPackages(gomock.Any(), bp.OrganizationID, bp.LedgerID, bp.EventFilter.TransactionRoute).
					Return([]*model.BillingPackage{}, nil)

				mockMidaz.EXPECT().
					AccountExistsByAlias(gomock.Any(), uuid.MustParse(bp.OrganizationID), uuid.MustParse(bp.LedgerID), *bp.DebitAccountAlias).
					Return(nil)

				mockMidaz.EXPECT().
					AccountExistsByAlias(gomock.Any(), uuid.MustParse(bp.OrganizationID), uuid.MustParse(bp.LedgerID), *bp.CreditAccountAlias).
					Return(errors.New("FEE-0014"))
			},
			errContains: "FEE-0014",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.CreateBillingPackage(ctx, tt.input)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestGetBillingPackageByID_Success(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	orgID := uuid.New().String()
	bpID := uuid.New().String()

	tests := []struct {
		name      string
		id        string
		orgID     string
		mockSetup func()
	}{
		{
			name:  "Success - Get billing package by ID",
			id:    bpID,
			orgID: orgID,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByID(gomock.Any(), bpID, orgID).
					Return(&model.BillingPackage{
						ID:             bpID,
						OrganizationID: orgID,
						Label:          "Test Package",
						Type:           model.BillingPackageTypeVolume,
					}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.GetBillingPackageByID(ctx, tt.id, tt.orgID)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, bpID, result.ID)
		})
	}
}

func TestGetBillingPackageByID_NotFound(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	orgID := uuid.New().String()
	bpID := uuid.New().String()

	tests := []struct {
		name        string
		id          string
		orgID       string
		mockSetup   func()
		errContains string
	}{
		{
			name:  "Error - Billing package not found returns FEE-0052",
			id:    bpID,
			orgID: orgID,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindByID(gomock.Any(), bpID, orgID).
					Return(nil, mongo.ErrNoDocuments)
			},
			errContains: "No billing package was found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.GetBillingPackageByID(ctx, tt.id, tt.orgID)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.errContains)

			var notFoundErr pkg.EntityNotFoundError
			assert.ErrorAs(t, err, &notFoundErr)
			assert.Equal(t, constant.ErrBillingPackageNotFound.Error(), notFoundErr.Code)
		})
	}
}

func TestGetAllBillingPackages_Success(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	tests := []struct {
		name      string
		orgID     string
		ledgerID  string
		limit     int
		page      int
		mockSetup func()
		wantCount int
		wantTotal int64
	}{
		{
			name:     "Success - Get all billing packages",
			orgID:    orgID,
			ledgerID: ledgerID,
			limit:    10,
			page:     1,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, "", 10, 1).
					Return([]*model.BillingPackage{
						{ID: uuid.New().String(), Label: "Package 1"},
						{ID: uuid.New().String(), Label: "Package 2"},
					}, int64(2), nil)
			},
			wantCount: 2,
			wantTotal: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			results, total, err := svc.GetAllBillingPackages(ctx, tt.orgID, tt.ledgerID, "", tt.limit, tt.page)

			assert.NoError(t, err)
			assert.Len(t, results, tt.wantCount)
			assert.Equal(t, tt.wantTotal, total)
		})
	}
}

func TestUpdateBillingPackage_Success(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	orgID := uuid.New().String()
	bpID := uuid.New().String()

	tests := []struct {
		name      string
		id        string
		orgID     string
		updates   map[string]any
		mockSetup func()
	}{
		{
			name:  "Success - Update billing package",
			id:    bpID,
			orgID: orgID,
			updates: map[string]any{
				"label":  "Updated Label",
				"enable": false,
			},
			mockSetup: func() {
				mockRepo.EXPECT().
					Update(gomock.Any(), bpID, orgID, gomock.Any()).
					Return(nil)

				mockRepo.EXPECT().
					FindByID(gomock.Any(), bpID, orgID).
					Return(&model.BillingPackage{
						ID:             bpID,
						OrganizationID: orgID,
						Label:          "Updated Label",
						Enable:         boolPtr(false),
					}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.UpdateBillingPackage(ctx, tt.id, tt.orgID, tt.updates)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, "Updated Label", result.Label)
		})
	}
}

func TestDeleteBillingPackage_Success(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	orgID := uuid.New().String()
	bpID := uuid.New().String()

	tests := []struct {
		name      string
		id        string
		orgID     string
		mockSetup func()
	}{
		{
			name:  "Success - Delete billing package",
			id:    bpID,
			orgID: orgID,
			mockSetup: func() {
				mockRepo.EXPECT().
					SoftDelete(gomock.Any(), bpID, orgID).
					Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			err := svc.DeleteBillingPackage(ctx, tt.id, tt.orgID)

			assert.NoError(t, err)
		})
	}
}

func TestDeleteBillingPackage_NotFound(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	orgID := uuid.New().String()
	bpID := uuid.New().String()

	tests := []struct {
		name        string
		id          string
		orgID       string
		mockSetup   func()
		errContains string
	}{
		{
			name:  "Error - Delete billing package not found returns FEE-0052",
			id:    bpID,
			orgID: orgID,
			mockSetup: func() {
				// The repo layer returns an EntityNotFoundError (FEE-0012) when matched_count is 0.
				// The service should remap this to FEE-0052 (BillingPackageNotFound).
				mockRepo.EXPECT().
					SoftDelete(gomock.Any(), bpID, orgID).
					Return(pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", constant.BillingPackageCollection))
			},
			errContains: "No billing package was found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			err := svc.DeleteBillingPackage(ctx, tt.id, tt.orgID)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)

			var notFoundErr pkg.EntityNotFoundError
			assert.ErrorAs(t, err, &notFoundErr)
			assert.Equal(t, constant.ErrBillingPackageNotFound.Error(), notFoundErr.Code)
		})
	}
}

func TestGetAllBillingPackages_WithTypeFilter(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	tests := []struct {
		name        string
		billingType string
		mockSetup   func()
		wantCount   int
		wantTotal   int64
	}{
		{
			name:        "Success - Filter by volume type",
			billingType: "volume",
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, "volume", 10, 1).
					Return([]*model.BillingPackage{
						{ID: uuid.New().String(), Label: "Volume 1", Type: model.BillingPackageTypeVolume},
					}, int64(1), nil)
			},
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:        "Success - Filter by maintenance type",
			billingType: "maintenance",
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, "maintenance", 10, 1).
					Return([]*model.BillingPackage{
						{ID: uuid.New().String(), Label: "Maintenance 1", Type: model.BillingPackageTypeMaintenance},
						{ID: uuid.New().String(), Label: "Maintenance 2", Type: model.BillingPackageTypeMaintenance},
					}, int64(2), nil)
			},
			wantCount: 2,
			wantTotal: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			results, total, err := svc.GetAllBillingPackages(ctx, orgID, ledgerID, tt.billingType, 10, 1)

			assert.NoError(t, err)
			assert.Len(t, results, tt.wantCount)
			assert.Equal(t, tt.wantTotal, total)
		})
	}
}

func TestGetAllBillingPackages_RepoError(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	tests := []struct {
		name        string
		mockSetup   func()
		errContains string
	}{
		{
			name: "Error - FindAll repo failure",
			mockSetup: func() {
				mockRepo.EXPECT().
					FindAll(gomock.Any(), orgID, ledgerID, "", 10, 1).
					Return(nil, int64(0), errors.New("connection timeout"))
			},
			errContains: "connection timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			results, total, err := svc.GetAllBillingPackages(ctx, orgID, ledgerID, "", 10, 1)

			assert.Error(t, err)
			assert.Nil(t, results)
			assert.Equal(t, int64(0), total)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestUpdateBillingPackage_FindByIDFailureAfterUpdate(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	orgID := uuid.New().String()
	bpID := uuid.New().String()

	tests := []struct {
		name        string
		mockSetup   func()
		errContains string
	}{
		{
			name: "Error - FindByID fails after successful Update",
			mockSetup: func() {
				mockRepo.EXPECT().
					Update(gomock.Any(), bpID, orgID, gomock.Any()).
					Return(nil)

				mockRepo.EXPECT().
					FindByID(gomock.Any(), bpID, orgID).
					Return(nil, errors.New("document not found after update"))
			},
			errContains: "document not found after update",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.UpdateBillingPackage(ctx, bpID, orgID, map[string]any{"label": "new"})

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestUpdateBillingPackage_RepoUpdateError(t *testing.T) {
	t.Parallel()

	svc, mockRepo, _ := newTestBillingPackageService(t)

	orgID := uuid.New().String()
	bpID := uuid.New().String()

	tests := []struct {
		name        string
		mockSetup   func()
		errContains string
	}{
		{
			name: "Error - Update repo failure",
			mockSetup: func() {
				mockRepo.EXPECT().
					Update(gomock.Any(), bpID, orgID, gomock.Any()).
					Return(errors.New("write concern error"))
			},
			errContains: "write concern error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.UpdateBillingPackage(ctx, bpID, orgID, map[string]any{"label": "new"})

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestValidateMaintenanceCreate_CreditAccountFails(t *testing.T) {
	t.Parallel()

	svc, _, mockMidaz := newTestBillingPackageService(t)

	bp := validMaintenanceBillingPackage()

	tests := []struct {
		name        string
		input       *model.BillingPackage
		mockSetup   func()
		errContains string
	}{
		{
			name:  "Error - MaintenanceCreditAccount Midaz validation fails",
			input: bp,
			mockSetup: func() {
				mockMidaz.EXPECT().
					AccountExistsByAlias(gomock.Any(), uuid.MustParse(bp.OrganizationID), uuid.MustParse(bp.LedgerID), *bp.MaintenanceCreditAccount).
					Return(errors.New("FEE-0014"))
			},
			errContains: "FEE-0014",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.CreateBillingPackage(ctx, tt.input)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestValidateVolumeCreate_DebitAccountAliasFails(t *testing.T) {
	t.Parallel()

	svc, mockRepo, mockMidaz := newTestBillingPackageService(t)

	bp := validVolumeBillingPackage()

	tests := []struct {
		name        string
		input       *model.BillingPackage
		mockSetup   func()
		errContains string
	}{
		{
			name:  "Error - DebitAccountAlias Midaz validation fails",
			input: bp,
			mockSetup: func() {
				mockRepo.EXPECT().
					FindMatchingPackages(gomock.Any(), bp.OrganizationID, bp.LedgerID, bp.EventFilter.TransactionRoute).
					Return([]*model.BillingPackage{}, nil)

				mockMidaz.EXPECT().
					AccountExistsByAlias(gomock.Any(), uuid.MustParse(bp.OrganizationID), uuid.MustParse(bp.LedgerID), *bp.DebitAccountAlias).
					Return(errors.New("FEE-0014"))
			},
			errContains: "FEE-0014",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			ctx := context.Background()

			result, err := svc.CreateBillingPackage(ctx, tt.input)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestNewBillingPackageService_NilDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		repo        billing_package.Repository
		midaz       pkg.MidazResolver
		expectErr   bool
		errContains string
	}{
		{
			name:        "Error - nil repository",
			repo:        nil,
			midaz:       pkg.NewMockMidazResolver(gomock.NewController(t)),
			expectErr:   true,
			errContains: "BillingPackage repository is required",
		},
		{
			name:        "Error - nil MidazResolver",
			repo:        billing_package.NewMockRepository(gomock.NewController(t)),
			midaz:       nil,
			expectErr:   true,
			errContains: "MidazResolver is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := NewBillingPackageService(tt.repo, tt.midaz)

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
