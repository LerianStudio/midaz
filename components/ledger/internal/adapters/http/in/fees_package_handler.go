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
//
//	@Summary		Create a Package
//	@Description	Create a Package with the input payload
//	@Tags			Packages
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string						false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			organization_id		path		string						true	"The unique identifier of the Organization."
//	@Param			pack				body		model.CreatePackageInput	true	"Package Input"
//	@Success		201					{object}	pack.Package
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		409					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/packages [post]
func (handler *PackageHandler) CreatePackage(p any, c *fiber.Ctx) error {
	var (
		segmentID    uuid.UUID
		ledgerID     uuid.UUID
		errParseUUID error
	)

	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_package")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	payload := p.(*model.CreatePackageInput)

	if !commons.IsNilOrEmpty(payload.SegmentID) {
		segmentID, errParseUUID = uuid.Parse(*payload.SegmentID)
		if errParseUUID != nil {
			return http.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidSegmentID, ""))
		}
	}

	ledgerID, errParseUUID = uuid.Parse(payload.LedgerID)
	if errParseUUID != nil {
		return http.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidLedgerID, ""))
	}

	if errAmount := payload.ValidateMinAndMaxAmount(); errAmount != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid min/max amount validation", errAmount)

		return http.WithError(c, errAmount)
	}

	errValidateInput := payload.ValidateFees()
	if errValidateInput != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Error on validation of input payload: Err ", errValidateInput)

		return http.WithError(c, errValidateInput)
	}

	seenPriorities := make(map[int]bool)
	for _, fee := range payload.Fee {
		if seenPriorities[fee.Priority] {
			return http.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrPriorityInvalid, feeconstant.EntityPackage))
		}

		seenPriorities[fee.Priority] = true
	}

	packOut, err := handler.Service.CreatePackage(ctx, payload, organizationID, ledgerID, segmentID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create pack on command", err)

		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusCreated, packOut)
}

// GetAllPackages is a method that retrieves all Package information.
//
//	@Summary		Get all packages
//	@Description	List all the packages
//	@Tags			Packages
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			organization_id		path		string	true	"The unique identifier of the Organization."
//	@Param			segmentId			query		string	false	"Segment ID"
//	@Param			ledgerId			query		string	false	"Ledger ID"
//	@Param			transactionRoute	query		string	false	"Transaction Route"
//	@Param			enable				query		bool	false	"Enable flag"
//	@Param			limit				query		int		false	"Limit"	default(10)
//	@Param			page				query		int		false	"Page"	default(1)
//	@Success		200					{object}	model.Pagination{items=[]pack.Package,page=int,limit=int,total=int}
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/packages [get]
func (handler *PackageHandler) GetAllPackages(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_package")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	headerParams, err := feehttp.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate query parameters")

		return http.WithError(c, err)
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

		return http.WithError(c, err)
	}

	pagination.SetItems(packs)
	pagination.SetTotal(len(packs))

	return commonsHttp.Respond(c, fiber.StatusOK, pagination)
}

// GetPackageByID is a method that retrieves a Package information by a given id.
//
//	@Summary		Get package
//	@Description	Get a package by id
//	@Tags			Packages
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			organization_id		path		string	true	"The unique identifier of the Organization."
//	@Param			id					path		string	true	"Package ID"
//	@Success		200					{object}	pack.Package
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/packages/{id} [get]
func (handler *PackageHandler) GetPackageByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_package_by_id")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	packModel, err := handler.Service.GetPackageByID(ctx, id, organizationID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve package on query", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to retrieve Package", libLog.String("package_id", id.String()))

		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusOK, packModel)
}

// UpdatePackageByID is a method that updates a Package information by a given id.
//
//	@Summary		Update a package
//	@Description	Update a package with the input payload
//	@Tags			Packages
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string						false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			organization_id		path		string						true	"The unique identifier of the Organization."
//	@Param			id					path		string						true	"Package ID"
//	@Param			package				body		model.UpdatePackageInput	true	"Update Package Input"
//	@Success		200					{object}	pack.Package
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/packages/{id} [patch]
func (handler *PackageHandler) UpdatePackageByID(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_package")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	payload := p.(*model.UpdatePackageInput)

	if payload.Fee != nil {
		errValidateInput := payload.ValidateFees()
		if errValidateInput != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Error on validation of input payload: Err ", errValidateInput)

			return http.WithError(c, errValidateInput)
		}

		seenPriorities := make(map[int]bool)

		for _, fee := range payload.Fee {
			if !fee.ValidateIfFeeIsNil() {
				if seenPriorities[fee.Priority] && fee.Priority != 0 {
					return http.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrPriorityInvalid, feeconstant.EntityPackage))
				}

				seenPriorities[fee.Priority] = true
			}
		}
	}

	if errValidateAmount := payload.ValidateMinAndMaxAmount(); errValidateAmount != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid values for maxAmount and minAmount", errValidateAmount)

		return http.WithError(c, errValidateAmount)
	}

	if errUpdate := handler.Service.UpdatePackageByID(ctx, id, organizationID, payload); errUpdate != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update package", errUpdate)

		return http.WithError(c, errUpdate)
	}

	packUpdated, err := handler.Service.GetPackageByID(ctx, id, organizationID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve package on query", err)

		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusOK, packUpdated)
}

// DeletePackageByID is a method that removes a package information by a given id.
//
//	@Summary		SoftDelete a Package by ID
//	@Description	SoftDelete a Package with the input ID
//	@Tags			Packages
//	@Param			Authorization		header	string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			organization_id		path	string	true	"The unique identifier of the Organization."
//	@Param			id					path	string	true	"Package ID"
//	@Success		204
//	@Failure		400	{object}	mmodel.Error
//	@Failure		401	{object}	mmodel.Error
//	@Failure		403	{object}	mmodel.Error
//	@Failure		404	{object}	mmodel.Error
//	@Failure		500	{object}	mmodel.Error
//	@Router			/v1/organizations/{organization_id}/packages/{id} [delete]
func (handler *PackageHandler) DeletePackageByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_package_by_id")
	defer span.End()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	if err := handler.Service.DeletePackageByID(ctx, id, organizationID); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to remove package on database", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to remove Package", libLog.String("package_id", id.String()))

		return http.WithError(c, err)
	}

	return commonsHttp.RespondStatus(c, fiber.StatusNoContent)
}
