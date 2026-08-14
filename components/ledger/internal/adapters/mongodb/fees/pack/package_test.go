// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package pack

import (
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/bsondecimal"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestPackage_GetSegmentID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		package_ *Package
		expected uuid.UUID
	}{
		{
			name: "With segment ID",
			package_: &Package{
				SegmentID: func() *uuid.UUID {
					id := uuid.New()
					return &id
				}(),
			},
			expected: func() uuid.UUID {
				id := uuid.New()
				return id
			}(),
		},
		{
			name: "Without segment ID (nil)",
			package_: &Package{
				SegmentID: nil,
			},
			expected: uuid.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.package_.SegmentID != nil {
				tt.expected = *tt.package_.SegmentID
			}
			result := tt.package_.GetSegmentID()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPackage_GetTransactionRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		package_ *Package
		expected string
	}{
		{
			name: "With transaction route",
			package_: &Package{
				TransactionRoute: stringPtr("debitoted"),
			},
			expected: "debitoted",
		},
		{
			name: "Without transaction route (nil)",
			package_: &Package{
				TransactionRoute: nil,
			},
			expected: "",
		},
		{
			name: "With empty transaction route",
			package_: &Package{
				TransactionRoute: stringPtr(""),
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.package_.GetTransactionRoute()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPackageMongoDBModel_ToEntity(t *testing.T) {
	t.Parallel()

	now := time.Now()
	segmentID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()
	packageID := uuid.New()
	waivedAccounts := []string{"acc001", "acc002"}
	enable := true

	feeKey := "feeKey1"
	fee := Fee{
		FeeLabel:        "Test Fee",
		ReferenceAmount: "originalAmount",
		Priority:        1,
		CreditAccount:   "credit_account",
		CalculationModel: CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations: []Calculation{
				{
					Type:  "percentage",
					Value: bsondecimal.Decimal{Decimal: decimal.NewFromInt(10)},
				},
			},
		},
	}

	pmm := &PackageMongoDBModel{
		ID:               packageID,
		FeeGroupLabel:    "Test Package",
		Description:      stringPtr("Test Description"),
		OrganizationID:   orgID,
		SegmentID:        &segmentID,
		LedgerID:         ledgerID,
		TransactionRoute: stringPtr("debitoted"),
		MinimumAmount:    bsondecimal.Decimal{Decimal: decimal.NewFromInt(100)},
		MaximumAmount:    bsondecimal.Decimal{Decimal: decimal.NewFromInt(1000)},
		WaivedAccounts:   &waivedAccounts,
		Fees:             map[string]Fee{feeKey: fee},
		Enable:           &enable,
		CreatedAt:        now,
		UpdatedAt:        now,
		DeletedAt:        nil,
	}

	result := pmm.ToEntity()

	assert.NotNil(t, result)
	assert.Equal(t, packageID, result.ID)
	assert.Equal(t, "Test Package", result.FeeGroupLabel)
	assert.Equal(t, "Test Description", *result.Description)
	assert.Equal(t, segmentID, *result.SegmentID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Equal(t, "debitoted", *result.TransactionRoute)
	assert.Equal(t, decimal.NewFromInt(100), result.MinimumAmount)
	assert.Equal(t, decimal.NewFromInt(1000), result.MaximumAmount)
	assert.Equal(t, waivedAccounts, *result.WaivedAccounts)
	assert.Equal(t, &enable, result.Enable)
	assert.Equal(t, now, result.CreatedAt)
	assert.Equal(t, now, result.UpdatedAt)
	assert.Nil(t, result.DeletedAt)
	assert.NotNil(t, result.Fees)
	assert.Contains(t, result.Fees, feeKey)
}

func TestPackageMongoDBModel_FromEntity(t *testing.T) {
	t.Parallel()

	now := time.Now()
	segmentID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()
	packageID := uuid.New()
	waivedAccounts := []string{"acc001", "acc002"}
	enable := true

	feeKey := "feeKey1"
	fee := model.Fee{
		FeeLabel:        "Test Fee",
		ReferenceAmount: "originalAmount",
		Priority:        1,
		CreditAccount:   "credit_account",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations: []model.Calculation{
				{
					Type:  "percentage",
					Value: "10",
				},
			},
		},
	}

	p := &Package{
		ID:               packageID,
		FeeGroupLabel:    "Test Package",
		Description:      stringPtr("Test Description"),
		SegmentID:        &segmentID,
		LedgerID:         ledgerID,
		TransactionRoute: stringPtr("debitoted"),
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		WaivedAccounts:   &waivedAccounts,
		Fees:             map[string]model.Fee{feeKey: fee},
		Enable:           &enable,
		CreatedAt:        now,
		UpdatedAt:        now,
		DeletedAt:        nil,
	}

	pmm := &PackageMongoDBModel{}
	err := pmm.FromEntity(p, orgID)

	assert.NoError(t, err)
	assert.Equal(t, packageID, pmm.ID)
	assert.Equal(t, "Test Package", pmm.FeeGroupLabel)
	assert.Equal(t, "Test Description", *pmm.Description)
	assert.Equal(t, orgID, pmm.OrganizationID)
	assert.Equal(t, segmentID, *pmm.SegmentID)
	assert.Equal(t, ledgerID, pmm.LedgerID)
	assert.Equal(t, "debitoted", *pmm.TransactionRoute)
	assert.Equal(t, decimal.NewFromInt(100), pmm.MinimumAmount.Decimal)
	assert.Equal(t, decimal.NewFromInt(1000), pmm.MaximumAmount.Decimal)
	assert.Equal(t, waivedAccounts, *pmm.WaivedAccounts)
	assert.Equal(t, &enable, pmm.Enable)
	assert.Equal(t, now, pmm.CreatedAt)
	assert.Equal(t, now, pmm.UpdatedAt)
	assert.NotNil(t, pmm.Fees)
}

func TestToEntityFeeMap(t *testing.T) {
	t.Parallel()

	feeKey := "feeKey1"
	fee := Fee{
		FeeLabel:         "Test Fee",
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: boolPtr(true),
		CreditAccount:    "credit_account",
		RouteFrom:        stringPtr("route_from"),
		RouteTo:          stringPtr("route_to"),
		CalculationModel: CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations: []Calculation{
				{
					Type:  "percentage",
					Value: bsondecimal.Decimal{Decimal: decimal.NewFromInt(10)},
				},
				{
					Type:  "flat",
					Value: bsondecimal.Decimal{Decimal: decimal.NewFromInt(5)},
				},
			},
		},
	}

	fees := map[string]Fee{
		feeKey: fee,
	}

	result := ToEntityFeeMap(fees)

	assert.NotNil(t, result)
	assert.Contains(t, result, feeKey)
	assert.Equal(t, "Test Fee", result[feeKey].FeeLabel)
	assert.Equal(t, "originalAmount", result[feeKey].ReferenceAmount)
	assert.Equal(t, 1, result[feeKey].Priority)
	assert.Equal(t, true, *result[feeKey].IsDeductibleFrom)
	assert.Equal(t, "credit_account", result[feeKey].CreditAccount)
	assert.Equal(t, "route_from", *result[feeKey].RouteFrom)
	assert.Equal(t, "route_to", *result[feeKey].RouteTo)
	assert.NotNil(t, result[feeKey].CalculationModel)
	assert.Equal(t, "maxBetweenTypes", result[feeKey].CalculationModel.ApplicationRule)
	assert.Len(t, result[feeKey].CalculationModel.Calculations, 2)
	assert.Equal(t, "percentage", result[feeKey].CalculationModel.Calculations[0].Type)
	assert.Equal(t, "10", result[feeKey].CalculationModel.Calculations[0].Value)
	assert.Equal(t, "flat", result[feeKey].CalculationModel.Calculations[1].Type)
	assert.Equal(t, "5", result[feeKey].CalculationModel.Calculations[1].Value)
}

func TestToEntityFeeMap_EmptyMap(t *testing.T) {
	t.Parallel()

	fees := map[string]Fee{}
	result := ToEntityFeeMap(fees)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestFromEntityFeeMap(t *testing.T) {
	t.Parallel()

	feeKey := "feeKey1"
	fee := model.Fee{
		FeeLabel:         "Test Fee",
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: boolPtr(true),
		CreditAccount:    "credit_account",
		RouteFrom:        stringPtr("route_from"),
		RouteTo:          stringPtr("route_to"),
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations: []model.Calculation{
				{
					Type:  "percentage",
					Value: "10",
				},
				{
					Type:  "flat",
					Value: "5",
				},
			},
		},
	}

	fees := map[string]model.Fee{
		feeKey: fee,
	}

	result, err := FromEntityFeeMap(fees)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, result, "feeKey1")
	assert.Equal(t, "Test Fee", result["feeKey1"].FeeLabel)
	assert.Equal(t, "originalAmount", result["feeKey1"].ReferenceAmount)
	assert.Equal(t, 1, result["feeKey1"].Priority)
	assert.Equal(t, true, *result["feeKey1"].IsDeductibleFrom)
	assert.Equal(t, "credit_account", result["feeKey1"].CreditAccount)
	assert.Equal(t, "route_from", *result["feeKey1"].RouteFrom)
	assert.Equal(t, "route_to", *result["feeKey1"].RouteTo)
	assert.Equal(t, "maxBetweenTypes", result["feeKey1"].CalculationModel.ApplicationRule)
	assert.Len(t, result["feeKey1"].CalculationModel.Calculations, 2)
	assert.Equal(t, "percentage", result["feeKey1"].CalculationModel.Calculations[0].Type)
	assert.Equal(t, decimal.NewFromInt(10), result["feeKey1"].CalculationModel.Calculations[0].Value.Decimal)
	assert.Equal(t, "flat", result["feeKey1"].CalculationModel.Calculations[1].Type)
	assert.Equal(t, decimal.NewFromInt(5), result["feeKey1"].CalculationModel.Calculations[1].Value.Decimal)
}

func TestFromEntityFeeMap_EmptyRouteFromAndRouteTo(t *testing.T) {
	t.Parallel()

	feeKey := "feeKey1"
	fee := model.Fee{
		FeeLabel:        "Test Fee",
		ReferenceAmount: "originalAmount",
		Priority:        1,
		CreditAccount:   "credit_account",
		RouteFrom:       stringPtr(""),
		RouteTo:         stringPtr(""),
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations:    []model.Calculation{},
		},
	}

	fees := map[string]model.Fee{
		feeKey: fee,
	}

	result, err := FromEntityFeeMap(fees)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, result, "feeKey1")
	assert.Nil(t, result["feeKey1"].RouteFrom)
	assert.Nil(t, result["feeKey1"].RouteTo)
}

