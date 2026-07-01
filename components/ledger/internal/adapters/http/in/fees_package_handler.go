// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

	"github.com/LerianStudio/lib-commons/v5/commons"
	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// PackageService defines the package-related business operations consumed by the
// package handler. The interface is defined where it is consumed so the handler
// depends on behavior, not on the concrete fee use case.
type PackageService interface {
	CreatePackage(ctx context.Context, cpi *model.CreatePackageInput, organizationID, ledgerID, segmentID uuid.UUID) (*pack.Package, error)
	GetAllPackages(ctx context.Context, filters feehttp.QueryHeader, organizationID uuid.UUID) ([]*pack.Package, error)
	GetPackageByID(ctx context.Context, id, organizationID uuid.UUID) (*pack.Package, error)
	UpdatePackageByID(ctx context.Context, id, organizationID uuid.UUID, up *model.UpdatePackageInput) error
	DeletePackageByID(ctx context.Context, id, organizationID uuid.UUID) error
}

// PackageHandler exposes the fee-package CRUD surface over HTTP.
type PackageHandler struct {
	Service PackageService
}

// CreatePackage is a method that creates Package information.
func (handler *PackageHandler) CreatePackage(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := p.(*model.CreatePackageInput)

	packOut, err := handler.createPackage(ctx, organizationID, payload)
	if err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusCreated, packOut)
}

// createPackage is the transport-agnostic core of the create-package op, shared by
// the Fiber wrapper (CreatePackage) and the Huma shell. It owns the span, the
// segment/ledger id parsing, the min/max + fee + duplicate-priority validation, and
// the service call; the caller resolves the org id, decodes the payload, and renders
// the returned package/error.
func (handler *PackageHandler) createPackage(ctx context.Context, organizationID uuid.UUID, payload *model.CreatePackageInput) (*pack.Package, error) {
	var (
		segmentID    uuid.UUID
		ledgerID     uuid.UUID
		errParseUUID error
	)

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_package")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	if !commons.IsNilOrEmpty(payload.SegmentID) {
		segmentID, errParseUUID = uuid.Parse(*payload.SegmentID)
		if errParseUUID != nil {
			return nil, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidSegmentID, "")
		}
	}

	ledgerID, errParseUUID = uuid.Parse(payload.LedgerID)
	if errParseUUID != nil {
		return nil, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidLedgerID, "")
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

// GetAllPackages is a method that retrieves all Package information.
func (handler *PackageHandler) GetAllPackages(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	pagination, err := handler.getAllPackages(ctx, organizationID, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusOK, pagination)
}

// getAllPackages is the transport-agnostic core of the list-packages op, shared by
// the Fiber wrapper (GetAllPackages) and the Huma shell. It owns the span, the
// fee-package query validation (feehttp.ValidateParameters — NOT pkg/net/http's),
// the service call, and the pagination envelope assembly. The caller resolves the
// org id (Fiber: locals; Huma: path param) and passes the raw query map
// (c.Queries() on Fiber, queriesFromValues(rawQuery) on Huma) so the binder is
// byte-identical, then renders the envelope/error.
func (handler *PackageHandler) getAllPackages(ctx context.Context, organizationID uuid.UUID, queries map[string]string) (model.Pagination, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_package")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	headerParams, err := feehttp.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate query parameters")

		return model.Pagination{}, err
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

// GetPackageByID is a method that retrieves a Package information by a given id.
func (handler *PackageHandler) GetPackageByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	packModel, err := handler.getPackageByID(ctx, organizationID, id)
	if err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusOK, packModel)
}

// getPackageByID is the transport-agnostic core of the get-package op, shared by the
// Fiber wrapper (GetPackageByID) and the Huma shell. It owns the span and the service
// call; the caller resolves the org+package ids and renders the returned package/error.
func (handler *PackageHandler) getPackageByID(ctx context.Context, organizationID, id uuid.UUID) (*pack.Package, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_package_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	packModel, err := handler.Service.GetPackageByID(ctx, id, organizationID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve package on query", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to retrieve Package", libLog.String("package_id", id.String()))

		return nil, err
	}

	return packModel, nil
}

// UpdatePackageByID is a method that updates a Package information by a given id.
func (handler *PackageHandler) UpdatePackageByID(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := p.(*model.UpdatePackageInput)

	packUpdated, err := handler.updatePackageByID(ctx, organizationID, id, payload)
	if err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusOK, packUpdated)
}

// updatePackageByID is the transport-agnostic core of the update-package op, shared
// by the Fiber wrapper (UpdatePackageByID) and the Huma shell. It owns the span, the
// fee + duplicate-priority + min/max validation, the update, and the re-read; the
// caller resolves the org+package ids, decodes the payload, and renders the returned
// package/error.
func (handler *PackageHandler) updatePackageByID(ctx context.Context, organizationID, id uuid.UUID, payload *model.UpdatePackageInput) (*pack.Package, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_package")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

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

	if errUpdate := handler.Service.UpdatePackageByID(ctx, id, organizationID, payload); errUpdate != nil {
		handleSpanByErrorClass(span, "Failed to update package", errUpdate)

		return nil, errUpdate
	}

	packUpdated, err := handler.Service.GetPackageByID(ctx, id, organizationID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve package on query", err)

		return nil, err
	}

	return packUpdated, nil
}

// DeletePackageByID is a method that removes a package information by a given id.
func (handler *PackageHandler) DeletePackageByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	if err := handler.deletePackageByID(ctx, organizationID, id); err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.RespondStatus(c, fiber.StatusNoContent)
}

// deletePackageByID is the transport-agnostic core of the delete-package op, shared
// by the Fiber wrapper (DeletePackageByID) and the Huma shell. It owns the span and
// the service call; the caller resolves the org+package ids and renders the 204/error.
func (handler *PackageHandler) deletePackageByID(ctx context.Context, organizationID, id uuid.UUID) error {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_package_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	if err := handler.Service.DeletePackageByID(ctx, id, organizationID); err != nil {
		handleSpanByErrorClass(span, "Failed to remove package on database", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to remove Package", libLog.String("package_id", id.String()))

		return err
	}

	return nil
}
