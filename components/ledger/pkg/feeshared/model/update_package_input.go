// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"context"
	"strings"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"

	"github.com/LerianStudio/lib-commons/v5/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// UpdatePackageInput is a struct designed to update data.
//
// swagger:model UpdatePackageInput
//
//	@Description	UpdatePackageInput is the input payload to update a pack.
type UpdatePackageInput struct {
	FeeGroupLabel  string         `json:"feeGroupLabel" example:"Pacote Padrão"`
	Description    string         `json:"description" example:"Pacote de taxas administrativas padrão"`
	MinAmount      *string        `json:"minimumAmount" example:"100" minimum:"0"`
	MaxAmount      *string        `json:"maximumAmount" example:"1000" minimum:"0"`
	WaivedAccounts *[]string      `json:"waivedAccounts" example:"['acc001', 'ac0002']"`
	Fee            map[string]Fee `json:"fees"`
	EnablePackage  *bool          `json:"enable,omitempty" example:"true"`
} //	@name	UpdatePackageInput

// GetMinimumAmount returns the minimum amount value
func (up *UpdatePackageInput) GetMinimumAmount() string {
	if up.MinAmount == nil {
		return ""
	}

	return *up.MinAmount
}

// GetMaximumAmount returns the maximum amount value
func (up *UpdatePackageInput) GetMaximumAmount() string {
	if up.MaxAmount == nil {
		return ""
	}

	return *up.MaxAmount
}

func (up *UpdatePackageInput) ValidateFees() error {
	for key, fee := range up.Fee {
		if !fee.ValidateIfFeeIsNil() {
			if fee.Priority != 0 && fee.ReferenceAmount != "" {
				if fee.Priority == 1 && fee.ReferenceAmount != OriginalAmount {
					return pkg.ValidateBusinessError(constant.ErrPriorityOne, "", key)
				}
			}

			if fee.ReferenceAmount != "" && fee.IsDeductibleFrom != nil {
				if fee.GetIsDeductibleFrom() && fee.ReferenceAmount != OriginalAmount {
					return pkg.ValidateBusinessError(constant.ErrIsDeductibleFrom, "", key)
				}
			}

			if fee.CalculationModel != nil {
				if err := validateCalculationModel(fee.CalculationModel, up.GetMinimumAmount(), key, fee.GetIsDeductibleFrom()); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// ValidateMinAndMaxAmount Validating if minimum amount value is greater than maximum amount value
func (up *UpdatePackageInput) ValidateMinAndMaxAmount() error {
	var (
		minRealValue decimal.Decimal
		maxRealValue decimal.Decimal
		err          error
	)

	if up.MinAmount != nil {
		minRealValue, err = decimal.NewFromString(*up.MinAmount)
		if err != nil {
			return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "minimumAmount")
		}
	}

	if up.MaxAmount != nil {
		maxRealValue, err = decimal.NewFromString(*up.MaxAmount)
		if err != nil {
			return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "maximumAmount")
		}
	}

	if up.MaxAmount != nil && up.MinAmount != nil {
		if minRealValue.GreaterThan(maxRealValue) {
			return pkg.ValidateBusinessError(constant.ErrMinAmountGreaterThanMaxAmount, "")
		}
	}

	return nil
}

// ValidateMinAndMaxAmountValue validate if minimum and maximum amount is valid
func (up *UpdatePackageInput) ValidateMinAndMaxAmountValue() error {
	if strings.Contains(up.GetMinimumAmount(), ",") {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "minimumAmount")
	}

	if strings.Contains(up.GetMaximumAmount(), ",") {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "maximumAmount")
	}

	return nil
}

// ValidateMinAmountUpdate Validate minimum amount value that will be updated
func (up *UpdatePackageInput) ValidateMinAmountUpdate(maxAmountData decimal.Decimal) error {
	minAmountConverted, err := decimal.NewFromString(*up.MinAmount)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "minimumAmount")
	}

	if minAmountConverted.GreaterThan(maxAmountData) {
		return pkg.ValidateBusinessError(constant.ErrMinAmountGreaterThanMaxAmount, "")
	}

	return nil
}

// ValidateMaxAmountUpdate Validate maximum amount value that will be updated
func (up *UpdatePackageInput) ValidateMaxAmountUpdate(minAmountData decimal.Decimal) error {
	maxAmountConverted, err := decimal.NewFromString(*up.MaxAmount)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "maximumAmount")
	}

	if maxAmountConverted.LessThan(minAmountData) {
		return pkg.ValidateBusinessError(constant.ErrMaxAmountLessThanMinAmount, "")
	}

	return nil
}

