// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"strconv"
	"strings"

	feeconstant "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"

	libLog "github.com/LerianStudio/lib-observability/log"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// applyDeductibleAndReferenceAmountRules applies the deductible and reference amount rules for a fee.
// When segmentIDs is non-empty and segCtx is non-nil, segment-based exemption is used instead of exact alias matching.
//
// For deductible fees (isDeductibleFrom=true), source (From) accounts are checked for exemption
// before any deduction is applied to destination (To) accounts. If all source accounts are exempt,
// the deductible fee is skipped entirely — the transaction initiator's exemption status determines
// whether the fee is triggered.
func applyDeductibleAndReferenceAmountRules(_ libLog.Logger, feeIndex int, waivedAccounts *[]string, segmentIDs []uuid.UUID, segCtx *SegmentContext, feeModel model.Fee,
	resp *transaction.Responses, result transaction.Amount, f *model.FeeCalculate,
) error {
	originalRespToSize := len(resp.To)

	if !feeModel.GetIsDeductibleFrom() {
		// Check if ALL source (From) accounts are exempt before applying non-deductible fee.
		allFromExempt, err := allAccountsExempt(resp.From, waivedAccounts, segmentIDs, segCtx)
		if err != nil {
			return err
		}

		if allFromExempt {
			setFeeExemptionMetadata(f, "all_source_accounts_exempt")

			// Also check destination — if both sides are exempt, the combined reason is set.
			allToExempt, err := allAccountsExempt(resp.To, waivedAccounts, segmentIDs, segCtx)
			if err != nil {
				return err
			}

			if allToExempt {
				setFeeExemptionMetadata(f, "all_destination_accounts_exempt")
			}

			*waivedAccounts = append(*waivedAccounts, feeModel.CreditAccount)

			return nil
		}

		var errFee error

		resp.From, resp.To, errFee = applyProportionalFee(feeModel, feeIndex, &resp.From, resp.To, result, waivedAccounts, segmentIDs, segCtx, false)
		if errFee != nil {
			return errFee
		}

		if originalRespToSize != len(resp.To) {
			f.Transaction.Send.Value = f.Transaction.Send.Value.Add(result.Value)
		}
	} else {
		// Check if ALL source (From) accounts are exempt before applying deductible fee.
		allFromExempt, err := allAccountsExempt(resp.From, waivedAccounts, segmentIDs, segCtx)
		if err != nil {
			return err
		}

		if allFromExempt {
			setFeeExemptionMetadata(f, "all_source_accounts_exempt")

			// Also check destination — if both sides are exempt, the combined reason is set.
			allToExempt, err := allAccountsExempt(resp.To, waivedAccounts, segmentIDs, segCtx)
			if err != nil {
				return err
			}

			if allToExempt {
				setFeeExemptionMetadata(f, "all_destination_accounts_exempt")
			}

			*waivedAccounts = append(*waivedAccounts, feeModel.CreditAccount)

			return nil
		}

		var errFee error

		resp.To, _, errFee = applyProportionalFee(feeModel, feeIndex, &resp.To, nil, result, waivedAccounts, segmentIDs, segCtx, true)
		if errFee != nil {
			return errFee
		}

		// Log when all destination accounts were exempt — fee was not deducted from anyone.
		allToExempt, err := allAccountsExempt(resp.To, waivedAccounts, segmentIDs, segCtx)
		if err != nil {
			return err
		}

		if allToExempt {
			setFeeExemptionMetadata(f, "all_destination_accounts_exempt")
		}
	}

	*waivedAccounts = append(*waivedAccounts, feeModel.CreditAccount)

	return nil
}

// exemptionMessages maps reason codes to human-readable messages.
var exemptionMessages = map[string]string{
	"all_source_accounts_exempt":      "All source accounts are exempt from fees.",
	"all_destination_accounts_exempt": "All destination accounts are exempt from fees.",
	"all_accounts_exempt":             "All accounts (source and destination) are exempt from fees.",
}