func TestFromEntityFeeMap_NilRouteFromAndRouteTo(t *testing.T) {
	t.Parallel()

	feeKey := "feeKey1"
	fee := model.Fee{
		FeeLabel:        "Test Fee",
		ReferenceAmount: "originalAmount",
		Priority:        1,
		CreditAccount:   "credit_account",
		RouteFrom:       nil,
		RouteTo:         nil,
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations:    []model.Calculation{},
		},
	}

	fees := map[string]model.Fee{
		feeKey: fee,
	}

	result, err := FromEntityFeeMap(fees)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Contains(t, result, "feeKey1")
	assert.Nil(t, result["feeKey1"].RouteFrom)
	assert.Nil(t, result["feeKey1"].RouteTo)
}

func TestFromEntityFeeMap_EmptyMap(t *testing.T) {
	t.Parallel()

	fees := map[string]model.Fee{}
	result, err := FromEntityFeeMap(fees)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestToEntityCalculationArray(t *testing.T) {
	t.Parallel()

	calculations := []Calculation{
		{
			Type:  "percentage",
			Value: bsondecimal.Decimal{Decimal: decimal.NewFromInt(10)},
		},
		{
			Type:  "flat",
			Value: bsondecimal.Decimal{Decimal: decimal.NewFromFloat(5.5)},
		},
	}

	result := ToEntityCalculationArray(calculations)

	assert.Len(t, result, 2)
	assert.Equal(t, "percentage", result[0].Type)
	assert.Equal(t, "10", result[0].Value)
	assert.Equal(t, "flat", result[1].Type)
	assert.Equal(t, "5.5", result[1].Value)
}

func TestToEntityCalculationArray_Empty(t *testing.T) {
	t.Parallel()

	calculations := []Calculation{}
	result := ToEntityCalculationArray(calculations)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestFromEntityCalculationArray(t *testing.T) {
	t.Parallel()

	calculations := []model.Calculation{
		{
			Type:  "percentage",
			Value: "10",
		},
		{
			Type:  "flat",
			Value: "5.5",
		},
	}

	result, err := FromEntityCalculationArray(calculations)

	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "percentage", result[0].Type)
	assert.Equal(t, decimal.NewFromInt(10), result[0].Value.Decimal)
	assert.Equal(t, "flat", result[1].Type)
	assert.Equal(t, decimal.NewFromFloat(5.5), result[1].Value.Decimal)
}

func TestFromEntityCalculationArray_Empty(t *testing.T) {
	t.Parallel()

	calculations := []model.Calculation{}
	result, err := FromEntityCalculationArray(calculations)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestFromEntityCalculationArray_InvalidDecimal(t *testing.T) {
	t.Parallel()

	calculations := []model.Calculation{
		{
			Type:  "percentage",
			Value: "invalid",
		},
	}

	result, err := FromEntityCalculationArray(calculations)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "invalid decimal value")
}

func TestPackageMongoDBModel_ToEntity_WithNilFields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	ledgerID := uuid.New()
	packageID := uuid.New()

	pmm := &PackageMongoDBModel{
		ID:               packageID,
		FeeGroupLabel:    "Test Package",
		Description:      nil,
		SegmentID:        nil,
		LedgerID:         ledgerID,
		TransactionRoute: nil,
		MinimumAmount:    bsondecimal.Decimal{Decimal: decimal.Zero},
		MaximumAmount:    bsondecimal.Decimal{Decimal: decimal.Zero},
		WaivedAccounts:   nil,
		Fees:             map[string]Fee{},
		Enable:           nil,
		CreatedAt:        now,
		UpdatedAt:        now,
		DeletedAt:        func() *time.Time { t := now; return &t }(),
	}

	result := pmm.ToEntity()

	assert.NotNil(t, result)
	assert.Equal(t, packageID, result.ID)
	assert.Equal(t, "Test Package", result.FeeGroupLabel)
	assert.Nil(t, result.Description)
	assert.Nil(t, result.SegmentID)
	assert.Equal(t, ledgerID, result.LedgerID)
	assert.Nil(t, result.TransactionRoute)
	assert.True(t, result.MinimumAmount.IsZero())
	assert.True(t, result.MaximumAmount.IsZero())
	assert.Nil(t, result.WaivedAccounts)
	assert.Nil(t, result.Enable)
	assert.NotNil(t, result.DeletedAt)
	assert.NotNil(t, result.Fees)
	assert.Empty(t, result.Fees)
}

