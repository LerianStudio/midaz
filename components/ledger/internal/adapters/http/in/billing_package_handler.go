// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"strconv"

	libObservability "github.com/LerianStudio/lib-observability"

	feeerrors "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared"
	feeconstant "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/constant"
	"github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/model"
	feehttp "github.com/LerianStudio/midaz/v3/components/ledger/pkg/feeshared/nethttp"

	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// BillingPackageUseCase defines the billing-package business operations consumed
// by the billing-package handler.
type BillingPackageUseCase interface {
	CreateBillingPackage(ctx context.Context, bp *model.BillingPackage) (*model.BillingPackage, error)
	GetBillingPackageByID(ctx context.Context, id, organizationID string) (*model.BillingPackage, error)
	GetAllBillingPackages(ctx context.Context, organizationID, ledgerID, billingType string, limit, page int) ([]*model.BillingPackage, int64, error)
	UpdateBillingPackage(ctx context.Context, id, organizationID string, updates map[string]any) (*model.BillingPackage, error)
	DeleteBillingPackage(ctx context.Context, id, organizationID string) error
}

// BillingPackageHandler exposes the billing-package CRUD surface over HTTP.
type BillingPackageHandler struct {
	Service BillingPackageUseCase
}

// CreateBillingPackage is a method that creates a BillingPackage.
//
//	@Summary		Create a BillingPackage
//	@Description	Create a BillingPackage with the input payload
//	@Tags			BillingPackages
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string					false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string					true	"The unique identifier of the Organization."
//	@Param			billingPackage		body		model.BillingPackage	true	"BillingPackage Input"
//	@Success		201					{object}	model.BillingPackage
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		409					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/billing-packages [post]
func (handler *BillingPackageHandler) CreateBillingPackage(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_billing_package")
	defer span.End()

	orgVal, orgOK := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)
	if !orgOK {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrHeaderParameterRequired, "", feeOrgIDHeaderParameter))
	}

	organizationID := orgVal

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	payload := p.(*model.BillingPackage)
	payload.OrganizationID = organizationID.String()
	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Request to create billing package: type=%s", payload.Type))

	span.SetAttributes(
		attribute.String("app.request.payload.type", payload.Type),
		attribute.String("app.request.payload.label", payload.Label),
		attribute.String("app.request.payload.ledger_id", payload.LedgerID),
		attribute.Bool("app.request.payload.has_enable", payload.Enable != nil),
		attribute.Bool("app.request.payload.enable", payload.Enable != nil && *payload.Enable),
	)

	result, errCreate := handler.Service.CreateBillingPackage(ctx, payload)
	if errCreate != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create billing package", errCreate)

		return feehttp.WithError(c, errCreate)
	}

	if result == nil {
		return feehttp.WithError(c, fmt.Errorf("service returned nil result without error"))
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully created billing package: %v", result.ID))

	return commonsHttp.Respond(c, fiber.StatusCreated, result)
}

// GetAllBillingPackages is a method that retrieves all BillingPackages.
//
//	@Summary		Get all billing packages
//	@Description	List all billing packages
//	@Tags			BillingPackages
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string	true	"The unique identifier of the Organization."
//	@Param			ledgerId			query		string	false	"Ledger ID (optional — omit to list all packages for the organization)"
//	@Param			type				query		string	false	"Filter by billing package type (volume or maintenance)"
//	@Param			limit				query		int		false	"Limit"	default(10)
//	@Param			page				query		int		false	"Page"	default(1)
//	@Success		200					{object}	model.Pagination{items=[]model.BillingPackage,page=int,limit=int,total=int}
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/billing-packages [get]
func (handler *BillingPackageHandler) GetAllBillingPackages(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_billing_packages")
	defer span.End()

	orgVal, orgOK := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)
	if !orgOK {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrHeaderParameterRequired, "", feeOrgIDHeaderParameter))
	}

	organizationID := orgVal

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	ledgerIDParam := c.Query("ledgerId")
	if ledgerIDParam != "" {
		if _, errParse := uuid.Parse(ledgerIDParam); errParse != nil {
			err := feeerrors.ValidateBusinessError(feeconstant.ErrInvalidQueryParameter, "", "ledgerId")
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ledgerId query parameter", err)

			return feehttp.WithError(c, err)
		}
	}

	limit := 10
	page := 1

	if l := c.Query("limit"); l != "" {
		parsed, errParse := strconv.Atoi(l)
		if errParse != nil || parsed < 1 {
			validationErr := feeerrors.ValidateBusinessError(feeconstant.ErrInvalidQueryParameter, "BillingPackage", "limit")
			return feehttp.WithError(c, validationErr)
		}

		limit = parsed
	}

	if p := c.Query("page"); p != "" {
		parsed, errParse := strconv.Atoi(p)
		if errParse != nil || parsed < 1 {
			validationErr := feeerrors.ValidateBusinessError(feeconstant.ErrInvalidQueryParameter, "BillingPackage", "page")
			return feehttp.WithError(c, validationErr)
		}

		page = parsed
	}

	billingType := c.Query("type")

	span.SetAttributes(
		attribute.String("app.request.ledger_id", ledgerIDParam),
		attribute.String("app.request.billing_type", billingType),
		attribute.Int("app.request.limit", limit),
		attribute.Int("app.request.page", page),
	)

	results, total, errGet := handler.Service.GetAllBillingPackages(ctx, organizationID.String(), ledgerIDParam, billingType, limit, page)
	if errGet != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all billing packages", errGet)

		return feehttp.WithError(c, errGet)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully retrieved all billing packages: count=%d, total=%d", len(results), total))

	pagination := model.Pagination{
		Limit: limit,
		Page:  page,
	}

	pagination.SetItems(results)
	pagination.SetTotal(int(total))

	return commonsHttp.Respond(c, fiber.StatusOK, pagination)
}

