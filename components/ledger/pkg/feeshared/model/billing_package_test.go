// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func newValidEventFilter() *EventFilter {
	return &EventFilter{
		TransactionRoute: "00000000-0000-0000-0000-000000000001",
		Status:           "APPROVED",
	}
}

func newValidPricingTiers() []PricingTier {
	maxQty := int64(100)

	return []PricingTier{
		{
			MinQuantity: 0,
			MaxQuantity: &maxQty,
			UnitPrice:   decimal.NewFromFloat(1.50),
		},
		{
			MinQuantity: 101,
			MaxQuantity: nil,
			UnitPrice:   decimal.NewFromFloat(1.00),
		},
	}
}

func newValidVolumeBillingPackage() BillingPackage {
	pricingModel := PricingModelTiered
	countMode := CountModePerRoute
	assetCode := "BRL"
	debitAlias := "debit-account"
	creditAlias := "credit-account"

	return BillingPackage{
		ID:                 "pkg-001",
		OrganizationID:     "org-001",
		LedgerID:           "ledger-001",
		Label:              "Volume Package",
		Type:               BillingPackageTypeVolume,
		Enable:             boolPtr(true),
		EventFilter:        newValidEventFilter(),
		PricingModel:       &pricingModel,
		Tiers:              newValidPricingTiers(),
		AssetCode:          &assetCode,
		DebitAccountAlias:  &debitAlias,
		CreditAccountAlias: &creditAlias,
		CountMode:          &countMode,
		CreatedAt:          "2026-01-01T00:00:00Z",
		UpdatedAt:          "2026-01-01T00:00:00Z",
	}
}

func newValidMaintenanceBillingPackage() BillingPackage {
	feeAmount := decimal.NewFromFloat(50.00)
	creditAccount := "maintenance-credit-account"
	assetCode := "BRL"
	segID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	return BillingPackage{
		ID:                       "pkg-002",
		OrganizationID:           "org-001",
		LedgerID:                 "ledger-001",
		Label:                    "Maintenance Package",
		Type:                     BillingPackageTypeMaintenance,
		Enable:                   boolPtr(true),
		FeeAmount:                &feeAmount,
		AssetCode:                &assetCode,
		MaintenanceCreditAccount: &creditAccount,
		AccountTarget: &AccountTarget{
			SegmentID: &segID,
		},
		CreatedAt: "2026-01-01T00:00:00Z",
		UpdatedAt: "2026-01-01T00:00:00Z",
	}
}

func TestBillingPackage_Validate_ValidVolume(t *testing.T) {
	t.Parallel()

	bp := newValidVolumeBillingPackage()
	err := bp.Validate()

	assert.NoError(t, err)
}

func TestBillingPackage_Validate_ValidMaintenance(t *testing.T) {
	t.Parallel()

	bp := newValidMaintenanceBillingPackage()
	err := bp.Validate()

	assert.NoError(t, err)
}