func TestPackageMongoDBModel_FromEntity_WithNilFields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	orgID := uuid.New()
	ledgerID := uuid.New()
	packageID := uuid.New()

	p := &Package{
		ID:               packageID,
		FeeGroupLabel:    "Test Package",
		Description:      nil,
		SegmentID:        nil,
		LedgerID:         ledgerID,
		TransactionRoute: nil,
		MinimumAmount:    decimal.Zero,
		MaximumAmount:    decimal.Zero,
		WaivedAccounts:   nil,
		Fees:             map[string]model.Fee{},
		Enable:           nil,
		CreatedAt:        now,
		UpdatedAt:        now,
		DeletedAt:        nil,
	}

	pmm := &PackageMongoDBModel{}
	err := pmm.FromEntity(p, orgID)

	assert.NoError(t, err)
	assert.Equal(t, packageID, pmm.ID)
	assert.Equal(t, "Test Package", pmm.FeeGroupLabel)
	assert.Nil(t, pmm.Description)
	assert.Equal(t, orgID, pmm.OrganizationID)
	assert.Nil(t, pmm.SegmentID)
	assert.Equal(t, ledgerID, pmm.LedgerID)
	assert.Nil(t, pmm.TransactionRoute)
	assert.True(t, pmm.MinimumAmount.Decimal.IsZero())
	assert.True(t, pmm.MaximumAmount.Decimal.IsZero())
	assert.Nil(t, pmm.WaivedAccounts)
	assert.Nil(t, pmm.Enable)
	assert.NotNil(t, pmm.Fees)
	assert.Empty(t, pmm.Fees)
}

