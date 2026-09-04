// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"

	"github.com/LerianStudio/lib-commons/v7/commons"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// PackageService defines the package-related business operations consumed by the
// package handler. The interface is defined where it is consumed so the handler
// depends on behavior, not on the concrete fee use case.
//
// The by-ID operations take the ledger the request is scoped to, so a package
// another ledger of the organization owns is out of reach. A uuid.Nil ledger widens
// the reach back to the whole organization, which is why the shells refuse a request
// whose path does not name a ledger.
type PackageService interface {
	CreatePackage(ctx context.Context, cpi *model.CreatePackageInput, organizationID, ledgerID, segmentID uuid.UUID) (*pack.Package, error)
	GetAllPackages(ctx context.Context, filters feehttp.QueryHeader, organizationID uuid.UUID) ([]*pack.Package, error)
	GetPackageByID(ctx context.Context, id, organizationID, ledgerID uuid.UUID) (*pack.Package, error)
	UpdatePackageByID(ctx context.Context, id, organizationID, ledgerID uuid.UUID, up *model.UpdatePackageInput) error
	DeletePackageByID(ctx context.Context, id, organizationID, ledgerID uuid.UUID) error
}

// PackageHandler exposes the fee-package CRUD surface over HTTP.
type PackageHandler struct {
	Service PackageService
}

// createPackage is the transport-agnostic core of the create-package op. It owns the
// span, the segment id parsing, the min/max + fee + duplicate-priority validation, and
// the service call; the shell resolves the org+ledger ids, decodes the payload, and
// renders the returned package/error.
func (handler *PackageHandler) createPackage(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *model.CreatePackageInput) (*pack.Package, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var (
		segmentID    uuid.UUID
		errParseUUID error
	)

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_package")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	if !commons.IsNilOrEmpty(payload.SegmentID) {
		segmentID, errParseUUID = uuid.Parse(*payload.SegmentID)
		if errParseUUID != nil {
			return nil, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidSegmentID, "")
		}
	}

	if errAmount := payload.ValidateMinAndMaxAmount(); errAmount != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid min/max amount validation", errAmount)

		return nil, errAmount
	}

	errValidateInput := payload.ValidateFees()
	if errValidateInput != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Error on validation of input payload: Err ", errValidateInput)

		return nil, errValidateInput
	}

	seenPriorities := make(map[int]bool)
	for _, fee := range payload.Fee {
		if seenPriorities[fee.Priority] {
			return nil, feeerrors.ValidateBusinessError(feeconstant.ErrPriorityInvalid, feeconstant.EntityPackage)
		}

		seenPriorities[fee.Priority] = true
	}

	packOut, err := handler.Service.CreatePackage(ctx, payload, organizationID, ledgerID, segmentID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create pack on command", err)

		return nil, err
	}

	return packOut, nil
}

// getAllPackagesInLedger lists the packages one ledger owns — the ledger-scoped
// surface. The ledger comes from the path and the query cannot restate it: a
// ledgerId key is refused rather than merged, because the only two readings of it
// here are a redundant repetition and a request for a different ledger, and the
// second must not be silently answered as the first. Refusing also keeps the
// listing away from the empty filter, which means every ledger of the organization.
func (handler *PackageHandler) getAllPackagesInLedger(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (model.Pagination, error) {
	if err := ctx.Err(); err != nil {
		return model.Pagination{}, err
	}

	if err := rejectLedgerQueryParameter(queries); err != nil {
		return model.Pagination{}, err
	}

	return handler.listPackages(ctx, organizationID, ledgerID, queries)
}

// listPackages is the transport-agnostic core of the list-packages op. It owns the
// span, the fee-package query validation (feehttp.ValidateParameters — NOT
// pkg/net/http's), the service call, and the pagination envelope assembly. The shell
// resolves the org id from the path and passes the raw query map, then renders the
// envelope/error.
//
// A ledgerID of uuid.Nil leaves the ledger to the query filter. Any other value
// pins the listing to that ledger AFTER the query has been validated, so no query
// the caller sends can widen or redirect it.
func (handler *PackageHandler) listPackages(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (model.Pagination, error) {
	if err := ctx.Err(); err != nil {
		return model.Pagination{}, err
	}

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_package")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	if ledgerID != uuid.Nil {
		span.SetAttributes(attribute.String("app.request.ledger_id", ledgerID.String()))
	}

	headerParams, err := feehttp.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate query parameters")

		return model.Pagination{}, err
	}

	if ledgerID != uuid.Nil {
		headerParams.LedgerID = ledgerID
	}

	span.SetAttributes(
		attribute.Int("app.request.limit", headerParams.Limit),
		attribute.Int("app.request.page", headerParams.Page),
		attribute.Bool("app.request.has_segment_id", headerParams.SegmentID != uuid.Nil),
		attribute.Bool("app.request.has_ledger_id", headerParams.LedgerID != uuid.Nil),
		attribute.Bool("app.request.has_transaction_route", headerParams.TransactionRoute != nil),
		attribute.Bool("app.request.has_enable", headerParams.Enable != nil),
	)

	pagination := model.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	packs, err := handler.Service.GetAllPackages(ctx, *headerParams, organizationID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all Packages on query", err)

		return model.Pagination{}, err
	}

	pagination.SetItems(packs)
	pagination.SetTotal(len(packs))

	return pagination, nil
}