// setFeeExemptionMetadata sets the feeExemption metadata on the transaction when all accounts
// on a given side (From or To) are exempt from fees. This allows API consumers to distinguish
// between "no package found" and "package found but accounts are exempt".
// When called multiple times (e.g., source exempt on fee1, destination exempt on fee2),
// the reason is combined to "all_accounts_exempt".
func setFeeExemptionMetadata(f *model.FeeCalculate, reason string) {
	if f.Transaction.Metadata == nil {
		f.Transaction.Metadata = make(map[string]any)
	}

	existing, hasExemption := f.Transaction.Metadata["feeExemption"]
	if hasExemption {
		exemptionMap, ok := existing.(map[string]any)
		if ok {
			existingReason, _ := exemptionMap["reason"].(string)
			if existingReason != reason && existingReason != "all_accounts_exempt" {
				reason = "all_accounts_exempt"
			} else {
				reason = existingReason
			}
		}
	}

	message := exemptionMessages[reason]

	f.Transaction.Metadata["feeExemption"] = map[string]any{
		"exempt":  true,
		"reason":  reason,
		"message": message,
	}
}

// updatedAmountsFromFee updates the amounts from the fee
func updatedAmountsFromFee(amounts map[string]transaction.Amount) []transaction.FromTo {
	newFromTo := make([]transaction.FromTo, 0, len(amounts))

	for account, amount := range amounts {
		parts := strings.Split(account, "->")
		cleanAccount := trimFeeSuffix(account)
		metadata := map[string]any{}

		var route string

		if strings.Contains(account, feeconstant.SuffixFeeSource) {
			cleanAccount, metadata = processAccount(account)
		}

		if len(parts) > 2 && parts[len(parts)-1] != "" {
			route = parts[len(parts)-1]
		}

		fromTo := transaction.FromTo{
			AccountAlias: cleanAccount,
			Amount:       &transaction.Amount{Asset: amount.Asset, Value: amount.Value},
		}
		if len(metadata) > 0 {
			fromTo.Metadata = metadata
		}

		if route != "" {
			fromTo.Route = route //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
		}

		newFromTo = append(newFromTo, fromTo)
	}

	return newFromTo
}

// trimFeeSuffix trims the fee suffix
func trimFeeSuffix(s string) string {
	if i := strings.Index(s, "->"); i != -1 {
		return s[:i]
	}

	return s
}

// processAccount processes the account
func processAccount(account string) (string, map[string]any) {
	parts := strings.Split(account, "->")
	metadata := make(map[string]any)

	if len(parts) >= 3 && strings.Contains(parts[1], "fee_source") {
		cleanAccount := parts[0]
		sourceAccount := parts[2]
		metadata["source"] = sourceAccount

		return cleanAccount, metadata
	}

	return account, metadata
}

// findMaxAccount Helper to find the account with the maximum value
func findMaxAccount(amounts map[string]transaction.Amount, exemptAccounts *[]string, segmentIDs []uuid.UUID, segCtx *SegmentContext) (string, error) {
	maxAmountValue := decimal.Zero
	maxAccount := ""

	for key, amount := range amounts {
		exempt, err := isAccountExemptOrSegment(key, exemptAccounts, segmentIDs, segCtx)
		if err != nil {
			return "", err
		}

		if !exempt {
			if amount.Value.GreaterThanOrEqual(maxAmountValue) {
				maxAmountValue = amount.Value
				maxAccount = key
			}
		}
	}

	return maxAccount, nil
}

// feeCorrectionTarget holds the leg keys captured at creation time for the max
// account; these keys are authoritative for addressing the legs the residual
// reconciliation must adjust, by direct map lookup (a route label may itself
// contain "fee"/"fee_source", so the key must be the one created, not one
// reconstructed by matching).
//
// Fields:
//   - debitLegKey: the fee leg that absorbs the residual.
//     Non-deductible: the "<acct>->feeN->routeFrom" debit leg in updateAmount.
//     Deductible:     the "<credit>->fee_sourceN->maxAccount->routeTo" leg in
//     updateAmount (the only leg the deductible path emits).
//   - creditLegKey: non-deductible only — the mirror "fee_source" leg in
//     updateAmountToStruct that moves in lockstep with the debit leg.
//   - payerKey: deductible only — the max account's reduced balance entry in
//     updateAmount; the residual is deducted from it symmetrically so
//     sum(deductions) stays equal to sum(fee_source legs) (debit==credit).
//   - found: whether the max account was processed (a non-exempt payer existed).
type feeCorrectionTarget struct {
	debitLegKey  string
	creditLegKey string
	payerKey     string
	found        bool
}