func TestPackageMongoDBModel_RoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now()
	segmentID := uuid.New()
	orgID := uuid.New()
	ledgerID := uuid.New()
	packageID := uuid.New()
	waivedAccounts := []string{"acc001", "acc002"}
	enable := true

	feeKey := "feeKey1"
	fee := model.Fee{
		FeeLabel:         "Test Fee",
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: boolPtr(true),
		CreditAccount:    "credit_account",
		RouteFrom:        stringPtr("route_from"),
		RouteTo:          stringPtr("route_to"),
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{
				{
					Type:  "flat",
					Value: "50.75",
				},
			},
		},
	}

	original := &Package{
		ID:               packageID,
		FeeGroupLabel:    "Test Package",
		Description:      stringPtr("Test Description"),
		SegmentID:        &segmentID,
		LedgerID:         ledgerID,
		TransactionRoute: stringPtr("debitoted"),
		MinimumAmount:    decimal.NewFromInt(100),
		MaximumAmount:    decimal.NewFromInt(1000),
		WaivedAccounts:   &waivedAccounts,
		Fees:             map[string]model.Fee{feeKey: fee},
		Enable:           &enable,
		CreatedAt:        now,
		UpdatedAt:        now,
		DeletedAt:        nil,
	}

	pmm := &PackageMongoDBModel{}
	err := pmm.FromEntity(original, orgID)
	assert.NoError(t, err)

	converted := pmm.ToEntity()

	assert.Equal(t, original.ID, converted.ID)
	assert.Equal(t, original.FeeGroupLabel, converted.FeeGroupLabel)
	assert.Equal(t, *original.Description, *converted.Description)
	assert.Equal(t, *original.SegmentID, *converted.SegmentID)
	assert.Equal(t, original.LedgerID, converted.LedgerID)
	assert.Equal(t, *original.TransactionRoute, *converted.TransactionRoute)
	assert.Equal(t, original.MinimumAmount, converted.MinimumAmount)
	assert.Equal(t, original.MaximumAmount, converted.MaximumAmount)
	assert.Equal(t, *original.WaivedAccounts, *converted.WaivedAccounts)
	assert.Equal(t, *original.Enable, *converted.Enable)
	assert.Equal(t, original.CreatedAt, converted.CreatedAt)
	assert.Equal(t, original.UpdatedAt, converted.UpdatedAt)
	assert.Equal(t, original.DeletedAt, converted.DeletedAt)
	assert.Len(t, converted.Fees, 1)
	assert.Contains(t, converted.Fees, feeKey)
}

