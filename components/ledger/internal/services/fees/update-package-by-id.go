// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/bsondecimal"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"

	"github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"github.com/iancoleman/strcase"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

var ErrDatabaseItemNotFound = errors.New("errDatabaseItemNotFound")

// UpdatePackageByID update an example from the repository.
func (uc *UseCase) UpdatePackageByID(ctx context.Context, id, organizationID uuid.UUID, up *model.UpdatePackageInput) error {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.update_package_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	logger.Log(ctx, libLog.LevelInfo, "Trying to update package", libLog.String("package_id", id.String()))

	setOperationFields, unsetOperationFields, errUpdateFields := uc.buildUpdateFields(ctx, logger, id, organizationID, up)
	if errUpdateFields != nil {
		return errUpdateFields
	}

	updateFields := bson.M{}
	if len(setOperationFields) > 0 {
		updateFields["$set"] = setOperationFields
	}

	if len(unsetOperationFields) > 0 {
		updateFields["$unset"] = unsetOperationFields
	}

	err := uc.packageRepo.Update(ctx, id, organizationID, &updateFields)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update organization on repo by id", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error updating package on repo by id: %v", err))

		if errors.Is(err, ErrDatabaseItemNotFound) {
			return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", reflect.TypeOf(pack.Package{}).Name())
		}

		return err
	}

	return nil
}

// buildUpdateFields Build the fields that will be updated
func (uc *UseCase) buildUpdateFields(ctx context.Context, logger libLog.Logger, packageID, organizationID uuid.UUID, up *model.UpdatePackageInput) (bson.M, bson.M, error) {
	setFields := bson.M{}
	unsetFields := bson.M{}

	feesAmountData, errFindFees := uc.packageRepo.
		FindFeesAndAmountDataByPackageID(ctx, organizationID, packageID)
	if errFindFees != nil {
		return nil, nil, errFindFees
	}

	// Update amounts
	if up.MinAmount != nil || up.MaxAmount != nil {
		if errSetAmounts := uc.SetAmountsDataToUpdate(ctx, logger, up, feesAmountData, organizationID, &packageID, setFields); errSetAmounts != nil {
			return nil, nil, errSetAmounts
		}
	}

	if !commons.IsNilOrEmpty(&up.FeeGroupLabel) {
		setFields["fee_group_label"] = up.FeeGroupLabel
	}

	if !commons.IsNilOrEmpty(&up.Description) {
		setFields["description"] = up.Description
	}

	if up.EnablePackage != nil {
		setFields["enable"] = *up.EnablePackage
	}

	if up.WaivedAccounts != nil {
		setFields["waived_accounts"] = up.WaivedAccounts
	}

	// Update fee map
	if up.Fee != nil {
		errValidationFeesSet := uc.validationFeesSetUnset(ctx, feesAmountData.MinAmount, organizationID, feesAmountData.LedgerID, feesAmountData.Fees, up.Fee, setFields, unsetFields)
		if errValidationFeesSet != nil {
			return nil, nil, errValidationFeesSet
		}
	}

	if len(setFields) == 0 && len(unsetFields) == 0 {
		return setFields, unsetFields, pkg.ValidateBusinessError(constant.ErrNothingToUpdate, reflect.TypeOf(pack.Package{}).Name())
	}

	setFields["updated_at"] = time.Now()

	return setFields, unsetFields, nil
}

// validationFeesSetUnset Validate the fee struct to update correctly
func (uc *UseCase) validationFeesSetUnset(ctx context.Context, minAmount decimal.Decimal, organizationID, ledgerID uuid.UUID, existingFees map[string]model.Fee, updateFeesEntity map[string]model.Fee, setFields, unsetFields bson.M) error {
	// First pass: process all fees and build final state
	finalFees := make(map[string]model.Fee)
	prioritySet := make(map[int]struct{})

	// Start with existing fees
	for key, fee := range existingFees {
		finalFees[key] = fee
		prioritySet[fee.Priority] = struct{}{}
	}

	// Process update fees
	for key, fee := range updateFeesEntity {
		keyFormatted := strcase.ToLowerCamel(key)
		_, feeExists := existingFees[keyFormatted]

		if !feeExists {
			// New fee - validate and add to final state
			err := fee.ValidateNewFee(key, minAmount)
			if err != nil {
				return err
			}

			// Validate that the credit account exists.
			if errGetAccount := uc.resolver.AccountExistsByAlias(ctx, organizationID, ledgerID, fee.CreditAccount); errGetAccount != nil {
				return errGetAccount
			}

			// Convert fee to MongoDB format and add to setFields
			mongoFee, errConvert := uc.convertFeeToMongoFormat(fee)
			if errConvert != nil {
				return errConvert
			}

			setFields["fees."+keyFormatted] = mongoFee

			// Add to final state for priority validation
			finalFees[keyFormatted] = fee
		} else {
			// Existing fee - check if it's being updated or removed
			hasFieldsToUpdate, errSetFieldsToUpdate := fee.SetAndValidateHasFieldsToUpdate(ctx, fee.IsDeductibleFrom, minAmount, existingFees, keyFormatted, organizationID, ledgerID, setFields, uc.resolver)
			if errSetFieldsToUpdate != nil {
				return errSetFieldsToUpdate
			}

			if !hasFieldsToUpdate {
				// Fee is being removed
				unsetFields["fees."+keyFormatted] = ""

				delete(finalFees, keyFormatted)
			} else {
				// Fee is being updated - update in final state
				finalFees[keyFormatted] = fee
			}
		}
	}

	// Second pass: validate priorities in final state
	finalPrioritySet := make(map[int]struct{})
	for _, fee := range finalFees {
		if _, exists := finalPrioritySet[fee.Priority]; exists {
			return pkg.ValidateBusinessError(constant.ErrPriorityInvalid, "")
		}

		finalPrioritySet[fee.Priority] = struct{}{}
	}

	return nil
}

