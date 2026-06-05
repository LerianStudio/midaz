// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string { return &s }

func decPtr(d decimal.Decimal) *decimal.Decimal { return &d }

func TestBuildVolumePayload(t *testing.T) {
	t.Parallel()

	basePkg := model.BillingPackage{
		ID:                 "bp-vol-001",
		OrganizationID:     "org-001",
		LedgerID:           "led-001",
		Label:              "API Calls",
		Type:               model.BillingPackageTypeVolume,
		AssetCode:          strPtr("BRL"),
		DebitAccountAlias:  strPtr("@billing-debit"),
		CreditAccountAlias: strPtr("@billing-credit"),
		PricingModel:       strPtr(model.PricingModelTiered),
		FreeQuota:          intPtr(100),
	}

	tests := []struct {
		name           string
		pkg            model.BillingPackage
		period         string
		totalEvents    int64
		netAmount      decimal.Decimal
		discount       *model.DiscountDetail
		wantCode       string
		wantDescPrefix string
		wantFromCount  int
		wantToCount    int
		wantAsset      string
		wantSendValue  decimal.Decimal
		checkDiscount  bool
	}{
		{
			name:           "1:1 transaction structure with discount",
			pkg:            basePkg,
			period:         "2026-01",
			totalEvents:    500,
			netAmount:      decimal.NewFromInt(850),
			discount:       &model.DiscountDetail{DiscountPercentage: decimal.NewFromInt(15), DiscountAmount: decimal.NewFromInt(150), MinQuantity: 200},
			wantCode:       "billing-volume",
			wantDescPrefix: "Billing - API Calls - 2026-01",
			wantFromCount:  1,
			wantToCount:    1,
			wantAsset:      "BRL",
			wantSendValue:  decimal.NewFromInt(850),
			checkDiscount:  true,
		},
		{
			name:           "1:1 transaction without discount",
			pkg:            basePkg,
			period:         "2026-02",
			totalEvents:    50,
			netAmount:      decimal.NewFromInt(500),
			discount:       nil,
			wantCode:       "billing-volume",
			wantDescPrefix: "Billing - API Calls - 2026-02",
			wantFromCount:  1,
			wantToCount:    1,
			wantAsset:      "BRL",
			wantSendValue:  decimal.NewFromInt(500),
			checkDiscount:  false,
		},
		{
			name:           "zero net amount produces valid payload",
			pkg:            basePkg,
			period:         "2026-03",
			totalEvents:    100,
			netAmount:      decimal.Zero,
			discount:       nil,
			wantCode:       "billing-volume",
			wantDescPrefix: "Billing - API Calls - 2026-03",
			wantFromCount:  1,
			wantToCount:    1,
			wantAsset:      "BRL",
			wantSendValue:  decimal.Zero,
			checkDiscount:  false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			result := BuildVolumePayload(ctx, tt.pkg, tt.period, tt.totalEvents, tt.netAmount, tt.discount)

			require.NotNil(t, result, "result must not be nil")

			// Verify transaction code and description.
			assert.Equal(t, tt.wantCode, result.Code)
			assert.Equal(t, tt.wantDescPrefix, result.Description)

			// Verify send fields.
			assert.Equal(t, tt.wantAsset, result.Send.Asset)
			assert.True(t, tt.wantSendValue.Equal(result.Send.Value), "send.value mismatch: want=%s, got=%s", tt.wantSendValue, result.Send.Value)

			// Verify 1:1 structure: exactly 1 from and 1 to.
			require.Len(t, result.Send.Source.From, tt.wantFromCount, "expected exactly %d source.from entry", tt.wantFromCount)
			require.Len(t, result.Send.Distribute.To, tt.wantToCount, "expected exactly %d distribute.to entry", tt.wantToCount)

			// Verify from entry.
			fromEntry := result.Send.Source.From[0]
			assert.Equal(t, "@billing-debit", fromEntry.AccountAlias)
			require.NotNil(t, fromEntry.Amount, "from entry amount must not be nil")
			assert.Equal(t, tt.wantAsset, fromEntry.Amount.Asset)
			assert.True(t, tt.wantSendValue.Equal(fromEntry.Amount.Value), "from amount value mismatch")

			// Verify to entry.
			toEntry := result.Send.Distribute.To[0]
			assert.Equal(t, "@billing-credit", toEntry.AccountAlias)
			require.NotNil(t, toEntry.Amount, "to entry amount must not be nil")
			assert.Equal(t, tt.wantAsset, toEntry.Amount.Asset)
			assert.True(t, tt.wantSendValue.Equal(toEntry.Amount.Value), "to amount value mismatch")

			// Verify send.value == sum(from amounts).
			fromSum := decimal.Zero
			for _, f := range result.Send.Source.From {
				if f.Amount != nil {
					fromSum = fromSum.Add(f.Amount.Value)
				}
			}
			assert.True(t, result.Send.Value.Equal(fromSum), "send.value must equal sum of from amounts")

			// Verify metadata.
			require.NotNil(t, result.Metadata, "metadata must not be nil")
			assert.Equal(t, "volume", result.Metadata["billingType"])
			assert.Equal(t, "bp-vol-001", result.Metadata["billingPackageId"])
			assert.Equal(t, "API Calls", result.Metadata["label"])
			assert.Equal(t, tt.period, result.Metadata["period"])
			assert.Equal(t, "tiered", result.Metadata["pricingModel"])

			if tt.checkDiscount {
				assert.NotNil(t, result.Metadata["discount"], "discount metadata must be present")
			} else {
				assert.Nil(t, result.Metadata["discount"], "discount metadata must be nil when no discount")
			}
		})
	}
}

