// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"

	"github.com/shopspring/decimal"
)

const (
	FlatFee        = "flatFee"
	OriginalAmount = "originalAmount"
	Percentual     = "percentual"
	MaxBetween     = "maxBetweenTypes"
	Flat           = "flat"
	Percentage     = "percentage"
)

// Fee is a struct designed to encapsulate request create payload data.
//
// swagger:model Fee
//
//	@Description	Fee is the input payload to create a fee of a pack.
type Fee struct {
	FeeLabel         string            `json:"feeLabel" validate:"required" example:"Taxa Administrativa"`
	CalculationModel *CalculationModel `json:"calculationModel" validate:"required"`
	ReferenceAmount  string            `json:"referenceAmount" validate:"oneof=originalAmount afterFeesAmount" example:"originalAmount"`
	Priority         int               `json:"priority,omitempty" validate:"gte=0" example:"1"`
	IsDeductibleFrom *bool             `json:"isDeductibleFrom" validate:"required" example:"true"`
	CreditAccount    string            `json:"creditAccount" validate:"required" example:"conta_receita_taxas_adm"`
	RouteFrom        *string           `json:"routeFrom,omitempty" example:"taxa_débito"`
	RouteTo          *string           `json:"routeTo,omitempty" example:"taxa_crédito"`
} //	@name	Fee

func (f *Fee) GetIsDeductibleFrom() bool {
	if f.IsDeductibleFrom == nil {
		return false
	}

	return *f.IsDeductibleFrom
}

func (f *Fee) GetRouteFrom() string {
	if f.RouteFrom == nil {
		return ""
	}

	return *f.RouteFrom
}

func (f *Fee) GetRouteTo() string {
	if f.RouteTo == nil {
		return ""
	}

	return *f.RouteTo
}

// CalculationModel structure for marshaling/unmarshalling JSON.
//
// swagger:model CalculationModel
//
//	@Description	CalculationModel is a struct designed to store the calculation of a fee from a pack.
type CalculationModel struct {
	ApplicationRule string        `json:"applicationRule" validate:"oneof=maxBetweenTypes flatFee percentual" example:"maxBetweenTypes"`
	Calculations    []Calculation `json:"calculations" validate:"dive"`
} //	@name	CalculationModel

// Calculation structure for marshaling/unmarshalling JSON.
//
// swagger:model Calculation
//
//	@Description	Calculation is a struct designed to store the calculation details of a fee from a pack.
type Calculation struct {
	Type  string `json:"type" validate:"oneof=percentage flat"`
	Value string `json:"value" validate:"required" example:"100.00"`
} //	@name	Calculation

// validateCalculationModel validate the calculation model
func validateCalculationModel(model *CalculationModel, minAmount, feeKey string, isDeductibleFrom bool) error {
	if model == nil {
		return pkg.ValidateBusinessError(constant.ErrCalculationRequired, "", feeKey)
	}

	if err := validateCalculationCountForRule(model, feeKey); err != nil {
		return err
	}

	if err := validateCalculationRuleAndTypes(model, feeKey); err != nil {
		return err
	}

	if err := validateCalculationValues(model, minAmount, feeKey, isDeductibleFrom); err != nil {
		return err
	}

	return nil
}

func validateCalculationCountForRule(model *CalculationModel, feeKey string) error {
	switch model.ApplicationRule {
	case FlatFee, Percentual:
		if len(model.Calculations) != 1 {
			return pkg.ValidateBusinessError(constant.ErrAppRuleFlatFeeAndPercentual, "", feeKey)
		}
	case MaxBetween:
		if len(model.Calculations) == 1 || len(model.Calculations) == 0 {
			return pkg.ValidateBusinessError(constant.ErrAppRuleMaxBetweenTypes, "", feeKey)
		}
	}

	return nil
}

func validateCalculationRuleAndTypes(model *CalculationModel, feeKey string) error {
	switch model.ApplicationRule {
	case FlatFee:
		if len(model.Calculations) == 0 || model.Calculations[0].Type != Flat {
			return pkg.ValidateBusinessError(constant.ErrCalculationTypeFlatFee, "", feeKey)
		}
	case Percentual:
		if len(model.Calculations) == 0 || model.Calculations[0].Type != Percentage {
			return pkg.ValidateBusinessError(constant.ErrCalculationTypePercentual, "", feeKey)
		}
	}

	return nil
}