// calculateProportionalFees Helper to calculate proportional fees for each account
func calculateProportionalFees(
	feeModel model.Fee,
	feeIndex int,
	amounts *map[string]transaction.Amount,
	amountsToStruct map[string]transaction.Amount,
	feeValue transaction.Amount,
	exemptAccounts *[]string,
	segmentIDs []uuid.UUID,
	segCtx *SegmentContext,
	isToStruct bool,
	maxAccount string,
	target *feeCorrectionTarget,
) (map[string]transaction.Amount, map[string]transaction.Amount, decimal.Decimal, error) {
	updateAmount := make(map[string]transaction.Amount)
	updateAmountToStruct := amountsToStruct
	totalPaying := decimal.Zero
	newFeeTotalPaying := decimal.Zero

	for key, amount := range *amounts {
		exempt, err := isAccountExemptOrSegment(key, exemptAccounts, segmentIDs, segCtx)
		if err != nil {
			return *amounts, updateAmountToStruct, newFeeTotalPaying, err
		}

		if !exempt {
			totalPaying = totalPaying.Add(amount.Value)
		}
	}

	if totalPaying.IsZero() {
		return *amounts, updateAmountToStruct, newFeeTotalPaying, nil
	}

	for key, amount := range *amounts {
		exempt, err := isAccountExemptOrSegment(key, exemptAccounts, segmentIDs, segCtx)
		if err != nil {
			return *amounts, updateAmountToStruct, newFeeTotalPaying, err
		}

		if !exempt {
			proportionalFeePercent := amount.Value.Div(totalPaying)
			feeApplied := feeValue.Value.Mul(proportionalFeePercent)

			// Legs are emitted unrounded: the ledger is arbitrary-precision decimal
			// and every serialization seam (JSONB body, msgpack queue, Mongo
			// metadata) round-trips full precision (P4-T23), so there is no scale
			// to round to. Div caps proportionalFeePercent at decimal's
			// DivisionPrecision, leaving a sub-precision residual that
			// applyFeeCorrection reconciles onto the max account's leg — that
			// reconciliation, not per-leg rounding, is what holds sum(legs) == fee
			// total exactly.

			// Denominate the fee leg in feeValue.Asset (the transaction's
			// Send.Asset, set by CalculateFee), NOT the payer account's asset
			// (P4-T24). This guarantees every emitted fee leg shares the
			// transaction asset, so the ledger validator's per-asset aggregation
			// balances under exact decimal.Equal — no leg can escape into the
			// global default currency or a divergent payer asset.
			resultAmount := transaction.Amount{
				Asset: feeValue.Asset,
				Value: feeApplied,
			}

			if exemptAccounts == nil {
				newExemptAccounts := make([]string, 0)
				exemptAccounts = &newExemptAccounts
			}

			if isToStruct {
				amount = emitDeductibleLeg(feeModel, feeIndex, key, amount, resultAmount, exemptAccounts, updateAmount, maxAccount, target)
			} else {
				updateAmountToStruct = emitNonDeductibleLeg(feeModel, feeIndex, key, resultAmount, exemptAccounts, updateAmount, updateAmountToStruct, maxAccount, target)
			}

			newFeeTotalPaying = newFeeTotalPaying.Add(feeApplied)
		}

		updateAmount[key] = amount
	}

	return updateAmount, updateAmountToStruct, newFeeTotalPaying, nil
}

// emitDeductibleLeg writes the single fee_source leg for a deductible fee into
// updateAmount, reduces the paying account by the leg amount, and (when this is
// the max account) records the leg/payer keys on target for residual
// reconciliation. Returns the reduced paying-account amount.
func emitDeductibleLeg(
	feeModel model.Fee,
	feeIndex int,
	key string,
	amount, resultAmount transaction.Amount,
	exemptAccounts *[]string,
	updateAmount map[string]transaction.Amount,
	maxAccount string,
	target *feeCorrectionTarget,
) transaction.Amount {
	legKey := feeModel.CreditAccount + "->fee_source" + strconv.Itoa(feeIndex) + "->" + key + "->" + feeModel.GetRouteTo()
	updateAmount[legKey] = resultAmount
	amount.Value = amount.Value.Sub(resultAmount.Value)

	*exemptAccounts = append(*exemptAccounts, legKey)

	if target != nil && key == maxAccount {
		target.debitLegKey = legKey
		target.payerKey = key
		target.found = true
	}

	return amount
}