func TestToEntityFeeMap_MultipleFees(t *testing.T) {
	t.Parallel()

	fee1 := Fee{
		FeeLabel:         "Fee 1",
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: boolPtr(true),
		CreditAccount:    "credit_account_1",
		CalculationModel: CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []Calculation{
				{
					Type:  "flat",
					Value: bsondecimal.Decimal{Decimal: decimal.NewFromInt(10)},
				},
			},
		},
	}

	fee2 := Fee{
		FeeLabel:         "Fee 2",
		ReferenceAmount:  "afterFeesAmount",
		Priority:         2,
		IsDeductibleFrom: boolPtr(false),
		CreditAccount:    "credit_account_2",
		CalculationModel: CalculationModel{
			ApplicationRule: "percentual",
			Calculations: []Calculation{
				{
					Type:  "percentage",
					Value: bsondecimal.Decimal{Decimal: decimal.NewFromInt(5)},
				},
			},
		},
	}

	fees := map[string]Fee{
		"feeKey1": fee1,
		"feeKey2": fee2,
	}

	result := ToEntityFeeMap(fees)

	assert.Len(t, result, 2)
	assert.Contains(t, result, "feeKey1")
	assert.Contains(t, result, "feeKey2")
	assert.Equal(t, "Fee 1", result["feeKey1"].FeeLabel)
	assert.Equal(t, "Fee 2", result["feeKey2"].FeeLabel)
	assert.Equal(t, "flatFee", result["feeKey1"].CalculationModel.ApplicationRule)
	assert.Equal(t, "percentual", result["feeKey2"].CalculationModel.ApplicationRule)
}

func TestFromEntityFeeMap_MultipleFees(t *testing.T) {
	t.Parallel()

	fee1 := model.Fee{
		FeeLabel:         "Fee 1",
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: boolPtr(true),
		CreditAccount:    "credit_account_1",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{
				{
					Type:  "flat",
					Value: "10",
				},
			},
		},
	}

	fee2 := model.Fee{
		FeeLabel:         "Fee 2",
		ReferenceAmount:  "afterFeesAmount",
		Priority:         2,
		IsDeductibleFrom: boolPtr(false),
		CreditAccount:    "credit_account_2",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "percentual",
			Calculations: []model.Calculation{
				{
					Type:  "percentage",
					Value: "5",
				},
			},
		},
	}

	fees := map[string]model.Fee{
		"FeeKey1": fee1,
		"FeeKey2": fee2,
	}

	result, err := FromEntityFeeMap(fees)

	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Contains(t, result, "feeKey1")
	assert.Contains(t, result, "feeKey2")
	assert.Equal(t, "Fee 1", result["feeKey1"].FeeLabel)
	assert.Equal(t, "Fee 2", result["feeKey2"].FeeLabel)
}