func TestBuildMaintenancePayload(t *testing.T) {
	t.Parallel()

	feeAmount := decimal.NewFromFloat(9.99)
	basePkg := model.BillingPackage{
		ID:                       "bp-maint-001",
		OrganizationID:           "org-001",
		LedgerID:                 "led-001",
		Label:                    "Account Maintenance",
		Type:                     model.BillingPackageTypeMaintenance,
		AssetCode:                strPtr("BRL"),
		FeeAmount:                decPtr(feeAmount),
		MaintenanceCreditAccount: strPtr("@maint-credit"),
	}

	tests := []struct {
		name          string
		pkg           model.BillingPackage
		period        string
		accounts      []pkg.Account
		wantCode      string
		wantFromCount int
		wantToCount   int
	}{
		{
			name:   "N:1 structure with 3 accounts",
			pkg:    basePkg,
			period: "2026-01",
			accounts: []pkg.Account{
				{ID: "acc-1", Alias: "@account-1"},
				{ID: "acc-2", Alias: "@account-2"},
				{ID: "acc-3", Alias: "@account-3"},
			},
			wantCode:      "billing-maintenance",
			wantFromCount: 3,
			wantToCount:   1,
		},
		{
			name:   "single account produces 1:1 structure",
			pkg:    basePkg,
			period: "2026-02",
			accounts: []pkg.Account{
				{ID: "acc-1", Alias: "@single-account"},
			},
			wantCode:      "billing-maintenance",
			wantFromCount: 1,
			wantToCount:   1,
		},
		{
			name:   "N:1 structure with 5 accounts",
			pkg:    basePkg,
			period: "2026-03",
			accounts: []pkg.Account{
				{ID: "acc-1", Alias: "@a1"},
				{ID: "acc-2", Alias: "@a2"},
				{ID: "acc-3", Alias: "@a3"},
				{ID: "acc-4", Alias: "@a4"},
				{ID: "acc-5", Alias: "@a5"},
			},
			wantCode:      "billing-maintenance",
			wantFromCount: 5,
			wantToCount:   1,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			result := BuildMaintenancePayload(ctx, tt.pkg, tt.period, tt.accounts)

			require.NotNil(t, result, "result must not be nil")

			// Verify code and description.
			assert.Equal(t, tt.wantCode, result.Code)
			expectedDesc := fmt.Sprintf("Billing - %s - %s", tt.pkg.Label, tt.period)
			assert.Equal(t, expectedDesc, result.Description)

			// Verify N:1 structure.
			require.Len(t, result.Send.Source.From, tt.wantFromCount, "expected %d from entries", tt.wantFromCount)
			require.Len(t, result.Send.Distribute.To, tt.wantToCount, "expected exactly 1 to entry")

			// Verify send.value == len(accounts) * feeAmount.
			expectedTotal := feeAmount.Mul(decimal.NewFromInt(int64(len(tt.accounts))))
			assert.True(t, expectedTotal.Equal(result.Send.Value), "send.value mismatch: want=%s, got=%s", expectedTotal, result.Send.Value)

			// Verify send.asset.
			assert.Equal(t, "BRL", result.Send.Asset)

			// Verify each from entry debits exactly feeAmount.
			for i, fromEntry := range result.Send.Source.From {
				assert.Equal(t, tt.accounts[i].Alias, fromEntry.AccountAlias, "from[%d] alias mismatch", i)
				require.NotNil(t, fromEntry.Amount, "from[%d] amount must not be nil", i)
				assert.True(t, feeAmount.Equal(fromEntry.Amount.Value), "from[%d] amount mismatch: want=%s, got=%s", i, feeAmount, fromEntry.Amount.Value)
				assert.Equal(t, "BRL", fromEntry.Amount.Asset)
			}

			// Verify to entry.
			toEntry := result.Send.Distribute.To[0]
			assert.Equal(t, "@maint-credit", toEntry.AccountAlias)
			require.NotNil(t, toEntry.Amount, "to entry amount must not be nil")
			assert.True(t, expectedTotal.Equal(toEntry.Amount.Value), "to amount mismatch")

			// Verify send.value == sum(from amounts).
			fromSum := decimal.Zero
			for _, f := range result.Send.Source.From {
				if f.Amount != nil {
					fromSum = fromSum.Add(f.Amount.Value)
				}
			}
			assert.True(t, result.Send.Value.Equal(fromSum), "send.value must equal sum of from amounts")

			// Verify metadata.
			require.NotNil(t, result.Metadata, "metadata must not be nil")
			assert.Equal(t, "maintenance", result.Metadata["billingType"])
			assert.Equal(t, "bp-maint-001", result.Metadata["billingPackageId"])
			assert.Equal(t, "Account Maintenance", result.Metadata["label"])
			assert.Equal(t, tt.period, result.Metadata["period"])
			assert.Equal(t, len(tt.accounts), result.Metadata["totalAccounts"])
			assert.Equal(t, feeAmount.String(), result.Metadata["feeAmount"])
		})
	}
}