// getPackageByID is the transport-agnostic core of the get-package op. It owns the
// span and the service call; the shell resolves the org+ledger+package ids and
// renders the returned package/error.
func (handler *PackageHandler) getPackageByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*pack.Package, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_package_by_id")
	defer span.End()

	span.SetAttributes(append(
		[]attribute.KeyValue{attribute.String("app.request.request_id", reqId)},
		feeLedgerScopeAttributes(organizationID, ledgerID, "app.request.package_id", id)...,
	)...)

	packModel, err := handler.Service.GetPackageByID(ctx, id, organizationID, ledgerID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve package on query", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to retrieve Package", libLog.String("package_id", id.String()))

		return nil, err
	}

	return packModel, nil
}

// updatePackageByID is the transport-agnostic core of the update-package op. It owns
// the span, the fee + duplicate-priority + min/max validation, the update, and the
// re-read; the shell resolves the org+ledger+package ids, decodes the payload, and
// renders the returned package/error.
func (handler *PackageHandler) updatePackageByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *model.UpdatePackageInput) (*pack.Package, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_package")
	defer span.End()

	span.SetAttributes(append(
		[]attribute.KeyValue{attribute.String("app.request.request_id", reqId)},
		feeLedgerScopeAttributes(organizationID, ledgerID, "app.request.package_id", id)...,
	)...)

	if payload.Fee != nil {
		errValidateInput := payload.ValidateFees()
		if errValidateInput != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Error on validation of input payload: Err ", errValidateInput)

			return nil, errValidateInput
		}

		seenPriorities := make(map[int]bool)

		for _, fee := range payload.Fee {
			if !fee.ValidateIfFeeIsNil() {
				if seenPriorities[fee.Priority] && fee.Priority != 0 {
					return nil, feeerrors.ValidateBusinessError(feeconstant.ErrPriorityInvalid, feeconstant.EntityPackage)
				}

				seenPriorities[fee.Priority] = true
			}
		}
	}

	if errValidateAmount := payload.ValidateMinAndMaxAmount(); errValidateAmount != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid values for maxAmount and minAmount", errValidateAmount)

		return nil, errValidateAmount
	}

	if errUpdate := handler.Service.UpdatePackageByID(ctx, id, organizationID, ledgerID, payload); errUpdate != nil {
		handleSpanByErrorClass(span, "Failed to update package", errUpdate)

		return nil, errUpdate
	}

	packUpdated, err := handler.Service.GetPackageByID(ctx, id, organizationID, ledgerID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve package on query", err)

		return nil, err
	}

	return packUpdated, nil
}

// deletePackageByID is the transport-agnostic core of the delete-package op. It owns
// the span and the service call; the shell resolves the org+ledger+package ids and
// renders the 204/error.
func (handler *PackageHandler) deletePackageByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_package_by_id")
	defer span.End()

	span.SetAttributes(append(
		[]attribute.KeyValue{attribute.String("app.request.request_id", reqId)},
		feeLedgerScopeAttributes(organizationID, ledgerID, "app.request.package_id", id)...,
	)...)

	if err := handler.Service.DeletePackageByID(ctx, id, organizationID, ledgerID); err != nil {
		handleSpanByErrorClass(span, "Failed to remove package on database", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to remove Package", libLog.String("package_id", id.String()))

		return err
	}

	return nil
}
