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

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	pkghttp "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"

	libLog "github.com/LerianStudio/lib-observability/log"
	transaction "github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// SegmentContext holds optional dependencies for segment-based waivedAccounts resolution.
// When nil or when MidazClient is nil, CalculateFee falls back to exact alias matching only.
type SegmentContext struct {
	Ctx            context.Context
	MidazClient    pkghttp.MidazClient
	OrganizationID string
	LedgerID       string
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
// The defaultCurrency parameter specifies the currency code for fee amounts (e.g., "BRL").
// The segCtx parameter is optional: when non-nil, segment-based waivedAccounts resolution
// is enabled (entries like "segment:<uuid>" trigger a Midaz API call to check account membership).
// When segCtx is nil, only exact alias matching is used for waivedAccounts.
func CalculateFee(logger libLog.Logger, f *model.FeeCalculate, p *pack.Package, resp *transaction.Responses, defaultCurrency string, segCtx *SegmentContext) error {
	if defaultCurrency == "" {
		defaultCurrency = DefaultCurrencyBRL
	}

	logger.Log(context.TODO(), libLog.LevelInfo, "Trying to calculate a fee")

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
		logger.Log(context.TODO(), libLog.LevelError, fmt.Sprintf("Invalid waivedAccounts configuration: %v", resolveErr))
		return resolveErr
	}

	directAliasesPtr := &directAliases

	for feeIndex, fee := range fees {
		valueToCalculate := selectReferenceAmount(fee, f.Transaction.Send.Value, originalTransactionValue)

		var result transaction.Amount

		var err error

		switch fee.CalculationModel.ApplicationRule {
		case constant.AppRuleMaxBetweenTypes:
			logger.Log(context.TODO(), libLog.LevelInfo, fmt.Sprintf("Calculating fee with app rule maxBetweenTypes (feeIndex=%d)", feeIndex))

			result, err = calculateMaxBetweenTypesFee(fee, valueToCalculate, defaultCurrency)
		case constant.AppRuleFlatFee:
			logger.Log(context.TODO(), libLog.LevelInfo, fmt.Sprintf("Calculating fee with app rule flatFee (feeIndex=%d)", feeIndex))

			result, err = calculateFlatFee(fee, defaultCurrency)
		case constant.AppRulePercentual:
			logger.Log(context.TODO(), libLog.LevelInfo, fmt.Sprintf("Calculating fee with app rule percentual (feeIndex=%d)", feeIndex))

			result, err = calculatePercentualFee(fee, valueToCalculate, defaultCurrency)
		default:
			return pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("unknown application rule: %s", fee.CalculationModel.ApplicationRule))
		}

		if err != nil {
			return err
		}

		// Round fee based on asset precision (Half Up)
		precision := getAssetPrecision(f.Transaction.Send.Asset)
		result.Value = result.Value.Round(precision)

		if err := applyDeductibleAndReferenceAmountRules(logger, feeIndex, directAliasesPtr, segmentIDs, segCtx, fee, resp, result, f); err != nil {
			logger.Log(context.TODO(), libLog.LevelError, fmt.Sprintf("Fee distribution failed due to segment resolution error: %v", err))
			return err
		}
	}

	logger.Log(context.TODO(), libLog.LevelInfo, "Fee calculated successfully")

	f.Transaction.Send.Source.From = updatedAmountsFromFee(resp.From)
	f.Transaction.Send.Distribute.To = updatedAmountsFromFee(resp.To)

	return nil
}

// selectReferenceAmount chooses the correct transaction value for fee calculation based on the fee's reference amount rule.
func selectReferenceAmount(fee model.Fee, currentValue, originalValue decimal.Decimal) decimal.Decimal {
	if fee.ReferenceAmount == constant.ReferenceAmountAfterFeesAmount {
		return currentValue
	}

	return originalValue
}

// calculateMaxBetweenTypesFee calculates the maximum fee between flat and percentual types.
func calculateMaxBetweenTypesFee(fee model.Fee, valueToCalculate decimal.Decimal, defaultCurrency string) (transaction.Amount, error) {
	var maxValue decimal.Decimal

	var maxAmount transaction.Amount

	for _, calc := range fee.CalculationModel.Calculations {
		var realValue decimal.Decimal

		var err error

		var resultAmount transaction.Amount

		switch calc.Type {
		case constant.FeeTypeFlat:
			realValue, err = decimal.NewFromString(calc.Value)
			if err != nil {
				return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("invalid flat fee value: %v", err))
			}

			resultAmount = transaction.Amount{
				Asset: defaultCurrency,
				Value: realValue,
			}
		case constant.FeeTypePercentage:
			percentValue, err := decimal.NewFromString(calc.Value)
			if err != nil {
				return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("invalid percentage fee value: %v", err))
			}

			resultAmount = findPercentualOfValue(percentValue, valueToCalculate, defaultCurrency)
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
func calculateFlatFee(fee model.Fee, defaultCurrency string) (transaction.Amount, error) {
	if len(fee.CalculationModel.Calculations) == 0 {
		return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrCalculationRequired, "", "flatFee requires at least one calculation")
	}

	calc := fee.CalculationModel.Calculations[0]

	value, err := decimal.NewFromString(calc.Value)
	if err != nil {
		return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("invalid flat fee value: %v", err))
	}

	return transaction.Amount{
		Asset: defaultCurrency,
		Value: value,
	}, nil
}

// calculatePercentualFee calculates a percentual fee amount.
func calculatePercentualFee(fee model.Fee, valueToCalculate decimal.Decimal, defaultCurrency string) (transaction.Amount, error) {
	if len(fee.CalculationModel.Calculations) == 0 {
		return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrCalculationRequired, "", "percentual requires at least one calculation")
	}

	calc := fee.CalculationModel.Calculations[0]

	percentValue, err := decimal.NewFromString(calc.Value)
	if err != nil {
		return transaction.Amount{}, pkg.ValidateBusinessError(constant.ErrApplicationRule, "", fmt.Sprintf("invalid percentage fee value: %v", err))
	}

	return findPercentualOfValue(percentValue, valueToCalculate, defaultCurrency), nil
}

// findPercentualOfValue finds the percentual of value
func findPercentualOfValue(feeValue, transactionValue decimal.Decimal, defaultCurrency string) transaction.Amount {
	percentConverted := feeValue.Div(decimal.NewFromInt(100))

	return transaction.Amount{
		Asset: defaultCurrency,
		Value: transactionValue.Mul(percentConverted),
	}
}
