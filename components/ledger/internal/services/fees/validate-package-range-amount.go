// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	http "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ValidatePackageMaxAndMinAmountRange validating max and min amount range of a package
func (uc *UseCase) ValidatePackageMaxAndMinAmountRange(ctx context.Context, logger libLog.Logger,
	maxAmount, minAmount, transactionRoute string,
	organizationID, ledgerID uuid.UUID,
	segmentID, packageID *uuid.UUID,
) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx) //nolint:dogsled // lib-commons API returns 4 values, only tracer needed here

	ctx, span := tracer.Start(ctx, "service.package.validate_amount_range")
	defer span.End()

	filterPackage := getFilterPackage(organizationID, ledgerID, segmentID, transactionRoute)

	packs, err := uc.packageRepo.FindList(ctx, filterPackage)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find package list", err)

		return err
	}

	if len(packs) > 0 {
		newMaxAmount, errMaxAmount := decimal.NewFromString(maxAmount)
		if errMaxAmount != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to convert max amount to decimal", errMaxAmount)

			return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "package.MaxAmount")
		}

		newMinAmount, errMinAmount := decimal.NewFromString(minAmount)
		if errMinAmount != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to convert min amount to decimal", errMinAmount)

			return pkg.ValidateBusinessError(constant.ErrConvertToDecimal, "", "package.MinAmount")
		}

		// Validate if the account exists on midaz
		for _, p := range packs {
			if packageID == nil || p.ID != *packageID {
				// Validate if all package data equals the new package
				if isSamePackage(p, newMinAmount, newMaxAmount, transactionRoute, segmentID) {
					err := pkg.ValidateBusinessError(constant.ErrDuplicatePackage, constant.EntityPackage)
					libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Duplicate package detected", err)

					return err
				}

				// Validate if max and min amount of new package is within the range of a package
				if isRangeOverlap(p, newMinAmount, newMaxAmount, transactionRoute, segmentID) {
					err := pkg.ValidateBusinessError(constant.ErrPackageRange, "")
					libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Package amount range overlap detected", err)

					return err
				}
			}
		}
	}

	return nil
}

func getFilterPackage(organizationID, ledgerID uuid.UUID, segmentID *uuid.UUID, transactionRoute string) http.QueryHeader {
	filter := http.QueryHeader{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
	}

	if segmentID != nil {
		filter.SegmentID = *segmentID
	}

	if transactionRoute != "" {
		filter.TransactionRoute = &transactionRoute
	}

	return filter
}

// isSamePackage validating if all package data is equal to the new package
func isSamePackage(p *pack.Package, newMin, newMax decimal.Decimal, transactionRoute string, segmentID *uuid.UUID) bool {
	if segmentID == nil {
		segmentID = &uuid.Nil
	}

	return p.MaximumAmount.Equal(newMax) &&
		p.MinimumAmount.Equal(newMin) &&
		p.GetSegmentID() == *segmentID &&
		p.GetTransactionRoute() == transactionRoute
}

// isRangeOverlap validating if max and min amount of new package is inside the range of a package
func isRangeOverlap(p *pack.Package, newMin, newMax decimal.Decimal, transactionRoute string, segmentID *uuid.UUID) bool {
	if segmentID == nil {
		segmentID = &uuid.Nil
	}

	return p.GetSegmentID() == *segmentID &&
		p.GetTransactionRoute() == transactionRoute &&
		newMin.LessThanOrEqual(p.MaximumAmount) &&
		newMax.GreaterThanOrEqual(p.MinimumAmount)
}
