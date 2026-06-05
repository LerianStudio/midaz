// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/shopspring/decimal"
)

// BuildVolumePayload assembles a 1:1 Midaz Transaction for a volume billing package.
// The source has a single debit entry and the distribution has a single credit entry,
// both carrying the net amount after any applicable discount.
func BuildVolumePayload(
	ctx context.Context,
	bp model.BillingPackage,
	period string,
	totalEvents int64,
	netAmount decimal.Decimal,
	discount *model.DiscountDetail,
) *transaction.Transaction {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "service.payload_builder.build_volume_payload")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Building volume payload: packageId=%s, period=%s, totalEvents=%d, netAmount=%s",
		bp.ID, period, totalEvents, netAmount.String()))

	asset := ""
	if bp.AssetCode != nil {
		asset = *bp.AssetCode
	}

	debitAlias := ""
	if bp.DebitAccountAlias != nil {
		debitAlias = *bp.DebitAccountAlias
	}

	creditAlias := ""
	if bp.CreditAccountAlias != nil {
		creditAlias = *bp.CreditAccountAlias
	}

	pricingModel := ""
	if bp.PricingModel != nil {
		pricingModel = *bp.PricingModel
	}

	freeQuota := 0
	if bp.FreeQuota != nil {
		freeQuota = *bp.FreeQuota
	}

	freeQuotaUsed := min(int64(freeQuota), totalEvents)

	// Compute gross amount (net + discount if present).
	grossAmount := netAmount
	if discount != nil {
		grossAmount = netAmount.Add(discount.DiscountAmount)
	}

	metadata := map[string]any{
		"billingType":      model.BillingPackageTypeVolume,
		"billingPackageId": bp.ID,
		"label":            bp.Label,
		"period":           period,
		"totalEvents":      totalEvents,
		"freeQuotaUsed":    freeQuotaUsed,
		"billableEvents":   max(totalEvents-freeQuotaUsed, 0),
		"pricingModel":     pricingModel,
		"grossAmount":      grossAmount.String(),
		"netAmount":        netAmount.String(),
		"discount":         nil,
	}

	if discount != nil {
		metadata["discount"] = map[string]any{
			"discountPercentage": discount.DiscountPercentage.String(),
			"discountAmount":     discount.DiscountAmount.String(),
			"minQuantity":        discount.MinQuantity,
		}
	}

	fromEntry := transaction.FromTo{
		AccountAlias: debitAlias,
		Amount: &transaction.Amount{
			Asset: asset,
			Value: netAmount,
		},
	}

	toEntry := transaction.FromTo{
		AccountAlias: creditAlias,
		Amount: &transaction.Amount{
			Asset: asset,
			Value: netAmount,
		},
	}

	tx := &transaction.Transaction{
		Code:        "billing-volume",
		Description: fmt.Sprintf("Billing - %s - %s", bp.Label, period),
		Metadata:    metadata,
		Send: transaction.Send{
			Asset: asset,
			Value: netAmount,
			Source: transaction.Source{
				From: []transaction.FromTo{fromEntry},
			},
			Distribute: transaction.Distribute{
				To: []transaction.FromTo{toEntry},
			},
		},
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Volume payload built successfully: packageId=%s, code=%s", bp.ID, tx.Code))

	return tx
}

// BuildMaintenancePayload assembles an N:1 Midaz Transaction for a maintenance billing package.
// Each active account becomes a source (debit) entry for the fixed fee amount, and all funds
// are distributed to a single maintenance credit account.
func BuildMaintenancePayload(
	ctx context.Context,
	bp model.BillingPackage,
	period string,
	accounts []pkg.Account,
) *transaction.Transaction {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "service.payload_builder.build_maintenance_payload")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Building maintenance payload: packageId=%s, period=%s, accounts=%d",
		bp.ID, period, len(accounts)))

	asset := ""
	if bp.AssetCode != nil {
		asset = *bp.AssetCode
	}

	feeAmount := decimal.Zero
	if bp.FeeAmount != nil {
		feeAmount = *bp.FeeAmount
	}

	creditAlias := ""
	if bp.MaintenanceCreditAccount != nil {
		creditAlias = *bp.MaintenanceCreditAccount
	}

	totalValue := feeAmount.Mul(decimal.NewFromInt(int64(len(accounts))))

	// Build from entries: one per account, each debiting feeAmount.
	fromEntries := make([]transaction.FromTo, 0, len(accounts))

	for _, acc := range accounts {
		entry := transaction.FromTo{
			AccountAlias: acc.Alias,
			Amount: &transaction.Amount{
				Asset: asset,
				Value: feeAmount,
			},
		}

		fromEntries = append(fromEntries, entry)
	}

	// Build single to entry: maintenance credit account receives totalValue.
	toEntry := transaction.FromTo{
		AccountAlias: creditAlias,
		Amount: &transaction.Amount{
			Asset: asset,
			Value: totalValue,
		},
	}

	metadata := map[string]any{
		"billingType":      model.BillingPackageTypeMaintenance,
		"billingPackageId": bp.ID,
		"label":            bp.Label,
		"period":           period,
		"totalAccounts":    len(accounts),
		"feeAmount":        feeAmount.String(),
	}

	tx := &transaction.Transaction{
		Code:        "billing-maintenance",
		Description: fmt.Sprintf("Billing - %s - %s", bp.Label, period),
		Metadata:    metadata,
		Send: transaction.Send{
			Asset: asset,
			Value: totalValue,
			Source: transaction.Source{
				From: fromEntries,
			},
			Distribute: transaction.Distribute{
				To: []transaction.FromTo{toEntry},
			},
		},
	}

	// Validate internal consistency: send.value == sum(from amounts).
	fromSum := decimal.Zero

	for _, f := range fromEntries {
		if f.Amount != nil {
			fromSum = fromSum.Add(f.Amount.Value)
		}
	}

	if !totalValue.Equal(fromSum) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Maintenance payload amount mismatch",
			fmt.Errorf("send.value=%s != sum(from)=%s", totalValue.String(), fromSum.String()))
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Maintenance payload amount mismatch: send.value=%s, sum(from)=%s",
			totalValue.String(), fromSum.String()))
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Maintenance payload built successfully: packageId=%s, code=%s, totalValue=%s",
		bp.ID, tx.Code, totalValue.String()))

	return tx
}
