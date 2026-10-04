// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	events "github.com/LerianStudio/midaz/v4/pkg/streaming/events"
	"github.com/LerianStudio/midaz/v4/pkg/utils"

	"github.com/LerianStudio/lib-commons/v7/commons"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libStreaming "github.com/LerianStudio/lib-streaming/v4"
	"github.com/google/uuid"
	"github.com/iancoleman/strcase"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// UpdatePackageByID update an example from the repository within the given ledger.
// A ledgerID of uuid.Nil updates the package on whichever ledger of the
// organization owns it.
func (uc *UseCase) UpdatePackageByID(ctx context.Context, id, organizationID, ledgerID uuid.UUID, up *model.UpdatePackageInput) (err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.update_package_by_id")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "fees", "update_package", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
		attribute.Bool("app.request.has_ledger_id", ledgerID != uuid.Nil),
	)

	feesAmountData, err := uc.packageRepo.FindFeesAndAmountDataByPackageID(ctx, organizationID, id)
	if err != nil {
		return err
	}

	// A package on another ledger reads as absent, in the exact envelope the lookup
	// gives an unknown id, before any check reads that ledger's accounts or ranges.
	if ledgerID != uuid.Nil && feesAmountData.LedgerID != ledgerID {
		return pkg.ValidateBusinessError(constant.ErrEntityNotFound, "", "Package")
	}

	unlock, err := uc.lockPackageScope(ctx, organizationID, feesAmountData.LedgerID)
	if err != nil {
		spanattr.HandleSpanByErrorClass(span, "Failed to lock the ledger's fee packages", err)

		return err
	}

	defer unlock()

	// Every check below judges the package as it stands under the lock: the read
	// above only names the ledger to lock, and a concurrent mutation may have moved
	// its bounds or fees since.
	feesAmountData, err = uc.packageRepo.FindFeesAndAmountDataByPackageID(ctx, organizationID, id)
	if err != nil {
		return err
	}

	setOperationFields, unsetOperationFields, errUpdateFields := uc.buildUpdateFields(ctx, logger, id, organizationID, feesAmountData, up)
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

	updatedPackage, err := uc.packageRepo.Update(ctx, id, organizationID, ledgerID, &updateFields)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update package on repo by id", err)

		return err
	}

	// The write has landed: the cache and the broker are no reason to hold the lock.
	unlock()

	// Invalidate the cached enabled-package set for this (org,ledger): an update
	// can change amounts, fees, waivers, or the enable flag, all of which the
	// cached set carries. The ledger is the owning one, which under organization
	// scope is the only place it appears.
	uc.invalidatePackageCache(ctx, logger, organizationID, feesAmountData.LedgerID)

	uc.emitFeesPackageUpdatedEvent(ctx, span, logger, updatedPackage, organizationID)

	return nil
}

// emitFeesPackageUpdatedEvent publishes fee-packages.updated. IMPORTANT posture.
func (uc *UseCase) emitFeesPackageUpdatedEvent(ctx context.Context, span trace.Span, logger libLog.Logger, p *pack.Package, organizationID uuid.UUID) {
	pkgStreaming.EmitBrokerBestEffort(ctx, span, logger, uc.Streaming, events.FeesPackageUpdatedDefinition.Key(),
		func(tenantID string) (libStreaming.EmitRequest, error) {
			return events.NewFeesPackageUpdated(
				p.ID.String(), organizationID.String(), p.LedgerID.String(),
				segmentIDToString(p.SegmentID), p.TransactionRoute, enableOrFalse(p.Enable),
				p.CreatedAt, p.UpdatedAt,
			).ToEmitRequest(tenantID, p.UpdatedAt)
		})
}

