// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package constant

import (
	"errors"
)

// List of errors that can be returned.
var (
	ErrUnexpectedFieldsInTheRequest         = errors.New("FEE-0001")
	ErrMissingFieldsInRequest               = errors.New("FEE-0002")
	ErrBadRequest                           = errors.New("FEE-0003")
	ErrInternalServer                       = errors.New("FEE-0004")
	ErrCalculationFieldType                 = errors.New("FEE-0005")
	ErrInvalidQueryParameter                = errors.New("FEE-0006")
	ErrInvalidDateFormat                    = errors.New("FEE-0007")
	ErrInvalidFinalDate                     = errors.New("FEE-0008")
	ErrDateRangeExceedsLimit                = errors.New("FEE-0009")
	ErrInvalidDateRange                     = errors.New("FEE-0010")
	ErrPaginationLimitExceeded              = errors.New("FEE-0011")
	ErrEntityNotFound                       = errors.New("FEE-0012")
	ErrPriorityInvalid                      = errors.New("FEE-0013")
	ErrFindAccountOnMidaz                   = errors.New("FEE-0014")
	ErrMinAmountGreaterThanMaxAmount        = errors.New("FEE-0015")
	ErrInvalidPathParameter                 = errors.New("FEE-0016")
	ErrNothingToUpdate                      = errors.New("FEE-0017")
	ErrDuplicatePackage                     = errors.New("FEE-0018")
	ErrInvalidHeaderParameter               = errors.New("FEE-0019")
	ErrInvalidTransactionType               = errors.New("FEE-0021")
	ErrCalculateFee                         = errors.New("FEE-0022")
	ErrCalculationRequired                  = errors.New("FEE-0023")
	ErrPriorityOne                          = errors.New("FEE-0024")
	ErrAppRuleFlatFeeAndPercentual          = errors.New("FEE-0025")
	ErrCalculationTypePercentual            = errors.New("FEE-0026")
	ErrCalculationTypeFlatFee               = errors.New("FEE-0027")
	ErrFeeFieldsRequired                    = errors.New("FEE-0028")
	ErrCalculationFieldOfFeeRequired        = errors.New("FEE-0029")
	ErrReferenceAmountInvalid               = errors.New("FEE-0030")
	ErrAppRuleInvalid                       = errors.New("FEE-0031")
	ErrCalculationTypeInvalid               = errors.New("FEE-0032")
	ErrMaxAmountLessThanMinAmount           = errors.New("FEE-0033")
	ErrFilterPackage                        = errors.New("FEE-0034")
	ErrPackageRange                         = errors.New("FEE-0035")
	ErrInvalidSortOrder                     = errors.New("FEE-0036")
	ErrValidateDistributeTransactionValue   = errors.New("FEE-0037")
	ErrAppRuleMaxBetweenTypes               = errors.New("FEE-0038")
	ErrInvalidSegmentID                     = errors.New("FEE-0039")
	ErrInvalidLedgerID                      = errors.New("FEE-0040")
	ErrInvalidRequestBody                   = errors.New("FEE-0041")
	ErrConvertToDecimal                     = errors.New("FEE-0042")
	ErrIsDeductibleFrom                     = errors.New("FEE-0043")
	ErrApplicationRule                      = errors.New("FEE-0044")
	ErrForbiddenAccessMidaz                 = errors.New("FEE-0045")
	ErrCalculationValuePercentage           = errors.New("FEE-0046")
	ErrCalculationValueFlatFee              = errors.New("FEE-0047")
	ErrAccessMidaz                          = errors.New("FEE-0048")
	ErrDeductibleCalculationValuePercentage = errors.New("FEE-0049")
	ErrDeductibleCalculationValueFlatFee    = errors.New("FEE-0050")
	ErrInvalidQueryParameterPage            = errors.New("FEE-0051")

	// Motor 2 - Billing CRUD errors
	ErrBillingPackageNotFound    = errors.New("FEE-0052")
	ErrInvalidBillingPackageType = errors.New("FEE-0053")
	ErrMissingVolumeFields       = errors.New("FEE-0054")
	ErrMissingMaintenanceFields  = errors.New("FEE-0055")
	ErrInvalidPricingModel       = errors.New("FEE-0056")
	ErrInvalidPricingTier        = errors.New("FEE-0057")
	ErrBillingRouteOverlap       = errors.New("FEE-0058")
	ErrInvalidBillingPeriod      = errors.New("FEE-0063")
	ErrInvalidFreeQuota          = errors.New("FEE-0064")
	ErrInvalidDiscountTier       = errors.New("FEE-0065")
	ErrInvalidCountMode          = errors.New("FEE-0067")
	ErrInvalidFeeAmount          = errors.New("FEE-0070")

	// Motor 2 - Billing Calculation errors
	ErrTargetAccountNotFound    = errors.New("FEE-0059")
	ErrBillingCalculationFailed = errors.New("FEE-0060")
	ErrNoActiveBillingPackages  = errors.New("FEE-0061")

	// Motor 2 - Integration errors
	ErrSegmentResolutionFailed = errors.New("FEE-0062")
	ErrMidazQueryFailed        = errors.New("FEE-0068")
	ErrInvalidAccountTarget    = errors.New("FEE-0069")
	ErrMissingSegmentContext   = errors.New("FEE-0071")
	ErrMidazRouteNotFound      = errors.New("FEE-0072")
)