// AmountData holds the amount range and fee data for a package.
type AmountData struct {
	MinAmount        decimal.Decimal
	MaxAmount        decimal.Decimal
	Fees             map[string]Fee
	LedgerID         uuid.UUID
	SegmentID        *uuid.UUID
	TransactionRoute *string
}

func (a *AmountData) GetTransactionRoute() string {
	if a.TransactionRoute == nil {
		return ""
	}

	return *a.TransactionRoute
}

func (f *Fee) SetAndValidateHasFieldsToUpdate(ctx context.Context, updateDeductibleFrom *bool, minAmount decimal.Decimal, existingFees map[string]Fee, feeKey string, organizationID, ledgerID uuid.UUID, upFields bson.M, resolver pkg.MidazResolver) (bool, error) {
	hasValueToUpdate := false

	if updated, err := f.updateCalculationModel(existingFees, updateDeductibleFrom, feeKey, minAmount, upFields); err != nil {
		return hasValueToUpdate, err
	} else {
		hasValueToUpdate = hasValueToUpdate || updated
	}

	if updated, err := f.updateIsDeductibleFrom(hasValueToUpdate, existingFees, feeKey, minAmount, upFields); err != nil {
		return hasValueToUpdate, err
	} else {
		hasValueToUpdate = hasValueToUpdate || updated
	}

	if updated := f.updateFeeLabel(feeKey, upFields); updated {
		hasValueToUpdate = true
	}

	if updated, err := f.updateReferenceAmount(existingFees, feeKey, upFields); err != nil {
		return hasValueToUpdate, err
	} else {
		hasValueToUpdate = hasValueToUpdate || updated
	}

	if updated := f.updatePriority(feeKey, upFields); updated {
		hasValueToUpdate = true
	}

	if updated, err := f.updateCreditAccount(ctx, feeKey, organizationID, ledgerID, upFields, resolver); err != nil {
		return false, err
	} else {
		hasValueToUpdate = hasValueToUpdate || updated
	}

	if updated := f.updateRouteFrom(feeKey, upFields); updated {
		hasValueToUpdate = true
	}

	if updated := f.updateRouteTo(feeKey, upFields); updated {
		hasValueToUpdate = true
	}

	return hasValueToUpdate, nil
}

// --- Helper methods for SetAndValidateHasFieldsToUpdate ---

func (f *Fee) updateFeeLabel(feeKey string, upFields bson.M) bool {
	if !commons.IsNilOrEmpty(&f.FeeLabel) {
		upFields["fees."+feeKey+".fee_label"] = f.FeeLabel
		return true
	}

	return false
}

func (f *Fee) updateCalculationModel(existingFees map[string]Fee, updateDeductibleFrom *bool, feeKey string, minAmount decimal.Decimal, upFields bson.M) (bool, error) {
	if f.CalculationModel != nil {
		return f.setAndValidateCalculationModel(existingFees, updateDeductibleFrom, feeKey, minAmount, upFields)
	}

	return false, nil
}

func (f *Fee) updateReferenceAmount(existingFees map[string]Fee, feeKey string, upFields bson.M) (bool, error) {
	if !commons.IsNilOrEmpty(&f.ReferenceAmount) {
		if f.validateReferenceAmountIsInvalid() {
			return false, pkg.ValidateBusinessError(constant.ErrReferenceAmountInvalid, "")
		}

		if f.IsDeductibleFrom == nil {
			if f.ReferenceAmount == constant.ReferenceAmountAfterFeesAmount && existingFees[feeKey].IsDeductibleFrom != nil && *existingFees[feeKey].IsDeductibleFrom {
				return false, pkg.ValidateBusinessError(constant.ErrIsDeductibleFrom, "", feeKey)
			}
		}

		upFields["fees."+feeKey+".reference_amount"] = f.ReferenceAmount

		return true, nil
	}

	return false, nil
}

func (f *Fee) updatePriority(feeKey string, upFields bson.M) bool {
	if f.Priority != 0 {
		upFields["fees."+feeKey+".priority"] = f.Priority
		return true
	}

	return false
}

