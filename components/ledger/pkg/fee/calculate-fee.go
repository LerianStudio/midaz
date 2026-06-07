// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package fee provides utilities for calculating transaction fees based on various rules and package configurations.
package fee

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	libLog "github.com/LerianStudio/lib-observability/log"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// SegmentContext holds optional dependencies for segment-based waivedAccounts resolution.
// When nil or when Resolver is nil, CalculateFee falls back to exact alias matching only.
type SegmentContext struct {
	Ctx            context.Context
	Resolver       feeshared.MidazResolver
	OrganizationID uuid.UUID
	LedgerID       uuid.UUID
}

// DefaultCurrencyBRL is the fallback currency when no default is configured.
const DefaultCurrencyBRL = "BRL"

// segmentPrefix is the prefix used to identify segment references in waivedAccounts entries.
const segmentPrefix = "segment:"

// isSegmentReference checks if a waivedAccounts entry is a segment reference.
// Returns (true, parsed UUID, nil) if the entry starts with "segment:" and has a valid UUID.
// Returns (false, uuid.Nil, nil) if the entry is a regular account alias (no segment prefix).
// Returns (true, uuid.Nil, error) if the entry has the segment prefix but an invalid UUID,
// indicating a configuration error that should not be silently ignored.
func isSegmentReference(entry string) (bool, uuid.UUID, error) {
	if !strings.HasPrefix(entry, segmentPrefix) {
		return false, uuid.Nil, nil
	}

	raw := strings.TrimPrefix(entry, segmentPrefix)

	id, err := uuid.Parse(raw)
	if err != nil {
		return true, uuid.Nil, fmt.Errorf("malformed segment waiver %q: invalid UUID %q: %w", entry, raw, err)
	}

	return true, id, nil
}

// CalculateFee calculates and applies all fees for a transaction package.
// It mutates f.Transaction.Send.Value and updates resp with fee results.
//
// Fee legs are ALWAYS denominated in the transaction's Send.Asset (P4-T24): the
// ledger validator aggregates per-asset and requires sum == 0 under exact
// decimal.Equal, so a fee leg in any asset other than Send.Asset would either
// trip ErrTransactionValueMismatch or silently create a multi-asset imbalance.
// The defaultCurrency parameter is accepted for the value-only fallback when the
// transaction carries no Send.Asset; it NEVER denominates a leg in a different
// asset than the transaction. Send.Asset is the single source of truth.
//
// The segCtx parameter is optional: when non-nil, segment-based waivedAccounts resolution
// is enabled (entries like "segment:<uuid>" trigger a Midaz API call to check account membership).
// When segCtx is nil, only exact alias matching is used for waivedAccounts.
func CalculateFee(logger libLog.Logger, f *model.FeeCalculate, p *pack.Package, resp *transaction.Responses, defaultCurrency string, segCtx *SegmentContext) error {
	if defaultCurrency == "" {
		defaultCurrency = DefaultCurrencyBRL
	}

	// Fee legs are denominated in the transaction's Send.Asset. The configured
	// default currency only acts as a fallback when the transaction omits one.
	feeAsset := f.Transaction.Send.Asset
	if feeAsset == "" {
		feeAsset = defaultCurrency
	}

	originalTransactionValue := f.Transaction.Send.Value

	fees := make([]model.Fee, 0, len(p.Fees))
	for _, fee := range p.Fees {
		fees = append(fees, fee)
	}

	sort.Slice(fees, func(i, j int) bool {
		return fees[i].Priority < fees[j].Priority
	})

	// Create a local copy of WaivedAccounts to avoid mutating the cached package.
	// This prevents state accumulation across multiple API calls when the package is cached.
	var waivedAccounts []string
	if p.WaivedAccounts != nil {
		waivedAccounts = make([]string, len(*p.WaivedAccounts))
		copy(waivedAccounts, *p.WaivedAccounts)
	} else {
		waivedAccounts = make([]string, 0)
	}

	// Resolve segment-based waivedAccounts: split into direct aliases and segment UUIDs.
	directAliases, segmentIDs, resolveErr := resolveSegmentWaivedAccounts(waivedAccounts)
	if resolveErr != nil {
		return resolveErr
	}

	directAliasesPtr := &directAliases

	for feeIndex, fee := range fees {
		valueToCalculate := selectReferenceAmount(fee, f.Transaction.Send.Value, originalTransactionValue)

		var result transaction.Amount

		var err error

		switch fee.CalculationModel.ApplicationRule {
		case feeconstant.AppRuleMaxBetweenTypes:
			result, err = calculateMaxBetweenTypesFee(fee, valueToCalculate, feeAsset)
		case feeconstant.AppRuleFlatFee:
			result, err = calculateFlatFee(fee, feeAsset)
		case feeconstant.AppRulePercentual:
			result, err = calculatePercentualFee(fee, valueToCalculate, feeAsset)
		default:
			return pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("unknown application rule: %s", fee.CalculationModel.ApplicationRule))
		}

		if err != nil {
			return err
		}

		// Fee total is emitted unrounded: the ledger is arbitrary-precision and
		// every serialization seam round-trips full precision (P4-T23). The
		// residual-to-max reconciliation in applyFeeCorrection holds sum(legs) ==
		// fee total exactly without any asset-scale rounding.

		if err := applyDeductibleAndReferenceAmountRules(logger, feeIndex, directAliasesPtr, segmentIDs, segCtx, fee, resp, result, f); err != nil {
			return err
		}
	}

	f.Transaction.Send.Source.From = updatedAmountsFromFee(resp.From)
	f.Transaction.Send.Distribute.To = updatedAmountsFromFee(resp.To)

	return nil
}

