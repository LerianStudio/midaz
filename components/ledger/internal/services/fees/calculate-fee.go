// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability"

	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	feeUtils "github.com/LerianStudio/midaz/v4/components/ledger/pkg/fee"
	feeshared "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	transaction "github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CalculateFee creates a new pack persists data in the repository.
func (uc *UseCase) CalculateFee(ctx context.Context, cf *model.FeeCalculate, organizationID uuid.UUID) (err error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	// Defensive nil check for the main input parameter
	if cf == nil {
		return pkg.ValidateBusinessError(constant.ErrCalculateFee, "")
	}

	ctx, span := tracer.Start(ctx, "service.calculate_fee")
	defer span.End()

	start := time.Now()

	defer func() {
		utils.RecordDomainOperation(ctx, uc.MetricsFactory, logger, "fees", "calculate_fee", start, err)
	}()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	span.SetAttributes(
		attribute.String("app.request.ledger_id", cf.LedgerID.String()),
	)

	packages, err := uc.findPackagesCached(ctx, logger, span, organizationID, cf.LedgerID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to find packages by organization and ledger", err)

		return err
	}

	if len(packages) == 0 {
		return nil
	}

	validationResult, errValidationSend := transaction.ValidateSendSourceAndDistribute(ctx, cf.Transaction, "")
	if errValidationSend != nil {
		bizErr := pkg.ValidateBusinessError(constant.ErrValidateDistributeTransactionValue, "")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate send struct", bizErr)

		return bizErr
	}

	sendModel := cf.Transaction.Send
	validationResultToSize := len(validationResult.To)
	validationResultFromSize := len(validationResult.From)

	// Populate the source segment so segment-scoped packages can match. Callers
	// that set cf.SegmentID explicitly (e.g. the estimate endpoint) keep their
	// value; only the JSON-transaction seam, which leaves it nil, gets resolution.
	if cf.SegmentID == nil {
		cf.SegmentID = uc.resolveSourceSegment(ctx, span, logger, validationResult.Sources, organizationID, cf.LedgerID)
	}

	if len(packages) == 1 {
		return uc.calculateFeeForSinglePackage(ctx, logger, cf, packages[0], sendModel, validationResult, validationResultFromSize, validationResultToSize, organizationID)
	}

	return uc.calculateFeeForMultiplePackages(ctx, logger, cf, packages, sendModel, validationResult, validationResultFromSize, validationResultToSize, organizationID)
}

// resolveSourceSegment resolves the segment shared by the transaction's real
// source accounts so segment-scoped fee packages can match. It returns the
// segment when every resolvable, non-external source shares exactly one segment,
// and nil when there is no segment, the sources span more than one segment, or a
// source cannot be resolved. A nil result is the conservative, fee-neutral path:
// it falls back to unscoped packages exactly as before, never guessing which
// segment of a mixed-segment source set should drive a money calculation.
//
// External sources (@external/*) are virtual accounts with no segment and are
// skipped. A resolver read failure is logged Warn and yields nil rather than
// failing the transaction over a fee-scoping read.
func (uc *UseCase) resolveSourceSegment(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	sources []string,
	organizationID, ledgerID uuid.UUID,
) *uuid.UUID {
	if uc.resolver == nil {
		return nil
	}

	var resolved *uuid.UUID

	for _, source := range sources {
		// validationResult.Sources entries are balance-key-decorated as
		// "<alias>#<balanceKey>"; the resolver keys on the bare alias, so strip
		// the suffix before the lookup.
		alias := strings.TrimSpace(source)
		if idx := strings.Index(alias, "#"); idx != -1 {
			alias = alias[:idx]
		}

		if alias == "" || strings.HasPrefix(alias, "@external/") {
			continue
		}

		account, err := uc.resolver.GetAccountByAlias(ctx, organizationID, ledgerID, alias)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to resolve source account segment for fee scoping; treating as unscoped",
				libLog.Err(err))
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to resolve source account segment for fee scoping", err)

			return nil
		}

		if account == nil || account.SegmentID == nil {
			// An unsegmented (or absent) source means the source set is not wholly
			// within a single segment: do not apply a segment-scoped package.
			return nil
		}

		if resolved == nil {
			seg := *account.SegmentID
			resolved = &seg

			continue
		}

		if *resolved != *account.SegmentID {
			// Sources span more than one segment — ambiguous; leave unscoped.
			return nil
		}
	}

	return resolved
}