// buildUpdateFields Build the fields that will be updated, validated against the
// stored package feesAmountData describes.
func (uc *UseCase) buildUpdateFields(ctx context.Context, logger libLog.Logger, packageID, organizationID uuid.UUID, feesAmountData *model.AmountData, up *model.UpdatePackageInput) (bson.M, bson.M, error) {
	setFields := bson.M{}
	unsetFields := bson.M{}

	// Update amounts
	if up.MinAmount != nil || up.MaxAmount != nil {
		if errSetAmounts := uc.SetAmountsDataToUpdate(ctx, logger, up, feesAmountData, organizationID, &packageID, setFields); errSetAmounts != nil {
			return nil, nil, errSetAmounts
		}
	}

	// A patch that moves the minimum is judged against the fees the package keeps: a
	// deductible fee larger than the new minimum would leave the package accepting
	// payments too small to charge it on. The fees this patch restates are validated
	// below, against this same new minimum.
	if errStoredFees := up.ValidateStoredFeesAgainstMinimum(feesAmountData.Fees); errStoredFees != nil {
		return nil, nil, errStoredFees
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

	feeCount := len(feesAmountData.Fees)

	// Update fee map
	if up.Fee != nil {
		minAmount, errMinAmount := up.EffectiveMinimumAmount(feesAmountData.MinAmount)
		if errMinAmount != nil {
			return nil, nil, errMinAmount
		}

		var errValidationFeesSet error

		feeCount, errValidationFeesSet = uc.validationFeesSetUnset(ctx, minAmount, organizationID, feesAmountData.LedgerID, feesAmountData.Fees, up.Fee, setFields, unsetFields)
		if errValidationFeesSet != nil {
			return nil, nil, errValidationFeesSet
		}
	}

	if len(setFields) == 0 && len(unsetFields) == 0 {
		return setFields, unsetFields, pkg.ValidateBusinessError(constant.ErrNothingToUpdate, constant.EntityPackage)
	}

	// A package without fees is disabled, whatever the patch asks of enable.
	if feeCount == 0 {
		setFields["enable"] = false
	}

	setFields["updated_at"] = time.Now()

	return setFields, unsetFields, nil
}

// validationFeesSetUnset validates the fee patch, writes it into the set and
// unset fields, and returns how many fees the package holds once it applies.
func (uc *UseCase) validationFeesSetUnset(ctx context.Context, minAmount decimal.Decimal, organizationID, ledgerID uuid.UUID, existingFees map[string]model.Fee, updateFeesEntity map[string]model.Fee, setFields, unsetFields bson.M) (int, error) {
	// The priority each fee holds once the patch is applied: a patch entry that
	// leaves priority out keeps the stored one.
	priorities := make(map[string]int, len(existingFees))
	for key, fee := range existingFees {
		priorities[key] = fee.Priority
	}

	// Process update fees
	for key, fee := range updateFeesEntity {
		keyFormatted := strcase.ToLowerCamel(key)
		_, feeExists := existingFees[keyFormatted]

		if !feeExists {
			// New fee - validate it and set it whole
			err := fee.ValidateNewFee(key, minAmount)
			if err != nil {
				return 0, err
			}

			// Validate that the credit account exists.
			if errGetAccount := uc.resolver.AccountExistsByAlias(ctx, organizationID, ledgerID, fee.CreditAccount); errGetAccount != nil {
				return 0, errGetAccount
			}

			// The converter normalizes the raw key into keyFormatted. Normalization is
			// not idempotent, so it must see the key the client sent.
			mongoFees, errConvert := pack.FromEntityFeeMap(map[string]model.Fee{key: fee})
			if errConvert != nil {
				return 0, errConvert
			}

			setFields["fees."+keyFormatted] = mongoFees[keyFormatted]
			priorities[keyFormatted] = fee.Priority
		} else {
			// Existing fee - check if it's being updated or removed
			hasFieldsToUpdate, errSetFieldsToUpdate := fee.SetAndValidateHasFieldsToUpdate(ctx, fee.IsDeductibleFrom, minAmount, existingFees, keyFormatted, organizationID, ledgerID, setFields, uc.resolver)
			if errSetFieldsToUpdate != nil {
				return 0, errSetFieldsToUpdate
			}

			switch {
			case !hasFieldsToUpdate:
				unsetFields["fees."+keyFormatted] = ""

				delete(priorities, keyFormatted)
			case fee.Priority != 0:
				priorities[keyFormatted] = fee.Priority
			}
		}
	}

	seen := make(map[int]struct{}, len(priorities))
	for _, priority := range priorities {
		if _, taken := seen[priority]; taken {
			return 0, pkg.ValidateBusinessError(constant.ErrPriorityInvalid, "")
		}

		seen[priority] = struct{}{}
	}

	return len(priorities), nil
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
		ctx, logger, maxAmount, minAmount, feesAmountData.GetTransactionRoute(), feesAmountData.MetadataSelector,
		organizationID, feesAmountData.LedgerID, feesAmountData.SegmentID, packageID,
	); errRange != nil {
		return errRange
	}

	return nil
}