// selectReferenceAmount chooses the correct transaction value for fee calculation based on the fee's reference amount rule.
func selectReferenceAmount(fee model.Fee, currentValue, originalValue decimal.Decimal) decimal.Decimal {
	if fee.ReferenceAmount == feeconstant.ReferenceAmountAfterFeesAmount {
		return currentValue
	}

	return originalValue
}

// calculateMaxBetweenTypesFee calculates the maximum fee between flat and percentual types.
func calculateMaxBetweenTypesFee(fee model.Fee, valueToCalculate decimal.Decimal, feeAsset string) (transaction.Amount, error) {
	var maxValue decimal.Decimal

	var maxAmount transaction.Amount

	for _, calc := range fee.CalculationModel.Calculations {
		var realValue decimal.Decimal

		var err error

		var resultAmount transaction.Amount

		switch calc.Type {
		case feeconstant.FeeTypeFlat:
			realValue, err = decimal.NewFromString(calc.Value)
			if err != nil {
				return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("invalid flat fee value: %v", err))
			}

			resultAmount = transaction.Amount{
				Asset: feeAsset,
				Value: realValue,
			}
		case feeconstant.FeeTypePercentage:
			percentValue, err := decimal.NewFromString(calc.Value)
			if err != nil {
				return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("invalid percentage fee value: %v", err))
			}

			resultAmount = findPercentualOfValue(percentValue, valueToCalculate, feeAsset)
			realValue = resultAmount.Value
		default:
			return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("unknown fee type: %v", err))
		}

		if realValue.GreaterThan(maxValue) {
			maxValue = realValue
			maxAmount = resultAmount
		}
	}

	return maxAmount, nil
}

// calculateFlatFee calculates a flat fee amount.
func calculateFlatFee(fee model.Fee, feeAsset string) (transaction.Amount, error) {
	if len(fee.CalculationModel.Calculations) == 0 {
		return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrCalculationRequired, "", "flatFee requires at least one calculation")
	}

	calc := fee.CalculationModel.Calculations[0]

	value, err := decimal.NewFromString(calc.Value)
	if err != nil {
		return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("invalid flat fee value: %v", err))
	}

	return transaction.Amount{
		Asset: feeAsset,
		Value: value,
	}, nil
}

// calculatePercentualFee calculates a percentual fee amount.
func calculatePercentualFee(fee model.Fee, valueToCalculate decimal.Decimal, feeAsset string) (transaction.Amount, error) {
	if len(fee.CalculationModel.Calculations) == 0 {
		return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrCalculationRequired, "", "percentual requires at least one calculation")
	}

	calc := fee.CalculationModel.Calculations[0]

	percentValue, err := decimal.NewFromString(calc.Value)
	if err != nil {
		return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("invalid percentage fee value: %v", err))
	}

	return findPercentualOfValue(percentValue, valueToCalculate, feeAsset), nil
}

// findPercentualOfValue finds the percentual of value
func findPercentualOfValue(feeValue, transactionValue decimal.Decimal, feeAsset string) transaction.Amount {
	percentConverted := feeValue.Div(decimal.NewFromInt(100))

	return transaction.Amount{
		Asset: feeAsset,
		Value: transactionValue.Mul(percentConverted),
	}
}