func TestFromEntityFeeMap_KeyCaseConversion(t *testing.T) {
	t.Parallel()

	fee := model.Fee{
		FeeLabel:        "Test Fee",
		ReferenceAmount: "originalAmount",
		Priority:        1,
		CreditAccount:   "credit_account",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "maxBetweenTypes",
			Calculations:    []model.Calculation{},
		},
	}

	tests := []struct {
		name     string
		inputKey string
		expected string
	}{
		{
			name:     "PascalCase to lowerCamelCase",
			inputKey: "FeeKey",
			expected: "feeKey",
		},
		{
			name:     "UPPER_CASE to lowerCamelCase",
			inputKey: "FEE_KEY",
			expected: "feeKey",
		},
		{
			name:     "snake_case to lowerCamelCase",
			inputKey: "fee_key",
			expected: "feeKey",
		},
		{
			name:     "Already lowerCamelCase",
			inputKey: "feeKey",
			expected: "feeKey",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fees := map[string]model.Fee{
				tt.inputKey: fee,
			}

			result, err := FromEntityFeeMap(fees)

			assert.NoError(t, err)
			assert.Contains(t, result, tt.expected)
			assert.Equal(t, "Test Fee", result[tt.expected].FeeLabel)
		})
	}
}

func TestToEntityFeeMap_WithNilIsDeductibleFrom(t *testing.T) {
	t.Parallel()

	fee := Fee{
		FeeLabel:         "Test Fee",
		ReferenceAmount:  "originalAmount",
		Priority:         1,
		IsDeductibleFrom: nil,
		CreditAccount:    "credit_account",
		CalculationModel: CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []Calculation{
				{
					Type:  "flat",
					Value: bsondecimal.Decimal{Decimal: decimal.NewFromInt(10)},
				},
			},
		},
	}

	fees := map[string]Fee{
		"feeKey1": fee,
	}

	result := ToEntityFeeMap(fees)

	assert.NotNil(t, result)
	assert.Contains(t, result, "feeKey1")
	assert.Nil(t, result["feeKey1"].IsDeductibleFrom)
}

func TestToEntityCalculationArray_WithZeroValue(t *testing.T) {
	t.Parallel()

	calculations := []Calculation{
		{
			Type:  "percentage",
			Value: bsondecimal.Decimal{Decimal: decimal.Zero},
		},
		{
			Type:  "flat",
			Value: bsondecimal.Decimal{Decimal: decimal.NewFromFloat(0.0)},
		},
	}

	result := ToEntityCalculationArray(calculations)

	assert.Len(t, result, 2)
	assert.Equal(t, "percentage", result[0].Type)
	assert.Equal(t, "0", result[0].Value)
	assert.Equal(t, "flat", result[1].Type)
	assert.Equal(t, "0", result[1].Value)
}

func TestFromEntityCalculationArray_WithZeroValue(t *testing.T) {
	t.Parallel()

	calculations := []model.Calculation{
		{
			Type:  "percentage",
			Value: "0",
		},
		{
			Type:  "flat",
			Value: "0.0",
		},
	}

	result, err := FromEntityCalculationArray(calculations)

	assert.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "percentage", result[0].Type)
	assert.True(t, result[0].Value.Decimal.IsZero())
	assert.Equal(t, "flat", result[1].Type)
	assert.True(t, result[1].Value.Decimal.IsZero())
}

func TestFromEntityCalculationArray_WithLargeDecimal(t *testing.T) {
	t.Parallel()

	calculations := []model.Calculation{
		{
			Type:  "flat",
			Value: "999999999.999999999",
		},
	}

	result, err := FromEntityCalculationArray(calculations)

	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "flat", result[0].Type)
	expected, _ := decimal.NewFromString("999999999.999999999")
	assert.Equal(t, expected, result[0].Value.Decimal)
}

func TestToEntityCalculationArray_WithLargeDecimal(t *testing.T) {
	t.Parallel()

	largeDecimal, _ := decimal.NewFromString("999999999.999999999")
	calculations := []Calculation{
		{
			Type:  "flat",
			Value: bsondecimal.Decimal{Decimal: largeDecimal},
		},
	}

	result := ToEntityCalculationArray(calculations)

	assert.Len(t, result, 1)
	assert.Equal(t, "flat", result[0].Type)
	assert.Equal(t, "999999999.999999999", result[0].Value)
}