// emitNonDeductibleLeg writes the debit leg (in updateAmount) and its credit
// fee_source mirror (in updateAmountToStruct) for a non-deductible fee, and
// (when this is the max account) records both keys on target. Returns the
// possibly-allocated updateAmountToStruct.
func emitNonDeductibleLeg(
	feeModel model.Fee,
	feeIndex int,
	key string,
	resultAmount transaction.Amount,
	exemptAccounts *[]string,
	updateAmount, updateAmountToStruct map[string]transaction.Amount,
	maxAccount string,
	target *feeCorrectionTarget,
) map[string]transaction.Amount {
	feeKey := key + "->fee" + strconv.Itoa(feeIndex)
	debitLegKey := feeKey + "->" + feeModel.GetRouteFrom()
	feeSourceKey := feeModel.CreditAccount + "->fee_source" + strconv.Itoa(feeIndex) + "->" + key + "->" + feeModel.GetRouteTo()

	updateAmount[debitLegKey] = resultAmount

	if updateAmountToStruct == nil {
		updateAmountToStruct = make(map[string]transaction.Amount)
	}

	updateAmountToStruct[feeSourceKey] = resultAmount

	*exemptAccounts = append(*exemptAccounts, feeSourceKey)
	*exemptAccounts = append(*exemptAccounts, debitLegKey)

	if target != nil && key == maxAccount {
		target.debitLegKey = debitLegKey
		target.creditLegKey = feeSourceKey
		target.found = true
	}

	return updateAmountToStruct
}

// applyFeeCorrection reconciles the division residual onto the max account's
// fee leg so that the distributed legs sum EXACTLY to the fee total
// (double-entry conservation, full decimal precision, zero tolerance).
//
// Each per-account leg is feeValue * (amount/totalPaying); decimal division
// caps the proportion at DivisionPrecision, so the distributed sum
// (newFeeTotalPaying) can fall short of (or exceed) the fee total by a
// sub-precision residual. The residual delta = feeValue - newFeeTotalPaying is
// computed at full precision and applied to the max account's leg, making
// sum(legs) == fee total exactly under decimal.Equal — no asset scale and no
// precision table are involved, which is why deleting the ISO-4217 table cannot
// break the balance invariant.
//
// The leg to correct is addressed by the keys captured at creation time
// (feeCorrectionTarget), so a route label that contains "fee"/"fee_source"
// cannot misdirect the residual.
//
// Non-deductible: the residual moves the debit leg (updateAmount) and its credit
// mirror (updateAmountToStruct) in lockstep — both represent the same fee amount
// on opposite sides, so both must change by delta to stay balanced.
//
// Deductible: only the single fee_source leg exists (in updateAmount), and the
// payer's balance was reduced by the leg amount at distribution time. The residual
// is added to that leg AND subtracted again from the max account's reduced balance,
// keeping sum(fee_source legs) == sum(deductions) (internal debit==credit) while
// raising both to the full fee total.
func applyFeeCorrection(
	updateAmount map[string]transaction.Amount,
	updateAmountToStruct map[string]transaction.Amount,
	feeValue transaction.Amount,
	newFeeTotalPaying decimal.Decimal,
	isToStruct bool,
	target feeCorrectionTarget,
) {
	if !target.found {
		return
	}

	delta := feeValue.Value.Sub(newFeeTotalPaying)

	if delta.IsZero() {
		return
	}

	if isToStruct {
		// Deductible: raise the max account's fee_source leg by delta and deduct
		// the same delta from the max account's balance so the internal
		// debit==credit identity (legs == deductions) is preserved.
		leg, ok := updateAmount[target.debitLegKey]
		if !ok {
			return
		}

		leg.Value = leg.Value.Add(delta)
		updateAmount[target.debitLegKey] = leg

		if payer, ok := updateAmount[target.payerKey]; ok {
			payer.Value = payer.Value.Sub(delta)
			updateAmount[target.payerKey] = payer
		}

		return
	}

	// Non-deductible: move the debit leg and its credit mirror together.
	debit, ok := updateAmount[target.debitLegKey]
	if !ok {
		return
	}

	debit.Value = debit.Value.Add(delta)
	updateAmount[target.debitLegKey] = debit

	credit, ok := updateAmountToStruct[target.creditLegKey]
	if !ok {
		return
	}

	credit.Value = credit.Value.Add(delta)
	updateAmountToStruct[target.creditLegKey] = credit
}