func validateCalculationValues(model *CalculationModel, minAmount, feeKey string, isDeductible bool) error {
	for _, calc := range model.Calculations {
		valueCalc, err := decimal.NewFromString(calc.Value)
		if err != nil {
			return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", feeKey+".calculationModel.calculations.value")
		}

		if minAmount != "" && isDeductible {
			if calc.Type == Percentage {
				oneHundredPercent := decimal.NewFromInt(100)
				if valueCalc.GreaterThan(oneHundredPercent) {
					return pkg.ValidateBusinessError(constant.ErrCalculationValuePercentage, "", feeKey)
				}
			}

			if calc.Type == Flat {
				minAmountDecimal, errMinAmt := decimal.NewFromString(minAmount)
				if errMinAmt != nil {
					return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", feeKey+".minimumAmount")
				}

				if valueCalc.GreaterThan(minAmountDecimal) {
					return pkg.ValidateBusinessError(constant.ErrCalculationValueFlatFee, "", minAmount, feeKey)
				}
			}
		}
	}

	return nil
}

func (f *Fee) ValidateIfFeeIsNil() bool {
	return f.FeeLabel == "" &&
		f.CalculationModel == nil &&
		f.ReferenceAmount == "" &&
		f.Priority == 0 &&
		f.IsDeductibleFrom == nil &&
		f.CreditAccount == ""
}

func (f *Fee) ValidateNewFee(feeKey string, minAmount decimal.Decimal) error {
	if err := f.validateRequiredFields(); err != nil {
		return err
	}

	if err := f.validateCalculations(feeKey, minAmount); err != nil {
		return err
	}

	if err := f.validateApplicationRule(feeKey); err != nil {
		return err
	}

	return nil
}

// validateRequiredFields checks if all required fields are present
func (f *Fee) validateRequiredFields() error {
	if f.FeeLabel == "" ||
		f.CalculationModel.ApplicationRule == "" ||
		len(f.CalculationModel.Calculations) == 0 ||
		f.ReferenceAmount == "" ||
		f.Priority == 0 ||
		f.CreditAccount == "" ||
		f.IsDeductibleFrom == nil {
		return pkg.ValidateBusinessError(constant.ErrFeeFieldsRequired, "")
	}

	return nil
}

// validateCalculations validates all calculations in the fee
func (f *Fee) validateCalculations(feeKey string, minAmount decimal.Decimal) error {
	for _, calc := range f.CalculationModel.Calculations {
		if err := f.validateCalculation(calc, feeKey, minAmount); err != nil {
			return err
		}
	}

	return nil
}

// validateCalculation validates a single calculation
func (f *Fee) validateCalculation(calc Calculation, feeKey string, minAmount decimal.Decimal) error {
	calcValueConverted, err := decimal.NewFromString(calc.Value)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", feeKey+".calculationModel.calculations.value")
	}

	if calc.Type == "" || calcValueConverted.IsZero() {
		return pkg.ValidateBusinessError(constant.ErrCalculationFieldOfFeeRequired, "")
	}

	// Skip deductible validation if not applicable
	if !f.shouldValidateDeductibleCalculationForNewFee(feeKey) {
		return nil
	}

	return f.validateNewFeeCalculationValue(calc, calcValueConverted, feeKey, minAmount)
}

// validateNewFeeCalculationValue validates calculation value for new fees
func (f *Fee) validateNewFeeCalculationValue(calc Calculation, calcValue decimal.Decimal, feeKey string, minAmount decimal.Decimal) error {
	switch calc.Type {
	case Percentage:
		return f.validatePercentageCalculation(calcValue, feeKey)
	case Flat:
		return f.validateFlatCalculation(calcValue, minAmount, feeKey)
	default:
		return nil
	}
}

// shouldValidateDeductibleCalculationForNewFee determines if deductible validation is needed for new fees
func (f *Fee) shouldValidateDeductibleCalculationForNewFee(feeKey string) bool {
	newFeeMap := make(map[string]Fee)
	newFeeMap[feeKey] = *f

	return f.shouldValidateDeductibleCalculation(newFeeMap, f.IsDeductibleFrom, feeKey)
}

// validateApplicationRule validates the application rule
func (f *Fee) validateApplicationRule(feeKey string) error {
	if f.CalculationModel.ApplicationRule == MaxBetween && len(f.CalculationModel.Calculations) <= 1 {
		return pkg.ValidateBusinessError(constant.ErrAppRuleMaxBetweenTypes, "", feeKey)
	}

	return nil
}

func (f *Fee) formatCalculationFieldName(c Calculation) map[string]any {
	return map[string]any{
		"type":  c.Type,
		"value": c.Value,
	}
}

// Validation of reference amount possible values
func (f *Fee) validateReferenceAmountIsInvalid() bool {
	return f.ReferenceAmount != constant.ReferenceAmountOriginalAmount && f.ReferenceAmount != constant.ReferenceAmountAfterFeesAmount
}

// Validation of application rule possible values
func (f *Fee) validateAppRuleIsInvalid() bool {
	return f.CalculationModel.ApplicationRule != "maxBetweenTypes" &&
		f.CalculationModel.ApplicationRule != "flatFee" &&
		f.CalculationModel.ApplicationRule != "percentual"
}