// calculateFeeForSinglePackage calculate the fee for a single package
func (uc *UseCase) calculateFeeForSinglePackage(
	ctx context.Context,
	logger libLog.Logger,
	cf *model.FeeCalculate,
	feePackage *pack.Package,
	sendModel transaction.Send,
	validationResult *transaction.Responses,
	validationResultFromSize, validationResultToSize int,
	organizationID uuid.UUID,
) error {
	// Route the sole package through the same scope filter the multi-package
	// path uses so a single SCOPED package (route and/or segment) is applied only
	// when its scope matches the transaction. An unscoped single package (nil
	// route, nil segment) still survives every filter and is selected as before.
	packFilter, errFilterPack := feeUtils.FindPackageToCalculateFee([]*pack.Package{feePackage}, cf.Transaction.Route, cf.SegmentID, sendModel.Value) //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
	if errFilterPack != nil {
		return pkg.ValidateBusinessError(constant.ErrFilterPackage, "")
	}

	if packFilter == nil {
		return nil
	}

	if !sendModel.Value.GreaterThanOrEqual(packFilter.MinimumAmount) || !sendModel.Value.LessThanOrEqual(packFilter.MaximumAmount) {
		return nil
	}

	segCtx := &feeUtils.SegmentContext{
		Ctx:            ctx,
		Resolver:       uc.resolver,
		OrganizationID: organizationID,
		LedgerID:       cf.LedgerID,
		ResolverCache:  make(map[string]*feeshared.Account),
	}

	errCalculateFee := feeUtils.CalculateFee(logger, cf, packFilter, validationResult, uc.defaultCurrency, segCtx)
	if errCalculateFee != nil {
		return errCalculateFee
	}

	uc.updateFeeMetadataIfNeeded(cf, validationResult, validationResultFromSize, validationResultToSize, packFilter.ID)

	return nil
}

func (uc *UseCase) calculateFeeForMultiplePackages(
	ctx context.Context,
	logger libLog.Logger,
	cf *model.FeeCalculate,
	packages []*pack.Package,
	sendModel transaction.Send,
	validationResult *transaction.Responses,
	validationResultFromSize, validationResultToSize int,
	organizationID uuid.UUID,
) error {
	packFilter, errFilterPack := feeUtils.FindPackageToCalculateFee(packages, cf.Transaction.Route, cf.SegmentID, sendModel.Value) //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical
	if errFilterPack != nil {
		return pkg.ValidateBusinessError(constant.ErrFilterPackage, "")
	}

	if packFilter == nil {
		return nil
	}

	if !sendModel.Value.GreaterThanOrEqual(packFilter.MinimumAmount) || !sendModel.Value.LessThanOrEqual(packFilter.MaximumAmount) {
		return nil
	}

	segCtx := &feeUtils.SegmentContext{
		Ctx:            ctx,
		Resolver:       uc.resolver,
		OrganizationID: organizationID,
		LedgerID:       cf.LedgerID,
		ResolverCache:  make(map[string]*feeshared.Account),
	}

	errCalculateFee := feeUtils.CalculateFee(logger, cf, packFilter, validationResult, uc.defaultCurrency, segCtx)
	if errCalculateFee != nil {
		return errCalculateFee
	}

	uc.updateFeeMetadataIfNeeded(cf, validationResult, validationResultFromSize, validationResultToSize, packFilter.ID)

	return nil
}

func (uc *UseCase) updateFeeMetadataIfNeeded(
	cf *model.FeeCalculate,
	validationResult *transaction.Responses,
	validationResultFromSize, validationResultToSize int,
	packageID uuid.UUID,
) {
	feeApplied := len(validationResult.From) != validationResultFromSize ||
		len(validationResult.To) != validationResultToSize
	_, hasExemption := cf.Transaction.Metadata["feeExemption"]

	if feeApplied || hasExemption {
		if cf.Transaction.Metadata == nil {
			cf.Transaction.Metadata = make(map[string]any)
		}

		cf.Transaction.Metadata["packageAppliedID"] = packageID.String()
	}
}
