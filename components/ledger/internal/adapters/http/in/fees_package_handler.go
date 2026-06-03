// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"reflect"

	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/mongodb/fees/pack"
	feeerrors "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	feeconstant "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"

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
//	@Param			X-Organization-Id	header		string						true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			pack				body		model.CreatePackageInput	true	"Package Input"
//	@Success		201					{object}	pack.Package
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		409					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/packages [post]
func (handler *PackageHandler) CreatePackage(p any, c *fiber.Ctx) error {
	var (
		segmentID    uuid.UUID
		ledgerID     uuid.UUID
		errParseUUID error
	)

	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_package")
	defer span.End()

	organizationID := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	payload := p.(*model.CreatePackageInput)
	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Request to create a pack with details: %#v", payload))

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", payload, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert payload to JSON string", err)
	}

	if !commons.IsNilOrEmpty(payload.SegmentID) {
		segmentID, errParseUUID = uuid.Parse(*payload.SegmentID)
		if errParseUUID != nil {
			return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidSegmentID, ""))
		}
	}

	ledgerID, errParseUUID = uuid.Parse(payload.LedgerID)
	if errParseUUID != nil {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidLedgerID, ""))
	}

	if errAmount := payload.ValidateMinAndMaxAmount(); errAmount != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid min/max amount validation", errAmount)

		return feehttp.WithError(c, errAmount)
	}

	errValidateInput := payload.ValidateFees()
	if errValidateInput != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Error on validation of input payload: Err ", errValidateInput)

		return feehttp.WithError(c, errValidateInput)
	}

	seenPriorities := make(map[int]bool)
	for _, fee := range payload.Fee {
		if seenPriorities[fee.Priority] {
			return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrPriorityInvalid, reflect.TypeOf(pack.Fee{}).Name()))
		}

		seenPriorities[fee.Priority] = true
	}

	packOut, err := handler.Service.CreatePackage(ctx, payload, organizationID, ledgerID, segmentID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create pack on command", err)

		return feehttp.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully created pack: %v", packOut))

	return commonsHttp.Respond(c, fiber.StatusCreated, packOut)
}

// GetAllPackages is a method that retrieves all Package information.
//
//	@Summary		Get all packages
//	@Description	List all the packages
//	@Tags			Packages
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string	true	"The unique identifier of the Organization associated with the Ledger."
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
//	@Router			/v1/packages [get]
func (handler *PackageHandler) GetAllPackages(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_package")
	defer span.End()

	organizationID := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	headerParams, err := feehttp.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate query parameters")

		return feehttp.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.query_params", headerParams, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert query_params to JSON string", err)
	}

	pagination := model.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	packs, err := handler.Service.GetAllPackages(ctx, *headerParams, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Packages on query", err)

		return feehttp.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, "Successfully retrieved all Packages")

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
//	@Param			X-Organization-Id	header		string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			id					path		string	true	"Package ID"
//	@Success		200					{object}	pack.Package
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/packages/{id} [get]
func (handler *PackageHandler) GetPackageByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_package_by_id")
	defer span.End()

	organizationID := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)
	id := c.Locals(feeUUIDPathParameter).(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	packModel, err := handler.Service.GetPackageByID(ctx, id, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve package on query", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to retrieve Package", libLog.String("package_id", id.String()))

		return feehttp.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully retrieved Package with ID: %s", id.String()))

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
//	@Param			X-Organization-Id	header		string						true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			id					path		string						true	"Package ID"
//	@Param			package				body		model.UpdatePackageInput	true	"Update Package Input"
//	@Success		200					{object}	pack.Package
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/packages/{id} [patch]
func (handler *PackageHandler) UpdatePackageByID(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_package")
	defer span.End()

	organizationID := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)
	id := c.Locals(feeUUIDPathParameter).(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	payload := p.(*model.UpdatePackageInput)
	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Request to update a package: %#v", payload))

	err := libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.payload", payload, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert payload to JSON string", err)
	}

	if payload.Fee != nil {
		errValidateInput := payload.ValidateFees()
		if errValidateInput != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Error on validation of input payload: Err ", errValidateInput)

			return feehttp.WithError(c, errValidateInput)
		}

		seenPriorities := make(map[int]bool)

		for _, fee := range payload.Fee {
			if !fee.ValidateIfFeeIsNil() {
				if seenPriorities[fee.Priority] && fee.Priority != 0 {
					return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrPriorityInvalid, reflect.TypeOf(pack.Fee{}).Name()))
				}

				seenPriorities[fee.Priority] = true
			}
		}
	}

	if errValidateAmount := payload.ValidateMinAndMaxAmount(); errValidateAmount != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid values for maxAmount and minAmount", errValidateAmount)

		return feehttp.WithError(c, errValidateAmount)
	}

	if errUpdate := handler.Service.UpdatePackageByID(ctx, id, organizationID, payload); errUpdate != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update package", errUpdate)

		return feehttp.WithError(c, errUpdate)
	}

	packUpdated, err := handler.Service.GetPackageByID(ctx, id, organizationID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve package on query", err)

		return feehttp.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully updated Package with ID: %s", id))

	return commonsHttp.Respond(c, fiber.StatusOK, packUpdated)
}

// DeletePackageByID is a method that removes a package information by a given id.
//
//	@Summary		SoftDelete a Package by ID
//	@Description	SoftDelete a Package with the input ID
//	@Tags			Packages
//	@Param			Authorization		header	string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header	string	true	"The unique identifier of the Organization associated with the Ledger."
//	@Param			id					path	string	true	"Package ID"
//	@Success		204
//	@Failure		400	{object}	mmodel.Error
//	@Failure		401	{object}	mmodel.Error
//	@Failure		403	{object}	mmodel.Error
//	@Failure		404	{object}	mmodel.Error
//	@Failure		500	{object}	mmodel.Error
//	@Router			/v1/packages/{id} [delete]
func (handler *PackageHandler) DeletePackageByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_package_by_id")
	defer span.End()

	organizationID := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)
	id := c.Locals(feeUUIDPathParameter).(uuid.UUID)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.package_id", id.String()),
	)

	if err := handler.Service.DeletePackageByID(ctx, id, organizationID); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to remove package on database", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to remove Package", libLog.String("package_id", id.String()))

		return feehttp.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully removed Package with ID: %s", id.String()))

	return commonsHttp.RespondStatus(c, fiber.StatusNoContent)
}