func TestToEntityFeeMap_WithAllApplicationRules(t *testing.T) {
	t.Parallel()

	rules := []string{"flatFee", "percentual", "maxBetweenTypes"}

	for _, rule := range rules {
		t.Run(rule, func(t *testing.T) {
			fee := Fee{
				FeeLabel:        "Test Fee",
				ReferenceAmount: "originalAmount",
				Priority:        1,
				CreditAccount:   "credit_account",
				CalculationModel: CalculationModel{
					ApplicationRule: rule,
					Calculations: []Calculation{
						{
							Type:  "flat",
							Value: bsondecimal.Decimal{Decimal: decimal.NewFromInt(10)},
						},
					},
				},
			}

			fees := map[string]Fee{
				"feeKey1": fee,
			}

			result := ToEntityFeeMap(fees)

			assert.NotNil(t, result)
			assert.Contains(t, result, "feeKey1")
			assert.Equal(t, rule, result["feeKey1"].CalculationModel.ApplicationRule)
		})
	}
}

func TestFromEntityFeeMap_WithAllApplicationRules(t *testing.T) {
	t.Parallel()

	rules := []string{"flatFee", "percentual", "maxBetweenTypes"}

	for _, rule := range rules {
		t.Run(rule, func(t *testing.T) {
			fee := model.Fee{
				FeeLabel:        "Test Fee",
				ReferenceAmount: "originalAmount",
				Priority:        1,
				CreditAccount:   "credit_account",
				CalculationModel: &model.CalculationModel{
					ApplicationRule: rule,
					Calculations: []model.Calculation{
						{
							Type:  "flat",
							Value: "10",
						},
					},
				},
			}

			fees := map[string]model.Fee{
				"feeKey1": fee,
			}

			result, err := FromEntityFeeMap(fees)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Contains(t, result, "feeKey1")
			assert.Equal(t, rule, result["feeKey1"].CalculationModel.ApplicationRule)
		})
	}
}

func TestNewPackage_ValidInputs(t *testing.T) {
	t.Parallel()

	orgID := uuid.New()
	ledgerID := uuid.New()
	enable := true
	fees := map[string]model.Fee{
		"testFee": {
			FeeLabel:        "Test Fee",
			ReferenceAmount: "originalAmount",
			Priority:        1,
			CreditAccount:   "credit_account",
			CalculationModel: &model.CalculationModel{
				ApplicationRule: "flatFee",
				Calculations: []model.Calculation{
					{Type: "flat", Value: "10"},
				},
			},
		},
	}

	p, err := NewPackage(
		orgID,
		ledgerID,
		"Test Package",
		decimal.NewFromInt(100),
		decimal.NewFromInt(1000),
		fees,
		&enable,
	)

	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.NotEqual(t, uuid.Nil, p.ID)
	assert.Equal(t, "Test Package", p.FeeGroupLabel)
	assert.Equal(t, ledgerID, p.LedgerID)
	assert.Equal(t, decimal.NewFromInt(100), p.MinimumAmount)
	assert.Equal(t, decimal.NewFromInt(1000), p.MaximumAmount)
	assert.Equal(t, &enable, p.Enable)
	assert.NotNil(t, p.Fees)
	assert.Contains(t, p.Fees, "testFee")
	assert.False(t, p.CreatedAt.IsZero())
	assert.False(t, p.UpdatedAt.IsZero())
	assert.Equal(t, p.CreatedAt, p.UpdatedAt)
}