// GetBillingPackageByID is a method that retrieves a BillingPackage by ID.
//
//	@Summary		Get billing package
//	@Description	Get a billing package by id
//	@Tags			BillingPackages
//	@Produce		json
//	@Param			Authorization		header		string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string	true	"The unique identifier of the Organization."
//	@Param			id					path		string	true	"BillingPackage ID"
//	@Success		200					{object}	model.BillingPackage
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/billing-packages/{id} [get]
func (handler *BillingPackageHandler) GetBillingPackageByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_billing_package_by_id")
	defer span.End()

	orgVal, orgOK := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)
	if !orgOK {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrHeaderParameterRequired, "", feeOrgIDHeaderParameter))
	}

	idVal, idOK := c.Locals(feeUUIDPathParameter).(uuid.UUID)
	if !idOK {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidPathParameter, "", "id"))
	}

	organizationID := orgVal
	id := idVal

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.billing_package_id", id.String()),
	)

	result, errGet := handler.Service.GetBillingPackageByID(ctx, id.String(), organizationID.String())
	if errGet != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve billing package", errGet)

		logger.Log(ctx, libLog.LevelWarn, "Failed to retrieve BillingPackage", libLog.String("billing_package_id", id.String()))

		return feehttp.WithError(c, errGet)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully retrieved BillingPackage with ID: %s", id.String()))

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}

// UpdateBillingPackage is a method that updates a BillingPackage by ID.
//
//	@Summary		Update a billing package
//	@Description	Update a billing package with the input payload
//	@Tags			BillingPackages
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string						false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header		string						true	"The unique identifier of the Organization."
//	@Param			id					path		string						true	"BillingPackage ID"
//	@Param			billingPackage		body		model.BillingPackageUpdate	true	"Update BillingPackage Input"
//	@Success		200					{object}	model.BillingPackage
//	@Failure		400					{object}	mmodel.Error
//	@Failure		401					{object}	mmodel.Error
//	@Failure		403					{object}	mmodel.Error
//	@Failure		404					{object}	mmodel.Error
//	@Failure		409					{object}	mmodel.Error
//	@Failure		500					{object}	mmodel.Error
//	@Router			/v1/billing-packages/{id} [patch]
func (handler *BillingPackageHandler) UpdateBillingPackage(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_billing_package")
	defer span.End()

	orgVal, orgOK := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)
	if !orgOK {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrHeaderParameterRequired, "", feeOrgIDHeaderParameter))
	}

	idVal, idOK := c.Locals(feeUUIDPathParameter).(uuid.UUID)
	if !idOK {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidPathParameter, "", "id"))
	}

	organizationID := orgVal
	id := idVal

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.billing_package_id", id.String()),
	)

	payload := p.(*model.BillingPackageUpdate)
	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Request to update billing package: id=%s, fields=%d", id.String(), len(payload.ToMap())))

	span.SetAttributes(
		attribute.Bool("app.request.payload.has_label", payload.Label != nil),
		attribute.Bool("app.request.payload.has_description", payload.Description != nil),
		attribute.Bool("app.request.payload.has_enable", payload.Enable != nil),
	)

	if validationErr := payload.Validate(); validationErr != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid update payload", validationErr)

		return feehttp.WithError(c, validationErr)
	}

	updates := payload.ToMap()
	if len(updates) == 0 {
		validationErr := feeerrors.ValidateBusinessError(feeconstant.ErrNothingToUpdate, "BillingPackage")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty update payload", validationErr)

		return feehttp.WithError(c, validationErr)
	}

	result, errUpdate := handler.Service.UpdateBillingPackage(ctx, id.String(), organizationID.String(), updates)
	if errUpdate != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update billing package", errUpdate)

		return feehttp.WithError(c, errUpdate)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully updated BillingPackage with ID: %s", id.String()))

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}

// DeleteBillingPackage is a method that soft-deletes a BillingPackage by ID.
//
//	@Summary		SoftDelete a BillingPackage by ID
//	@Description	SoftDelete a BillingPackage with the input ID
//	@Tags			BillingPackages
//	@Param			Authorization		header	string	false	"The authorization token in the 'Bearer	access_token' format. Only required when auth plugin is enabled."
//	@Param			X-Organization-Id	header	string	true	"The unique identifier of the Organization."
//	@Param			id					path	string	true	"BillingPackage ID"
//	@Success		204
//	@Failure		400	{object}	mmodel.Error
//	@Failure		401	{object}	mmodel.Error
//	@Failure		403	{object}	mmodel.Error
//	@Failure		404	{object}	mmodel.Error
//	@Failure		500	{object}	mmodel.Error
//	@Router			/v1/billing-packages/{id} [delete]
func (handler *BillingPackageHandler) DeleteBillingPackage(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_billing_package")
	defer span.End()

	orgVal, orgOK := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)
	if !orgOK {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrHeaderParameterRequired, "", feeOrgIDHeaderParameter))
	}

	idVal, idOK := c.Locals(feeUUIDPathParameter).(uuid.UUID)
	if !idOK {
		return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidPathParameter, "", "id"))
	}

	organizationID := orgVal
	id := idVal

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.billing_package_id", id.String()),
	)

	if errDelete := handler.Service.DeleteBillingPackage(ctx, id.String(), organizationID.String()); errDelete != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete billing package", errDelete)

		logger.Log(ctx, libLog.LevelWarn, "Failed to remove BillingPackage", libLog.String("billing_package_id", id.String()))

		return feehttp.WithError(c, errDelete)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully removed BillingPackage with ID: %s", id.String()))

	return commonsHttp.RespondStatus(c, fiber.StatusNoContent)
}
