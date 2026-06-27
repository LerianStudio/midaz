// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package model

import (
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// CreatePackageInput is a struct designed to encapsulate request create payload data.
//
// swagger:model CreatePackageInput
//
//	@Description	CreatePackageInput is the input payload to create a pack.
type CreatePackageInput struct {
	FeeGroupLabel    string         `json:"feeGroupLabel" validate:"required" example:"Pacote Padrão"`
	Description      *string        `json:"description,omitempty" example:"Pacote de taxas administrativas padrão"`
	SegmentID        *string        `json:"segmentId" example:"00000000-0000-0000-0000-000000000000"`
	LedgerID         string         `json:"ledgerId" validate:"required" example:"00000000-0000-0000-0000-000000000000"`
	TransactionRoute *string        `json:"transactionRoute,omitempty" example:"debitoted"`
	MinAmount        string         `json:"minimumAmount" validate:"required" example:"100.00" minimum:"0"`
	MaxAmount        string         `json:"maximumAmount" validate:"required" example:"1000.20" minimum:"0"`
	WaivedAccounts   *[]string      `json:"waivedAccounts,omitempty" example:"[\"acc001\", \"acc002\"]"`
	Fee              map[string]Fee `json:"fees" validate:"required,min=1,dive"`
	Enable           *bool          `json:"enable" validate:"required"`
} //	@name	CreatePackageInput

func (cp *CreatePackageInput) GetTransactionRoute() string {
	if cp.TransactionRoute == nil {
		return ""
	}

	return *cp.TransactionRoute
}

// ValidateFees Validating the Fee map values
func (cp *CreatePackageInput) ValidateFees() error {
	for key, fee := range cp.Fee {
		if fee.Priority == 1 && fee.ReferenceAmount != OriginalAmount {
			return pkg.ValidateBusinessError(constant.ErrPriorityOne, "", key)
		}

		if fee.GetIsDeductibleFrom() && fee.ReferenceAmount != OriginalAmount {
			return pkg.ValidateBusinessError(constant.ErrIsDeductibleFrom, "", key)
		}

		if err := validateCalculationModel(fee.CalculationModel, cp.MinAmount, key, fee.GetIsDeductibleFrom()); err != nil {
			return err
		}
	}

	return nil
}

// ValidateMinAndMaxAmount Validating if minimum amount value is greater than maximum amount value
func (cp *CreatePackageInput) ValidateMinAndMaxAmount() error {
	minRealValue, err := parseAmountDecimal(cp.MinAmount)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "minimumAmount")
	}

	maxRealValue, err := parseAmountDecimal(cp.MaxAmount)
	if err != nil {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "maximumAmount")
	}

	if minRealValue.GreaterThan(maxRealValue) {
		return pkg.ValidateBusinessError(constant.ErrMinAmountGreaterThanMaxAmount, "")
	}

	if strings.Contains(cp.MinAmount, ",") {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "minimumAmount")
	}

	if strings.Contains(cp.MaxAmount, ",") {
		return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "maximumAmount")
	}

	return nil
}