func (f *Fee) updateIsDeductibleFrom(hasUpdatedCalculationModel bool, existingFees map[string]Fee, feeKey string, minAmount decimal.Decimal, upFields bson.M) (bool, error) {
	if f.IsDeductibleFrom == nil {
		return false, nil
	}

	if err := f.validateDeductibleFromReferenceAmount(existingFees, feeKey); err != nil {
		return false, err
	}

	if err := f.validateDeductibleFromCalculations(hasUpdatedCalculationModel, existingFees, feeKey, minAmount); err != nil {
		return false, err
	}

	upFields["fees."+feeKey+".is_deductible_from"] = f.IsDeductibleFrom

	return true, nil
}

// validateDeductibleFromReferenceAmount validates reference amount constraints for deductible fees
func (f *Fee) validateDeductibleFromReferenceAmount(existingFees map[string]Fee, feeKey string) error {
	if !commons.IsNilOrEmpty(&f.ReferenceAmount) {
		return nil
	}

	existingFee := existingFees[feeKey]
	if existingFee.ReferenceAmount == constant.ReferenceAmountAfterFeesAmount && f.GetIsDeductibleFrom() {
		return pkg.ValidateBusinessError(constant.ErrIsDeductibleFrom, "", feeKey)
	}

	return nil
}

// validateDeductibleFromCalculations validates calculation values for deductible fees
func (f *Fee) validateDeductibleFromCalculations(hasUpdatedCalculationModel bool, existingFees map[string]Fee, feeKey string, minAmount decimal.Decimal) error {
	if !f.GetIsDeductibleFrom() || hasUpdatedCalculationModel {
		return nil
	}

	existingFee := existingFees[feeKey]
	if existingFee.CalculationModel == nil {
		return nil
	}

	for _, calc := range existingFee.CalculationModel.Calculations {
		if err := f.validateDeductibleCalculation(calc, minAmount, feeKey); err != nil {
			return err
		}
	}

	return nil
}

// validateDeductibleCalculation validates a single calculation for deductible fees
func (f *Fee) validateDeductibleCalculation(calc Calculation, minAmount decimal.Decimal, feeKey string) error {
	valueCalc, err := decimal.NewFromString(calc.Value)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", feeKey+".calculationModel.calculations.value")
	}

	switch calc.Type {
	case Percentage:
		return f.validatePercentageCalculation(valueCalc, feeKey)
	case Flat:
		return f.validateFlatCalculation(valueCalc, minAmount, feeKey)
	}

	return nil
}

// validatePercentageCalculation validates percentage calculation for deductible fees
func (f *Fee) validatePercentageCalculation(valueCalc decimal.Decimal, feeKey string) error {
	oneHundredPercent := decimal.NewFromInt(100)
	if valueCalc.GreaterThan(oneHundredPercent) {
		return pkg.ValidateBusinessError(constant.ErrDeductibleCalculationValuePercentage, "", feeKey)
	}

	return nil
}

// validateFlatCalculation validates flat calculation for deductible fees
func (f *Fee) validateFlatCalculation(valueCalc, minAmount decimal.Decimal, feeKey string) error {
	if valueCalc.GreaterThan(minAmount) {
		return pkg.ValidateBusinessError(constant.ErrDeductibleCalculationValueFlatFee, "", minAmount, feeKey)
	}

	return nil
}

func (f *Fee) updateCreditAccount(ctx context.Context, feeKey string, organizationID, ledgerID uuid.UUID, upFields bson.M, resolver pkg.MidazResolver) (bool, error) {
	if !commons.IsNilOrEmpty(&f.CreditAccount) {
		return f.setAndValidateCreditAccount(ctx, feeKey, organizationID, ledgerID, upFields, resolver)
	}

	return false, nil
}

func (f *Fee) updateRouteFrom(feeKey string, upFields bson.M) bool {
	if !commons.IsNilOrEmpty(f.RouteFrom) {
		upFields["fees."+feeKey+".route_from"] = f.RouteFrom
		return true
	}

	return false
}

func (f *Fee) updateRouteTo(feeKey string, upFields bson.M) bool {
	if !commons.IsNilOrEmpty(f.RouteTo) {
		upFields["fees."+feeKey+".route_to"] = f.RouteTo
		return true
	}

	return false
}

// setAndValidateCalculationModel handles calculation model validation and update logic
func (f *Fee) setAndValidateCalculationModel(existingFees map[string]Fee, updateDeductibleFrom *bool, feeKey string, minAmount decimal.Decimal, upFields bson.M) (bool, error) {
	if f.hasNoCalculationModelUpdates() {
		return false, nil
	}

	appRuleUpdated, err := f.updateApplicationRule(feeKey, upFields)
	if err != nil {
		return false, err
	}

	calculationsUpdated, err := f.updateCalculations(existingFees, updateDeductibleFrom, feeKey, minAmount, upFields)
	if err != nil {
		return false, err
	}

	return appRuleUpdated || calculationsUpdated, nil
}