// SetAmountsDataToUpdate Setting the amounts data existent of update object
func (uc *UseCase) SetAmountsDataToUpdate(ctx context.Context, logger libLog.Logger, up *model.UpdatePackageInput,
	feesAmountData *model.AmountData, organizationID uuid.UUID, packageID *uuid.UUID, setFields bson.M,
) error {
	var (
		maxAmount string
		minAmount string
	)

	// validate minimum and maximum amount value
	if errMinMaxValue := up.ValidateMinAndMaxAmount(); errMinMaxValue != nil {
		return errMinMaxValue
	}

	switch {
	case up.MinAmount != nil && up.MaxAmount != nil:
		maxAmount = *up.MaxAmount
		minAmount = *up.MinAmount
		setFields["minimum_amount"] = up.MinAmount
		setFields["maximum_amount"] = up.MaxAmount
	case up.MinAmount != nil:
		// validate the minimum amount that will be updated
		errValidateMinAmount := up.ValidateMinAmountUpdate(feesAmountData.MaxAmount)
		if errValidateMinAmount != nil {
			return errValidateMinAmount
		}

		minAmount = *up.MinAmount
		maxAmount = feesAmountData.MaxAmount.String()
		setFields["minimum_amount"] = up.MinAmount
	case up.MaxAmount != nil:
		// validate the maximum amount that will be updated
		errValidateMaxAmount := up.ValidateMaxAmountUpdate(feesAmountData.MinAmount)
		if errValidateMaxAmount != nil {
			return errValidateMaxAmount
		}

		minAmount = feesAmountData.MinAmount.String()
		maxAmount = *up.MaxAmount
		setFields["maximum_amount"] = up.MaxAmount
	}

	// validating max and min amount range of a package
	if errRange := uc.ValidatePackageMaxAndMinAmountRange(
		ctx, logger, maxAmount, minAmount, feesAmountData.GetTransactionRoute(),
		organizationID, feesAmountData.LedgerID, feesAmountData.SegmentID, packageID); errRange != nil {
		return errRange
	}

	return nil
}

// convertFeeToMongoFormat converts a model.Fee to pack.Fee (MongoDB format)
func (uc *UseCase) convertFeeToMongoFormat(fee model.Fee) (pack.Fee, error) {
	// Convert calculations to MongoDB format
	calculations := make([]pack.Calculation, 0, len(fee.CalculationModel.Calculations))

	for _, calc := range fee.CalculationModel.Calculations {
		value, err := decimal.NewFromString(calc.Value)
		if err != nil {
			return pack.Fee{}, pkg.ValidateBusinessError(constant.ErrConvertToDecimal, reflect.TypeOf(pack.Package{}).Name(), "calculationModel.calculations.value")
		}

		calculations = append(calculations, pack.Calculation{
			Type:  calc.Type,
			Value: bsondecimal.Decimal{Decimal: value},
		})
	}

	// Convert calculation model
	calcModel := pack.CalculationModel{
		ApplicationRule: fee.CalculationModel.ApplicationRule,
		Calculations:    calculations,
	}

	return pack.Fee{
		FeeLabel:         fee.FeeLabel,
		CalculationModel: calcModel,
		ReferenceAmount:  fee.ReferenceAmount,
		Priority:         fee.Priority,
		IsDeductibleFrom: fee.IsDeductibleFrom,
		CreditAccount:    fee.CreditAccount,
		RouteFrom:        fee.RouteFrom,
		RouteTo:          fee.RouteTo,
	}, nil
}