// applyProportionalFee applies the proportional fee
func applyProportionalFee(feeModel model.Fee, feeIndex int, amounts *map[string]transaction.Amount,
	amountsToStruct map[string]transaction.Amount, feeValue transaction.Amount, exemptAccounts *[]string,
	segmentIDs []uuid.UUID, segCtx *SegmentContext, isToStruct bool,
) (map[string]transaction.Amount, map[string]transaction.Amount, error) {
	maxAccount, err := findMaxAccount(*amounts, exemptAccounts, segmentIDs, segCtx)
	if err != nil {
		return *amounts, amountsToStruct, err
	}

	var target feeCorrectionTarget

	updateAmount, updateAmountToStruct, newFeeTotalPaying, err := calculateProportionalFees(
		feeModel, feeIndex, amounts, amountsToStruct, feeValue, exemptAccounts, segmentIDs, segCtx, isToStruct, maxAccount, &target,
	)
	if err != nil {
		return *amounts, amountsToStruct, err
	}

	applyFeeCorrection(updateAmount, updateAmountToStruct, feeValue, newFeeTotalPaying, isToStruct, target)

	return updateAmount, updateAmountToStruct, nil
}

// allAccountsExempt returns true when every account in the map is exempt from fees,
// either via direct alias matching or segment-based resolution.
// Returns false for an empty map (no accounts to exempt).
func allAccountsExempt(accounts map[string]transaction.Amount, exemptAccounts *[]string, segmentIDs []uuid.UUID, segCtx *SegmentContext) (bool, error) {
	if len(accounts) == 0 {
		return false, nil
	}

	for key := range accounts {
		exempt, err := isAccountExemptOrSegment(key, exemptAccounts, segmentIDs, segCtx)
		if err != nil {
			return false, err
		}

		if !exempt {
			return false, nil
		}
	}

	return true, nil
}

// isAccountExempt checks if the account is exempt
func isAccountExempt(account string, exemptAccounts *[]string) bool {
	if exemptAccounts != nil {
		for _, exemptAccount := range *exemptAccounts {
			if account == exemptAccount {
				return true
			}
		}
	}

	return false
}

// isAccountExemptOrSegment checks exemption using both direct alias matching and segment-based resolution.
// It first canonicalizes the account key via trimFeeSuffix, stripping any decorated suffixes
// (->fee, ->fee_source, ->route) to ensure that exemption checks use the base alias, not a synthetic key.
// When segmentIDs is non-empty, it requires segCtx with a valid Resolver and delegates to
// isAccountExemptWithSegments for full segment resolution via the ledger query layer.
// If segmentIDs is non-empty but segCtx or Resolver is nil, it returns an error to prevent
// silently charging accounts that should be exempt via segment waivers.
// When segmentIDs is empty, it falls back to exact alias matching only.
func isAccountExemptOrSegment(account string, exemptAccounts *[]string, segmentIDs []uuid.UUID, segCtx *SegmentContext) (bool, error) {
	account = trimFeeSuffix(account)

	if len(segmentIDs) > 0 {
		if segCtx == nil || segCtx.Resolver == nil {
			return false, pkg.ValidateBusinessError(constant.ErrMissingSegmentContext, "")
		}

		return isAccountExemptWithSegments(
			segCtx.Ctx, account, exemptAccounts, segmentIDs,
			segCtx.Resolver, segCtx.OrganizationID, segCtx.LedgerID,
		)
	}

	return isAccountExempt(account, exemptAccounts), nil
}
