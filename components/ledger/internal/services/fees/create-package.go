// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"

	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel/attribute"
)

// CreatePackage creates a new pack persists data in the repository.
func (uc *UseCase) CreatePackage(ctx context.Context, cpi *model.CreatePackageInput, organizationID, ledgerID, segmentID uuid.UUID) (*pack.Package, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "service.create_package")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	newSegmentID := &segmentID
	if segmentID.ID() == 0 {
		newSegmentID = nil
	}

	// validating existence of an account on midaz
	if errAccountOnMidaz := uc.validateExistenceOfAccountOnMidaz(ctx, *cpi, organizationID, ledgerID); errAccountOnMidaz != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate existence of an account on midaz", errAccountOnMidaz)

		return nil, errAccountOnMidaz
	}

	if errRange := uc.ValidatePackageMaxAndMinAmountRange(ctx, logger, cpi.MaxAmount, cpi.MinAmount, cpi.GetTransactionRoute(), organizationID, ledgerID, newSegmentID, nil); errRange != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate package max and min amount range", errRange)

		return nil, errRange
	}

	minAmount, errMinDec := decimal.NewFromString(cpi.MinAmount)
	if errMinDec != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse minAmount decimal", errMinDec)

		return nil, pkg.ValidateBusinessError(constant.ErrConvertToDecimal, constant.EntityPackage, "minimumAmount")
	}

	maxAmount, errMaxDec := decimal.NewFromString(cpi.MaxAmount)
	if errMaxDec != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to parse maxAmount decimal", errMaxDec)

		return nil, pkg.ValidateBusinessError(constant.ErrConvertToDecimal, constant.EntityPackage, "maximumAmount")
	}

	packModel, errNewPkg := pack.NewPackage(organizationID, ledgerID, cpi.FeeGroupLabel, minAmount, maxAmount, cpi.Fee, cpi.Enable)
	if errNewPkg != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create package entity", errNewPkg)

		return nil, errNewPkg
	}

	// Set optional fields not covered by the NewPackage constructor
	packModel.Description = cpi.Description
	packModel.SegmentID = newSegmentID
	packModel.TransactionRoute = cpi.TransactionRoute
	packModel.WaivedAccounts = cpi.WaivedAccounts

	resultPackModel, err := uc.packageRepo.Create(ctx, packModel, organizationID)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			bizErr := pkg.ValidateBusinessError(constant.ErrDuplicatePackage, constant.EntityPackage)
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Duplicate package on create", bizErr)

			return nil, bizErr
		}

		libOpentelemetry.HandleSpanError(span, "Failed to create package on repo", err)

		return nil, err
	}

	return resultPackModel, nil
}

// validateExistenceOfAccountOnMidaz validates that all credit accounts referenced by fees exist on Midaz.
// It deduplicates aliases to avoid redundant HTTP calls (N+1 query prevention).
func (uc *UseCase) validateExistenceOfAccountOnMidaz(ctx context.Context, cpi model.CreatePackageInput, organizationID, ledgerID uuid.UUID) error {
	uniqueAliases := make(map[string]struct{})

	for _, fee := range cpi.Fee {
		uniqueAliases[fee.CreditAccount] = struct{}{}
	}

	for alias := range uniqueAliases {
		if errGetAccount := uc.resolver.AccountExistsByAlias(ctx, organizationID, ledgerID, alias); errGetAccount != nil {
			return errGetAccount
		}
	}

	return nil
}