func TestBillingPackage_Validate_CommonFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		setup       func(*BillingPackage)
		wantErrCode string
	}{
		{
			name: "empty label",
			setup: func(bp *BillingPackage) {
				bp.Label = ""
			},
			wantErrCode: constant.ErrMissingFieldsInRequest.Error(),
		},
		{
			name: "whitespace-only label",
			setup: func(bp *BillingPackage) {
				bp.Label = "   "
			},
			wantErrCode: constant.ErrMissingFieldsInRequest.Error(),
		},
		{
			name: "empty ledgerId",
			setup: func(bp *BillingPackage) {
				bp.LedgerID = ""
			},
			wantErrCode: constant.ErrMissingFieldsInRequest.Error(),
		},
		{
			name: "whitespace-only ledgerId",
			setup: func(bp *BillingPackage) {
				bp.LedgerID = "   "
			},
			wantErrCode: constant.ErrMissingFieldsInRequest.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			tt.setup(&bp)

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_EventFilterContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		eventFilter *EventFilter
		wantErrCode string
	}{
		{
			name:        "empty transactionRoute",
			eventFilter: &EventFilter{TransactionRoute: "", Status: "APPROVED"},
			wantErrCode: constant.ErrMissingFieldsInRequest.Error(),
		},
		{
			name:        "whitespace-only transactionRoute",
			eventFilter: &EventFilter{TransactionRoute: "   ", Status: "APPROVED"},
			wantErrCode: constant.ErrMissingFieldsInRequest.Error(),
		},
		{
			name:        "empty status",
			eventFilter: &EventFilter{TransactionRoute: "00000000-0000-0000-0000-000000000001", Status: ""},
			wantErrCode: constant.ErrMissingFieldsInRequest.Error(),
		},
		{
			name:        "whitespace-only status",
			eventFilter: &EventFilter{TransactionRoute: "00000000-0000-0000-0000-000000000001", Status: "   "},
			wantErrCode: constant.ErrMissingFieldsInRequest.Error(),
		},
		{
			name:        "empty eventFilter (all zero values)",
			eventFilter: &EventFilter{},
			wantErrCode: constant.ErrMissingFieldsInRequest.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			bp.EventFilter = tt.eventFilter

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_InvalidType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		packageType string
		wantErrCode string
	}{
		{
			name:        "empty type",
			packageType: "",
			wantErrCode: constant.ErrInvalidBillingPackageType.Error(),
		},
		{
			name:        "unknown type",
			packageType: "unknown",
			wantErrCode: constant.ErrInvalidBillingPackageType.Error(),
		},
		{
			name:        "mixed case type",
			packageType: "Volume",
			wantErrCode: constant.ErrInvalidBillingPackageType.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			bp.Type = tt.packageType

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_MissingVolumeFields(t *testing.T) {
	t.Parallel()

	pricingModel := PricingModelTiered
	assetCode := "BRL"
	debitAlias := "debit-account"
	creditAlias := "credit-account"

	tests := []struct {
		name        string
		setup       func(*BillingPackage)
		wantErrCode string
	}{
		{
			name: "missing event filter",
			setup: func(bp *BillingPackage) {
				bp.EventFilter = nil
			},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
		{
			name: "missing pricing model",
			setup: func(bp *BillingPackage) {
				bp.PricingModel = nil
			},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
		{
			name: "missing tiers",
			setup: func(bp *BillingPackage) {
				bp.Tiers = nil
			},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
		{
			name: "missing asset code",
			setup: func(bp *BillingPackage) {
				bp.AssetCode = nil
			},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
		{
			name: "missing debit account alias",
			setup: func(bp *BillingPackage) {
				bp.DebitAccountAlias = nil
			},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
		{
			name: "missing credit account alias",
			setup: func(bp *BillingPackage) {
				bp.CreditAccountAlias = nil
			},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
		{
			name: "all volume fields missing",
			setup: func(bp *BillingPackage) {
				bp.EventFilter = nil
				bp.PricingModel = nil
				bp.Tiers = nil
				bp.AssetCode = nil
				bp.DebitAccountAlias = nil
				bp.CreditAccountAlias = nil
			},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := BillingPackage{
				ID:                 "pkg-001",
				OrganizationID:     "org-001",
				LedgerID:           "ledger-001",
				Label:              "Volume Package",
				Type:               BillingPackageTypeVolume,
				Enable:             boolPtr(true),
				EventFilter:        newValidEventFilter(),
				PricingModel:       &pricingModel,
				Tiers:              newValidPricingTiers(),
				AssetCode:          &assetCode,
				DebitAccountAlias:  &debitAlias,
				CreditAccountAlias: &creditAlias,
				CreatedAt:          "2026-01-01T00:00:00Z",
				UpdatedAt:          "2026-01-01T00:00:00Z",
			}
			tt.setup(&bp)

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_MissingMaintenanceFields(t *testing.T) {
	t.Parallel()

	feeAmount := decimal.NewFromFloat(50.00)
	creditAccount := "maintenance-credit-account"
	assetCode := "BRL"
	segID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	tests := []struct {
		name        string
		setup       func(*BillingPackage)
		wantErrCode string
	}{
		{
			name: "missing fee amount",
			setup: func(bp *BillingPackage) {
				bp.FeeAmount = nil
			},
			wantErrCode: constant.ErrMissingMaintenanceFields.Error(),
		},
		{
			name: "missing asset code",
			setup: func(bp *BillingPackage) {
				bp.AssetCode = nil
			},
			wantErrCode: constant.ErrMissingMaintenanceFields.Error(),
		},
		{
			name: "missing maintenance credit account",
			setup: func(bp *BillingPackage) {
				bp.MaintenanceCreditAccount = nil
			},
			wantErrCode: constant.ErrMissingMaintenanceFields.Error(),
		},
		{
			name: "missing account target",
			setup: func(bp *BillingPackage) {
				bp.AccountTarget = nil
			},
			wantErrCode: constant.ErrMissingMaintenanceFields.Error(),
		},
		{
			name: "zero fee amount",
			setup: func(bp *BillingPackage) {
				zero := decimal.NewFromInt(0)
				bp.FeeAmount = &zero
			},
			wantErrCode: constant.ErrInvalidFeeAmount.Error(),
		},
		{
			name: "negative fee amount",
			setup: func(bp *BillingPackage) {
				negative := decimal.NewFromFloat(-50.00)
				bp.FeeAmount = &negative
			},
			wantErrCode: constant.ErrInvalidFeeAmount.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := BillingPackage{
				ID:                       "pkg-002",
				OrganizationID:           "org-001",
				LedgerID:                 "ledger-001",
				Label:                    "Maintenance Package",
				Type:                     BillingPackageTypeMaintenance,
				Enable:                   boolPtr(true),
				FeeAmount:                &feeAmount,
				AssetCode:                &assetCode,
				MaintenanceCreditAccount: &creditAccount,
				AccountTarget: &AccountTarget{
					SegmentID: &segID,
				},
				CreatedAt: "2026-01-01T00:00:00Z",
				UpdatedAt: "2026-01-01T00:00:00Z",
			}
			tt.setup(&bp)

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_UnexpectedFieldsOnVolume(t *testing.T) {
	t.Parallel()

	pricingModel := PricingModelTiered
	assetCode := "BRL"
	debitAlias := "debit-alias"
	creditAlias := "credit-alias"
	feeAmount := decimal.NewFromFloat(50.00)
	creditAccount := "maintenance-credit-account"
	segID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	tests := []struct {
		name  string
		setup func(*BillingPackage)
	}{
		{
			name: "volume with feeAmount",
			setup: func(bp *BillingPackage) {
				bp.FeeAmount = &feeAmount
			},
		},
		{
			name: "volume with maintenanceCreditAccount",
			setup: func(bp *BillingPackage) {
				bp.MaintenanceCreditAccount = &creditAccount
			},
		},
		{
			name: "volume with accountTarget",
			setup: func(bp *BillingPackage) {
				bp.AccountTarget = &AccountTarget{SegmentID: &segID}
			},
		},
		{
			name: "volume with all maintenance fields",
			setup: func(bp *BillingPackage) {
				bp.FeeAmount = &feeAmount
				bp.MaintenanceCreditAccount = &creditAccount
				bp.AccountTarget = &AccountTarget{SegmentID: &segID}
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := BillingPackage{
				ID:                 "pkg-001",
				OrganizationID:     "org-001",
				LedgerID:           "ledger-001",
				Label:              "Volume Package",
				Type:               BillingPackageTypeVolume,
				Enable:             boolPtr(true),
				EventFilter:        newValidEventFilter(),
				PricingModel:       &pricingModel,
				Tiers:              newValidPricingTiers(),
				AssetCode:          &assetCode,
				DebitAccountAlias:  &debitAlias,
				CreditAccountAlias: &creditAlias,
				CreatedAt:          "2026-01-01T00:00:00Z",
				UpdatedAt:          "2026-01-01T00:00:00Z",
			}
			tt.setup(&bp)

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), constant.ErrUnexpectedFieldsInTheRequest.Error())
		})
	}
}

func TestBillingPackage_Validate_UnexpectedFieldsOnMaintenance(t *testing.T) {
	t.Parallel()

	feeAmount := decimal.NewFromFloat(50.00)
	assetCode := "BRL"
	creditAccount := "maintenance-credit-account"
	segID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	pricingModel := PricingModelTiered
	debitAlias := "debit-alias"
	creditAlias := "credit-alias"
	countMode := CountModePerRoute

	tests := []struct {
		name  string
		setup func(*BillingPackage)
	}{
		{
			name: "maintenance with pricingModel",
			setup: func(bp *BillingPackage) {
				bp.PricingModel = &pricingModel
			},
		},
		{
			name: "maintenance with tiers",
			setup: func(bp *BillingPackage) {
				bp.Tiers = newValidPricingTiers()
			},
		},
		{
			name: "maintenance with eventFilter",
			setup: func(bp *BillingPackage) {
				bp.EventFilter = newValidEventFilter()
			},
		},
		{
			name: "maintenance with freeQuota",
			setup: func(bp *BillingPackage) {
				fq := 100
				bp.FreeQuota = &fq
			},
		},
		{
			name: "maintenance with discountTiers",
			setup: func(bp *BillingPackage) {
				bp.DiscountTiers = []DiscountTier{
					{MinQuantity: 10, DiscountPercentage: decimal.NewFromFloat(0.10)},
				}
			},
		},
		{
			name: "maintenance with countMode",
			setup: func(bp *BillingPackage) {
				bp.CountMode = &countMode
			},
		},
		{
			name: "maintenance with debitAccountAlias",
			setup: func(bp *BillingPackage) {
				bp.DebitAccountAlias = &debitAlias
			},
		},
		{
			name: "maintenance with creditAccountAlias",
			setup: func(bp *BillingPackage) {
				bp.CreditAccountAlias = &creditAlias
			},
		},
		{
			name: "maintenance with all volume fields",
			setup: func(bp *BillingPackage) {
				bp.PricingModel = &pricingModel
				bp.Tiers = newValidPricingTiers()
				bp.EventFilter = newValidEventFilter()
				bp.DebitAccountAlias = &debitAlias
				bp.CreditAccountAlias = &creditAlias
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := BillingPackage{
				ID:                       "pkg-002",
				OrganizationID:           "org-001",
				LedgerID:                 "ledger-001",
				Label:                    "Maintenance Package",
				Type:                     BillingPackageTypeMaintenance,
				Enable:                   boolPtr(true),
				FeeAmount:                &feeAmount,
				AssetCode:                &assetCode,
				MaintenanceCreditAccount: &creditAccount,
				AccountTarget: &AccountTarget{
					SegmentID: &segID,
				},
				CreatedAt: "2026-01-01T00:00:00Z",
				UpdatedAt: "2026-01-01T00:00:00Z",
			}
			tt.setup(&bp)

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), constant.ErrUnexpectedFieldsInTheRequest.Error())
		})
	}
}

func TestBillingPackage_Validate_InvalidPricingModel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pricingModel string
		wantErrCode  string
	}{
		{
			name:         "unknown pricing model",
			pricingModel: "unknown",
			wantErrCode:  constant.ErrInvalidPricingModel.Error(),
		},
		{
			name:         "mixed case tiered",
			pricingModel: "Tiered",
			wantErrCode:  constant.ErrInvalidPricingModel.Error(),
		},
		{
			name:         "fixed uppercase",
			pricingModel: "FIXED",
			wantErrCode:  constant.ErrInvalidPricingModel.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			bp.PricingModel = &tt.pricingModel

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_InvalidPricingTiers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tiers       []PricingTier
		wantErrCode string
	}{
		{
			name: "tier with negative min quantity",
			tiers: []PricingTier{
				{
					MinQuantity: -1,
					UnitPrice:   decimal.NewFromFloat(1.00),
				},
			},
			wantErrCode: constant.ErrInvalidPricingTier.Error(),
		},
		{
			name: "tier with zero unit price",
			tiers: []PricingTier{
				{
					MinQuantity: 0,
					UnitPrice:   decimal.NewFromFloat(0),
				},
			},
			wantErrCode: constant.ErrInvalidPricingTier.Error(),
		},
		{
			name: "tier with negative unit price",
			tiers: []PricingTier{
				{
					MinQuantity: 0,
					UnitPrice:   decimal.NewFromFloat(-1.00),
				},
			},
			wantErrCode: constant.ErrInvalidPricingTier.Error(),
		},
		{
			name:        "empty tiers list",
			tiers:       []PricingTier{},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
		{
			name: "overlapping tiers",
			tiers: func() []PricingTier {
				max1 := int64(100)
				max2 := int64(150)

				return []PricingTier{
					{
						MinQuantity: 0,
						MaxQuantity: &max1,
						UnitPrice:   decimal.NewFromFloat(1.50),
					},
					{
						MinQuantity: 50,
						MaxQuantity: &max2,
						UnitPrice:   decimal.NewFromFloat(1.00),
					},
				}
			}(),
			wantErrCode: constant.ErrInvalidPricingTier.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			bp.Tiers = tt.tiers

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_InvalidFreeQuota(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		freeQuota   int
		wantErrCode string
	}{
		{
			name:        "negative free quota",
			freeQuota:   -1,
			wantErrCode: constant.ErrInvalidFreeQuota.Error(),
		},
		{
			name:        "very negative free quota",
			freeQuota:   -100,
			wantErrCode: constant.ErrInvalidFreeQuota.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			fq := tt.freeQuota
			bp.FreeQuota = &fq

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_ValidFreeQuota(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		freeQuota *int
	}{
		{
			name:      "nil free quota",
			freeQuota: nil,
		},
		{
			name: "zero free quota",
			freeQuota: func() *int {
				v := 0
				return &v
			}(),
		},
		{
			name: "positive free quota",
			freeQuota: func() *int {
				v := 10
				return &v
			}(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			bp.FreeQuota = tt.freeQuota

			err := bp.Validate()

			assert.NoError(t, err)
		})
	}
}

func TestBillingPackage_Validate_InvalidDiscountTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		discountTiers []DiscountTier
		wantErrCode   string
	}{
		{
			name: "discount percentage above 100",
			discountTiers: []DiscountTier{
				{
					MinQuantity:        0,
					DiscountPercentage: decimal.NewFromFloat(101),
				},
			},
			wantErrCode: constant.ErrInvalidDiscountTier.Error(),
		},
		{
			name: "discount percentage negative",
			discountTiers: []DiscountTier{
				{
					MinQuantity:        0,
					DiscountPercentage: decimal.NewFromFloat(-1),
				},
			},
			wantErrCode: constant.ErrInvalidDiscountTier.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			bp.DiscountTiers = tt.discountTiers

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_ValidDiscountTier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		discountTiers []DiscountTier
	}{
		{
			name:          "nil discount tiers",
			discountTiers: nil,
		},
		{
			name: "zero percent discount",
			discountTiers: []DiscountTier{
				{
					MinQuantity:        0,
					DiscountPercentage: decimal.NewFromFloat(0),
				},
			},
		},
		{
			name: "100 percent discount",
			discountTiers: []DiscountTier{
				{
					MinQuantity:        0,
					DiscountPercentage: decimal.NewFromFloat(100),
				},
			},
		},
		{
			name: "50 percent discount",
			discountTiers: []DiscountTier{
				{
					MinQuantity:        0,
					DiscountPercentage: decimal.NewFromFloat(50),
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			bp.DiscountTiers = tt.discountTiers

			err := bp.Validate()

			assert.NoError(t, err)
		})
	}
}

func TestBillingPackage_Validate_InvalidCountMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		countMode   string
		wantErrCode string
	}{
		{
			name:        "unknown count mode",
			countMode:   "unknown",
			wantErrCode: constant.ErrInvalidCountMode.Error(),
		},
		{
			name:        "mixed case perRoute",
			countMode:   "PerRoute",
			wantErrCode: constant.ErrInvalidCountMode.Error(),
		},
		{
			name:        "per_route with underscore",
			countMode:   "per_route",
			wantErrCode: constant.ErrInvalidCountMode.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			cm := tt.countMode
			bp.CountMode = &cm

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_ValidCountMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		countMode *string
	}{
		{
			name:      "nil count mode",
			countMode: nil,
		},
		{
			name: "perRoute count mode",
			countMode: func() *string {
				v := CountModePerRoute
				return &v
			}(),
		},
		{
			name: "perAccount count mode",
			countMode: func() *string {
				v := CountModePerAccount
				return &v
			}(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := newValidVolumeBillingPackage()
			bp.CountMode = tt.countMode

			err := bp.Validate()

			assert.NoError(t, err)
		})
	}
}

func TestAccountTarget_Validate(t *testing.T) {
	t.Parallel()

	segID := uuid.MustParse("00000000-0000-0000-0000-000000000010")
	portID := uuid.MustParse("00000000-0000-0000-0000-000000000011")

	tests := []struct {
		name        string
		target      AccountTarget
		wantErr     bool
		wantErrCode string
	}{
		{
			name: "valid with segment ID",
			target: AccountTarget{
				SegmentID: &segID,
			},
			wantErr: false,
		},
		{
			name: "valid with portfolio ID",
			target: AccountTarget{
				PortfolioID: &portID,
			},
			wantErr: false,
		},
		{
			name: "valid with aliases",
			target: AccountTarget{
				Aliases: []string{"alias-1", "alias-2"},
			},
			wantErr: false,
		},
		{
			name:        "none set",
			target:      AccountTarget{},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
		{
			name: "segment and portfolio both set",
			target: AccountTarget{
				SegmentID:   &segID,
				PortfolioID: &portID,
			},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
		{
			name: "segment and aliases both set",
			target: AccountTarget{
				SegmentID: &segID,
				Aliases:   []string{"alias-1"},
			},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
		{
			name: "portfolio and aliases both set",
			target: AccountTarget{
				PortfolioID: &portID,
				Aliases:     []string{"alias-1"},
			},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
		{
			name: "all three set",
			target: AccountTarget{
				SegmentID:   &segID,
				PortfolioID: &portID,
				Aliases:     []string{"alias-1"},
			},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
		{
			name: "nil UUID segmentId",
			target: AccountTarget{
				SegmentID: func() *uuid.UUID { id := uuid.Nil; return &id }(),
			},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
		{
			name: "nil UUID portfolioId",
			target: AccountTarget{
				PortfolioID: func() *uuid.UUID { id := uuid.Nil; return &id }(),
			},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
		{
			name: "aliases exceeds max 100",
			target: AccountTarget{
				Aliases: func() []string {
					aliases := make([]string, 101)
					for i := range aliases {
						aliases[i] = "alias"
					}

					return aliases
				}(),
			},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
		{
			name: "aliases exactly 100 is valid",
			target: AccountTarget{
				Aliases: func() []string {
					aliases := make([]string, 100)
					for i := range aliases {
						aliases[i] = "alias"
					}

					return aliases
				}(),
			},
			wantErr: false,
		},
		{
			name: "aliases with empty string",
			target: AccountTarget{
				Aliases: []string{"alias-1", "", "alias-3"},
			},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
		{
			name: "aliases with whitespace-only string",
			target: AccountTarget{
				Aliases: []string{"alias-1", "   ", "alias-3"},
			},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.target.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrCode)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBillingPackageUpdate_Validate(t *testing.T) {
	t.Parallel()

	validLabel := "Valid Label"
	emptyLabel := ""
	whitespaceLabel := "   "

	tests := []struct {
		name    string
		update  BillingPackageUpdate
		wantErr bool
	}{
		{
			name:    "nil label is valid",
			update:  BillingPackageUpdate{},
			wantErr: false,
		},
		{
			name:    "non-empty label is valid",
			update:  BillingPackageUpdate{Label: &validLabel},
			wantErr: false,
		},
		{
			name:    "empty string label is invalid",
			update:  BillingPackageUpdate{Label: &emptyLabel},
			wantErr: true,
		},
		{
			name:    "whitespace-only label is invalid",
			update:  BillingPackageUpdate{Label: &whitespaceLabel},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.update.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), constant.ErrMissingFieldsInRequest.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBillingPackageUpdate_ToMap(t *testing.T) {
	t.Parallel()

	label := "Updated Label"
	description := "Updated Description"
	enableTrue := true
	enableFalse := false
	emptyStr := ""

	tests := []struct {
		name     string
		update   BillingPackageUpdate
		expected map[string]any
	}{
		{
			name:     "all fields nil returns empty map",
			update:   BillingPackageUpdate{},
			expected: map[string]any{},
		},
		{
			name: "only label set",
			update: BillingPackageUpdate{
				Label: &label,
			},
			expected: map[string]any{
				"label": "Updated Label",
			},
		},
		{
			name: "only description set",
			update: BillingPackageUpdate{
				Description: &description,
			},
			expected: map[string]any{
				"description": "Updated Description",
			},
		},
		{
			name: "only enable set to true",
			update: BillingPackageUpdate{
				Enable: &enableTrue,
			},
			expected: map[string]any{
				"enable": true,
			},
		},
		{
			name: "only enable set to false",
			update: BillingPackageUpdate{
				Enable: &enableFalse,
			},
			expected: map[string]any{
				"enable": false,
			},
		},
		{
			name: "all fields set",
			update: BillingPackageUpdate{
				Label:       &label,
				Description: &description,
				Enable:      &enableTrue,
			},
			expected: map[string]any{
				"label":       "Updated Label",
				"description": "Updated Description",
				"enable":      true,
			},
		},
		{
			name: "empty string label is included",
			update: BillingPackageUpdate{
				Label: &emptyStr,
			},
			expected: map[string]any{
				"label": "",
			},
		},
		{
			name: "empty string description is included",
			update: BillingPackageUpdate{
				Description: &emptyStr,
			},
			expected: map[string]any{
				"description": "",
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := tt.update.ToMap()

			assert.Equal(t, len(tt.expected), len(result), "map length must match")

			for key, expectedVal := range tt.expected {
				actualVal, exists := result[key]
				assert.True(t, exists, "key %q must exist in result map", key)
				assert.Equal(t, expectedVal, actualVal, "value for key %q must match", key)
			}
		})
	}
}

func TestBillingPackage_Validate_AccountTargetInMaintenance(t *testing.T) {
	t.Parallel()

	feeAmount := decimal.NewFromFloat(50.00)
	creditAccount := "maintenance-credit-account"
	assetCode := "BRL"
	segID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	portID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	tests := []struct {
		name        string
		target      *AccountTarget
		wantErr     bool
		wantErrCode string
	}{
		{
			name:    "valid account target with segment",
			target:  &AccountTarget{SegmentID: &segID},
			wantErr: false,
		},
		{
			name:        "invalid account target - none set",
			target:      &AccountTarget{},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
		{
			name: "invalid account target - multiple set",
			target: &AccountTarget{
				SegmentID:   &segID,
				PortfolioID: &portID,
			},
			wantErr:     true,
			wantErrCode: constant.ErrInvalidAccountTarget.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := BillingPackage{
				ID:                       "pkg-002",
				OrganizationID:           "org-001",
				LedgerID:                 "ledger-001",
				Label:                    "Maintenance Package",
				Type:                     BillingPackageTypeMaintenance,
				Enable:                   boolPtr(true),
				FeeAmount:                &feeAmount,
				AssetCode:                &assetCode,
				MaintenanceCreditAccount: &creditAccount,
				AccountTarget:            tt.target,
				CreatedAt:                "2026-01-01T00:00:00Z",
				UpdatedAt:                "2026-01-01T00:00:00Z",
			}

			err := bp.Validate()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrCode)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestBillingPackage_Validate_BlankPointerBackedStrings(t *testing.T) {
	t.Parallel()

	blank := "   "
	valid := "BRL"
	debitAlias := "debit-account"
	creditAlias := "credit-account"
	pricingModel := PricingModelTiered
	feeAmount := decimal.NewFromFloat(50.00)
	creditAccount := "maintenance-credit-account"
	segID := uuid.MustParse("00000000-0000-0000-0000-000000000002")

	tests := []struct {
		name        string
		pkg         BillingPackage
		wantErrCode string
	}{
		{
			name: "volume: blank assetCode",
			pkg: BillingPackage{
				ID:                 "pkg-001",
				OrganizationID:     "org-001",
				LedgerID:           "ledger-001",
				Label:              "Volume Package",
				Type:               BillingPackageTypeVolume,
				Enable:             boolPtr(true),
				EventFilter:        newValidEventFilter(),
				PricingModel:       &pricingModel,
				Tiers:              newValidPricingTiers(),
				AssetCode:          &blank,
				DebitAccountAlias:  &debitAlias,
				CreditAccountAlias: &creditAlias,
				CreatedAt:          "2026-01-01T00:00:00Z",
				UpdatedAt:          "2026-01-01T00:00:00Z",
			},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
		{
			name: "volume: blank debitAccountAlias",
			pkg: BillingPackage{
				ID:                 "pkg-001",
				OrganizationID:     "org-001",
				LedgerID:           "ledger-001",
				Label:              "Volume Package",
				Type:               BillingPackageTypeVolume,
				Enable:             boolPtr(true),
				EventFilter:        newValidEventFilter(),
				PricingModel:       &pricingModel,
				Tiers:              newValidPricingTiers(),
				AssetCode:          &valid,
				DebitAccountAlias:  &blank,
				CreditAccountAlias: &creditAlias,
				CreatedAt:          "2026-01-01T00:00:00Z",
				UpdatedAt:          "2026-01-01T00:00:00Z",
			},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
		{
			name: "volume: blank creditAccountAlias",
			pkg: BillingPackage{
				ID:                 "pkg-001",
				OrganizationID:     "org-001",
				LedgerID:           "ledger-001",
				Label:              "Volume Package",
				Type:               BillingPackageTypeVolume,
				Enable:             boolPtr(true),
				EventFilter:        newValidEventFilter(),
				PricingModel:       &pricingModel,
				Tiers:              newValidPricingTiers(),
				AssetCode:          &valid,
				DebitAccountAlias:  &debitAlias,
				CreditAccountAlias: &blank,
				CreatedAt:          "2026-01-01T00:00:00Z",
				UpdatedAt:          "2026-01-01T00:00:00Z",
			},
			wantErrCode: constant.ErrMissingVolumeFields.Error(),
		},
		{
			name: "maintenance: blank assetCode",
			pkg: BillingPackage{
				ID:                       "pkg-002",
				OrganizationID:           "org-001",
				LedgerID:                 "ledger-001",
				Label:                    "Maintenance Package",
				Type:                     BillingPackageTypeMaintenance,
				Enable:                   boolPtr(true),
				FeeAmount:                &feeAmount,
				AssetCode:                &blank,
				MaintenanceCreditAccount: &creditAccount,
				AccountTarget:            &AccountTarget{SegmentID: &segID},
				CreatedAt:                "2026-01-01T00:00:00Z",
				UpdatedAt:                "2026-01-01T00:00:00Z",
			},
			wantErrCode: constant.ErrMissingMaintenanceFields.Error(),
		},
		{
			name: "maintenance: blank maintenanceCreditAccount",
			pkg: BillingPackage{
				ID:                       "pkg-002",
				OrganizationID:           "org-001",
				LedgerID:                 "ledger-001",
				Label:                    "Maintenance Package",
				Type:                     BillingPackageTypeMaintenance,
				Enable:                   boolPtr(true),
				FeeAmount:                &feeAmount,
				AssetCode:                &valid,
				MaintenanceCreditAccount: &blank,
				AccountTarget:            &AccountTarget{SegmentID: &segID},
				CreatedAt:                "2026-01-01T00:00:00Z",
				UpdatedAt:                "2026-01-01T00:00:00Z",
			},
			wantErrCode: constant.ErrMissingMaintenanceFields.Error(),
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.pkg.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrCode)
		})
	}
}

func TestBillingPackage_Validate_TooManyPricingTiers(t *testing.T) {
	t.Parallel()

	pricingModel := PricingModelTiered
	assetCode := "BRL"
	debitAlias := "debit-account"
	creditAlias := "credit-account"

	// Build 51 non-overlapping tiers (maxPricingTiers = 50).
	tiers := make([]PricingTier, 51)
	for i := 0; i < 51; i++ {
		min := int64(i * 10)
		max := int64(min + 9)
		tiers[i] = PricingTier{
			MinQuantity: min,
			MaxQuantity: &max,
			UnitPrice:   decimal.NewFromFloat(1.00),
		}
	}

	bp := BillingPackage{
		ID:                 "pkg-001",
		OrganizationID:     "org-001",
		LedgerID:           "ledger-001",
		Label:              "Volume Package",
		Type:               BillingPackageTypeVolume,
		Enable:             boolPtr(true),
		EventFilter:        newValidEventFilter(),
		PricingModel:       &pricingModel,
		Tiers:              tiers,
		AssetCode:          &assetCode,
		DebitAccountAlias:  &debitAlias,
		CreditAccountAlias: &creditAlias,
		CreatedAt:          "2026-01-01T00:00:00Z",
		UpdatedAt:          "2026-01-01T00:00:00Z",
	}

	err := bp.Validate()

	assert.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrInvalidPricingTier.Error())
}

func TestBillingPackage_Validate_PricingTierMaxQuantityNotGreaterThanMin(t *testing.T) {
	t.Parallel()

	pricingModel := PricingModelTiered
	assetCode := "BRL"
	debitAlias := "debit-account"
	creditAlias := "credit-account"

	tests := []struct {
		name string
		tier PricingTier
	}{
		{
			name: "maxQuantity equal to minQuantity",
			tier: PricingTier{
				MinQuantity: func() int64 { v := int64(10); return v }(),
				MaxQuantity: func() *int64 { v := int64(10); return &v }(),
				UnitPrice:   decimal.NewFromFloat(1.00),
			},
		},
		{
			name: "maxQuantity less than minQuantity",
			tier: PricingTier{
				MinQuantity: func() int64 { v := int64(20); return v }(),
				MaxQuantity: func() *int64 { v := int64(5); return &v }(),
				UnitPrice:   decimal.NewFromFloat(1.00),
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := BillingPackage{
				ID:                 "pkg-001",
				OrganizationID:     "org-001",
				LedgerID:           "ledger-001",
				Label:              "Volume Package",
				Type:               BillingPackageTypeVolume,
				Enable:             boolPtr(true),
				EventFilter:        newValidEventFilter(),
				PricingModel:       &pricingModel,
				Tiers:              []PricingTier{tt.tier},
				AssetCode:          &assetCode,
				DebitAccountAlias:  &debitAlias,
				CreditAccountAlias: &creditAlias,
				CreatedAt:          "2026-01-01T00:00:00Z",
				UpdatedAt:          "2026-01-01T00:00:00Z",
			}

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), constant.ErrInvalidPricingTier.Error())
		})
	}
}

func TestBillingPackage_Validate_InvalidDiscountTiers(t *testing.T) {
	t.Parallel()

	pricingModel := PricingModelTiered
	assetCode := "BRL"
	debitAlias := "debit-account"
	creditAlias := "credit-account"

	tests := []struct {
		name         string
		discountTier DiscountTier
	}{
		{
			name: "negative minQuantity",
			discountTier: DiscountTier{
				MinQuantity:        -1,
				DiscountPercentage: decimal.NewFromFloat(10.00),
			},
		},
		{
			name: "negative discountPercentage",
			discountTier: DiscountTier{
				MinQuantity:        10,
				DiscountPercentage: decimal.NewFromFloat(-1.00),
			},
		},
		{
			name: "discountPercentage above 100",
			discountTier: DiscountTier{
				MinQuantity:        10,
				DiscountPercentage: decimal.NewFromFloat(101.00),
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			bp := BillingPackage{
				ID:                 "pkg-001",
				OrganizationID:     "org-001",
				LedgerID:           "ledger-001",
				Label:              "Volume Package",
				Type:               BillingPackageTypeVolume,
				Enable:             boolPtr(true),
				EventFilter:        newValidEventFilter(),
				PricingModel:       &pricingModel,
				Tiers:              newValidPricingTiers(),
				DiscountTiers:      []DiscountTier{tt.discountTier},
				AssetCode:          &assetCode,
				DebitAccountAlias:  &debitAlias,
				CreditAccountAlias: &creditAlias,
				CreatedAt:          "2026-01-01T00:00:00Z",
				UpdatedAt:          "2026-01-01T00:00:00Z",
			}

			err := bp.Validate()

			assert.Error(t, err)
			assert.Contains(t, err.Error(), constant.ErrInvalidDiscountTier.Error())
		})
	}
}