func TestBuildVolumePayload_FreeQuotaUsedMetadata(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		freeQuota         int
		totalEvents       int64
		wantFreeQuotaUsed int64
		wantBillable      int64
	}{
		{
			name:              "totalEvents > freeQuota — freeQuotaUsed equals freeQuota",
			freeQuota:         10,
			totalEvents:       50,
			wantFreeQuotaUsed: 10,
			wantBillable:      40,
		},
		{
			name:              "totalEvents == freeQuota — freeQuotaUsed equals freeQuota",
			freeQuota:         10,
			totalEvents:       10,
			wantFreeQuotaUsed: 10,
			wantBillable:      0,
		},
		{
			name:              "totalEvents < freeQuota — freeQuotaUsed equals totalEvents",
			freeQuota:         10,
			totalEvents:       5,
			wantFreeQuotaUsed: 5,
			wantBillable:      0,
		},
		{
			name:              "totalEvents == 0 — freeQuotaUsed is 0",
			freeQuota:         10,
			totalEvents:       0,
			wantFreeQuotaUsed: 0,
			wantBillable:      0,
		},
		{
			name:              "no freeQuota configured — freeQuotaUsed is 0",
			freeQuota:         0,
			totalEvents:       50,
			wantFreeQuotaUsed: 0,
			wantBillable:      50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fq := tt.freeQuota
			bp := model.BillingPackage{
				ID:                 "bp-fq-test",
				Label:              "FQ Test",
				Type:               model.BillingPackageTypeVolume,
				AssetCode:          strPtr("BRL"),
				DebitAccountAlias:  strPtr("@debit"),
				CreditAccountAlias: strPtr("@credit"),
				PricingModel:       strPtr(model.PricingModelTiered),
				FreeQuota:          &fq,
			}

			ctx := context.Background()
			result := BuildVolumePayload(ctx, bp, "2026-01", tt.totalEvents, decimal.Zero, nil)

			assert.Equal(t, tt.wantFreeQuotaUsed, result.Metadata["freeQuotaUsed"])
			assert.Equal(t, tt.wantBillable, result.Metadata["billableEvents"])
		})
	}
}

func intPtr(i int) *int { return &i }