func TestNewPackage_ValidationErrors(t *testing.T) {
	t.Parallel()

	validOrgID := uuid.New()
	validLedgerID := uuid.New()
	enable := true

	tests := []struct {
		name           string
		organizationID uuid.UUID
		ledgerID       uuid.UUID
		feeGroupLabel  string
		expectedErr    error
	}{
		{
			name:           "Missing OrganizationID",
			organizationID: uuid.Nil,
			ledgerID:       validLedgerID,
			feeGroupLabel:  "Test",
			expectedErr:    ErrMissingOrganizationID,
		},
		{
			name:           "Missing LedgerID",
			organizationID: validOrgID,
			ledgerID:       uuid.Nil,
			feeGroupLabel:  "Test",
			expectedErr:    ErrMissingLedgerID,
		},
		{
			name:           "Missing FeeGroupLabel (empty string)",
			organizationID: validOrgID,
			ledgerID:       validLedgerID,
			feeGroupLabel:  "",
			expectedErr:    ErrMissingName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, err := NewPackage(
				tt.organizationID,
				tt.ledgerID,
				tt.feeGroupLabel,
				decimal.NewFromInt(100),
				decimal.NewFromInt(1000),
				nil,
				&enable,
			)

			assert.Nil(t, p)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestReconstructPackage(t *testing.T) {
	t.Parallel()

	now := time.Now()
	id := uuid.New()
	segmentID := uuid.New()
	ledgerID := uuid.New()
	desc := "Test Description"
	route := "debitoted"
	waivedAccounts := []string{"acc001"}
	enable := true
	deletedAt := now.Add(time.Hour)

	fees := map[string]model.Fee{
		"testFee": {
			FeeLabel:        "Test Fee",
			ReferenceAmount: "originalAmount",
			Priority:        1,
			CreditAccount:   "credit_account",
			CalculationModel: &model.CalculationModel{
				ApplicationRule: "flatFee",
				Calculations: []model.Calculation{
					{Type: "flat", Value: "10"},
				},
			},
		},
	}

	p := ReconstructPackage(
		id,
		"Reconstructed Package",
		&desc,
		&segmentID,
		ledgerID,
		&route,
		decimal.NewFromInt(50),
		decimal.NewFromInt(500),
		&waivedAccounts,
		fees,
		&enable,
		now,
		now,
		&deletedAt,
	)

	assert.NotNil(t, p)
	assert.Equal(t, id, p.ID)
	assert.Equal(t, "Reconstructed Package", p.FeeGroupLabel)
	assert.Equal(t, &desc, p.Description)
	assert.Equal(t, &segmentID, p.SegmentID)
	assert.Equal(t, ledgerID, p.LedgerID)
	assert.Equal(t, &route, p.TransactionRoute)
	assert.Equal(t, decimal.NewFromInt(50), p.MinimumAmount)
	assert.Equal(t, decimal.NewFromInt(500), p.MaximumAmount)
	assert.Equal(t, &waivedAccounts, p.WaivedAccounts)
	assert.Equal(t, fees, p.Fees)
	assert.Equal(t, &enable, p.Enable)
	assert.Equal(t, now, p.CreatedAt)
	assert.Equal(t, now, p.UpdatedAt)
	assert.Equal(t, &deletedAt, p.DeletedAt)
}

func TestReconstructPackage_WithNilOptionalFields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	id := uuid.New()
	ledgerID := uuid.New()

	p := ReconstructPackage(
		id,
		"Minimal Package",
		nil,
		nil,
		ledgerID,
		nil,
		decimal.Zero,
		decimal.Zero,
		nil,
		nil,
		nil,
		now,
		now,
		nil,
	)

	assert.NotNil(t, p)
	assert.Equal(t, id, p.ID)
	assert.Equal(t, "Minimal Package", p.FeeGroupLabel)
	assert.Nil(t, p.Description)
	assert.Nil(t, p.SegmentID)
	assert.Nil(t, p.TransactionRoute)
	assert.Nil(t, p.WaivedAccounts)
	assert.Nil(t, p.Fees)
	assert.Nil(t, p.Enable)
	assert.Nil(t, p.DeletedAt)
}

func TestFromEntityFeeMap_InvalidCalculationValue(t *testing.T) {
	t.Parallel()

	feeKey := "feeKey1"
	fee := model.Fee{
		FeeLabel:        "Test Fee",
		ReferenceAmount: "originalAmount",
		Priority:        1,
		CreditAccount:   "credit_account",
		CalculationModel: &model.CalculationModel{
			ApplicationRule: "flatFee",
			Calculations: []model.Calculation{
				{
					Type:  "percentage",
					Value: "not-a-number",
				},
			},
		},
	}

	fees := map[string]model.Fee{
		feeKey: fee,
	}

	result, err := FromEntityFeeMap(fees)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "fee")
	assert.Contains(t, err.Error(), "invalid decimal value")
}

func TestPackageMongoDBModel_FromEntity_InvalidFees(t *testing.T) {
	t.Parallel()

	now := time.Now()
	orgID := uuid.New()
	ledgerID := uuid.New()
	packageID := uuid.New()

	p := &Package{
		ID:            packageID,
		FeeGroupLabel: "Test Package",
		LedgerID:      ledgerID,
		MinimumAmount: decimal.NewFromInt(100),
		MaximumAmount: decimal.NewFromInt(1000),
		Fees: map[string]model.Fee{
			"badFee": {
				FeeLabel:        "Bad Fee",
				ReferenceAmount: "originalAmount",
				Priority:        1,
				CreditAccount:   "credit_account",
				CalculationModel: &model.CalculationModel{
					ApplicationRule: "flatFee",
					Calculations: []model.Calculation{
						{
							Type:  "flat",
							Value: "invalid-decimal",
						},
					},
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	pmm := &PackageMongoDBModel{}
	err := pmm.FromEntity(p, orgID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to convert fees")
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}