// hasNoCalculationModelUpdates checks if there are no calculation model updates to process
func (f *Fee) hasNoCalculationModelUpdates() bool {
	return commons.IsNilOrEmpty(&f.CalculationModel.ApplicationRule) && len(f.CalculationModel.Calculations) == 0
}

// updateApplicationRule handles application rule validation and update
func (f *Fee) updateApplicationRule(feeKey string, upFields bson.M) (bool, error) {
	if commons.IsNilOrEmpty(&f.CalculationModel.ApplicationRule) {
		return false, nil
	}

	if f.validateAppRuleIsInvalid() {
		return false, pkg.ValidateBusinessError(constant.ErrAppRuleInvalid, "")
	}

	upFields["fees."+feeKey+".calculation_model.application_rule"] = f.CalculationModel.ApplicationRule

	return true, nil
}

// updateCalculations handles calculations validation and update
func (f *Fee) updateCalculations(existingFees map[string]Fee, updateDeductibleFrom *bool, feeKey string, minAmount decimal.Decimal, upFields bson.M) (bool, error) {
	if len(f.CalculationModel.Calculations) == 0 {
		return false, nil
	}

	calculations, err := f.validateAndFormatCalculations(existingFees, updateDeductibleFrom, feeKey, minAmount)
	if err != nil {
		return false, err
	}

	upFields["fees."+feeKey+".calculation_model.calculations"] = calculations

	return true, nil
}

// validateAndFormatCalculations validates and formats all calculations
func (f *Fee) validateAndFormatCalculations(existingFees map[string]Fee, updateDeductibleFrom *bool, feeKey string, minAmount decimal.Decimal) ([]map[string]any, error) {
	calculations := make([]map[string]any, len(f.CalculationModel.Calculations))

	for i, calc := range f.CalculationModel.Calculations {
		if err := f.validateCalculationType(calc); err != nil {
			return nil, err
		}

		if err := f.validateCalculationValue(calc, existingFees, updateDeductibleFrom, feeKey, minAmount); err != nil {
			return nil, err
		}

		calculations[i] = f.formatCalculationFieldName(calc)
	}

	return calculations, nil
}

// validateCalculationType validates the calculation type
func (f *Fee) validateCalculationType(calc Calculation) error {
	if calc.Type != "percentage" && calc.Type != "flat" {
		return pkg.ValidateBusinessError(constant.ErrCalculationTypeInvalid, "")
	}

	return nil
}

// validateCalculationValue validates calculation value based on deductible status
func (f *Fee) validateCalculationValue(calc Calculation, existingFees map[string]Fee, updateDeductibleFrom *bool, feeKey string, minAmount decimal.Decimal) error {
	valueCalc, err := decimal.NewFromString(calc.Value)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", feeKey+".calculationModel.calculations.value")
	}

	if !f.shouldValidateDeductibleCalculation(existingFees, updateDeductibleFrom, feeKey) {
		return nil
	}

	switch calc.Type {
	case Percentage:
		return f.validatePercentageCalculation(valueCalc, feeKey)
	case Flat:
		return f.validateFlatCalculation(valueCalc, minAmount, feeKey)
	}

	return nil
}

// shouldValidateDeductibleCalculation determines if deductible validation is needed
func (f *Fee) shouldValidateDeductibleCalculation(existingFees map[string]Fee, updateDeductibleFrom *bool, feeKey string) bool {
	if updateDeductibleFrom != nil {
		return *updateDeductibleFrom
	}

	existing := existingFees[feeKey]

	return existing.IsDeductibleFrom != nil && *existing.IsDeductibleFrom
}

// setAndValidateCreditAccount handles credit account validation and update logic
func (f *Fee) setAndValidateCreditAccount(ctx context.Context, feeKey string, organizationID, ledgerID uuid.UUID, upFields bson.M, resolver pkg.MidazResolver) (bool, error) {
	if errValidate := resolver.AccountExistsByAlias(ctx, organizationID, ledgerID, f.CreditAccount); errValidate != nil {
		return false, errValidate
	}

	upFields["fees."+feeKey+".credit_account"] = f.CreditAccount

	return true, nil
}
