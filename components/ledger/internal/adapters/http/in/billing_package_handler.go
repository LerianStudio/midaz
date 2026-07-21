// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"strconv"

	libObservability "github.com/LerianStudio/lib-observability"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	feeerrors "github.com/LerianStudio/midaz/v4/pkg"
	feeconstant "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"

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
	GetBillingPackageByID(ctx context.Context, id, organizationID uuid.UUID) (*model.BillingPackage, error)
	GetAllBillingPackages(ctx context.Context, organizationID uuid.UUID, ledgerID *uuid.UUID, billingType string, limit, page int) ([]*model.BillingPackage, int64, error)
	UpdateBillingPackage(ctx context.Context, id, organizationID uuid.UUID, updates map[string]any) (*model.BillingPackage, error)
	DeleteBillingPackage(ctx context.Context, id, organizationID uuid.UUID) error
}

// BillingPackageHandler exposes the billing-package CRUD surface over HTTP.
type BillingPackageHandler struct {
	Service BillingPackageUseCase
}

// CreateBillingPackage is a method that creates a BillingPackage.
func (handler *BillingPackageHandler) CreateBillingPackage(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload, ok := p.(*model.BillingPackage)
	if !ok || payload == nil {
		return http.WithError(c, feeerrors.ValidateInternalError(nil, feeconstant.EntityBillingPackage))
	}

	result, err := handler.createBillingPackage(ctx, organizationID, payload)
	if err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusCreated, result)
}

// createBillingPackage is the transport-agnostic core of the create op, shared by the
// Fiber wrapper (CreateBillingPackage) and the Huma shell. It owns the span, stamps
// the path org onto the payload, and calls the service; the caller resolves the org
// id, decodes the payload, and renders the created package/error.
func (handler *BillingPackageHandler) createBillingPackage(ctx context.Context, organizationID uuid.UUID, payload *model.BillingPackage) (*model.BillingPackage, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_billing_package")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	payload.OrganizationID = organizationID.String()

	span.SetAttributes(
		attribute.String("app.request.payload.type", payload.Type),
		attribute.String("app.request.payload.label", payload.Label),
		attribute.String("app.request.payload.ledger_id", payload.LedgerID),
		attribute.Bool("app.request.payload.has_enable", payload.Enable != nil),
		attribute.Bool("app.request.payload.enable", payload.Enable != nil && *payload.Enable),
	)

	result, errCreate := handler.Service.CreateBillingPackage(ctx, payload)
	if errCreate != nil {
		handleSpanByErrorClass(span, "Failed to create billing package", errCreate)

		return nil, errCreate
	}

	if result == nil {
		return nil, feeerrors.ValidateInternalError(feeconstant.ErrInternalServer, "BillingPackage")
	}

	return result, nil
}

// GetAllBillingPackages is a method that retrieves all BillingPackages.
func (handler *BillingPackageHandler) GetAllBillingPackages(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	pagination, err := handler.getAllBillingPackages(ctx, organizationID, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusOK, pagination)
}

// getAllBillingPackages is the transport-agnostic core of the list op, shared by the
// Fiber wrapper (GetAllBillingPackages) and the Huma shell. It owns the span, the
// ledgerId/type/limit/page query parsing+validation, the service call, and the
// pagination envelope. The caller resolves the org id and passes the raw query map
// (c.Queries() on Fiber, queriesFromValues(rawQuery) on Huma) so the binder is
// byte-identical, then renders the envelope/error.
func (handler *BillingPackageHandler) getAllBillingPackages(ctx context.Context, organizationID uuid.UUID, queries map[string]string) (model.Pagination, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_billing_packages")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
	)

	ledgerIDParam := queries["ledgerId"]

	var ledgerID *uuid.UUID

	if ledgerIDParam != "" {
		parsedLedgerID, errParse := uuid.Parse(ledgerIDParam)
		if errParse != nil {
			err := feeerrors.ValidateBusinessError(feeconstant.ErrInvalidQueryParameter, "", "ledgerId")
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ledgerId query parameter", err)

			return model.Pagination{}, err
		}

		ledgerID = &parsedLedgerID
	}

	const maxPaginationLimit = 100

	limit := 10
	page := 1

	if l := queries["limit"]; l != "" {
		parsed, errParse := strconv.Atoi(l)
		if errParse != nil || parsed < 1 {
			return model.Pagination{}, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidQueryParameter, "BillingPackage", "limit")
		}

		if parsed > maxPaginationLimit {
			return model.Pagination{}, feeerrors.ValidateBusinessError(feeconstant.ErrPaginationLimitExceeded, "BillingPackage", maxPaginationLimit)
		}

		limit = parsed
	}

	if p := queries["page"]; p != "" {
		parsed, errParse := strconv.Atoi(p)
		if errParse != nil || parsed < 1 {
			return model.Pagination{}, feeerrors.ValidateBusinessError(feeconstant.ErrInvalidQueryParameter, "BillingPackage", "page")
		}

		page = parsed
	}

	billingType := queries["type"]

	span.SetAttributes(
		attribute.String("app.request.ledger_id", ledgerIDParam),
		attribute.String("app.request.billing_type", billingType),
		attribute.Int("app.request.limit", limit),
		attribute.Int("app.request.page", page),
	)

	results, total, errGet := handler.Service.GetAllBillingPackages(ctx, organizationID, ledgerID, billingType, limit, page)
	if errGet != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all billing packages", errGet)

		return model.Pagination{}, errGet
	}

	pagination := model.Pagination{
		Limit: limit,
		Page:  page,
	}

	pagination.SetItems(results)
	pagination.SetTotal(int(total))

	return pagination, nil
}

// GetBillingPackageByID is a method that retrieves a BillingPackage by ID.
func (handler *BillingPackageHandler) GetBillingPackageByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	result, err := handler.getBillingPackageByID(ctx, organizationID, id)
	if err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}

// getBillingPackageByID is the transport-agnostic core of the get-by-id op, shared by
// the Fiber wrapper (GetBillingPackageByID) and the Huma shell. It owns the span, the
// service call, and the error log-level branch; the caller resolves the org+package
// ids and renders the returned package/error.
func (handler *BillingPackageHandler) getBillingPackageByID(ctx context.Context, organizationID, id uuid.UUID) (*model.BillingPackage, error) {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_billing_package_by_id")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.billing_package_id", id.String()),
	)

	result, errGet := handler.Service.GetBillingPackageByID(ctx, id, organizationID)
	if errGet != nil {
		handleSpanByErrorClass(span, "Failed to retrieve billing package", errGet)

		logLevel := libLog.LevelError
		if feeerrors.IsBusinessError(errGet) {
			logLevel = libLog.LevelWarn
		}

		logger.Log(ctx, logLevel, "Failed to retrieve BillingPackage", libLog.String("billing_package_id", id.String()), libLog.Err(errGet))

		return nil, errGet
	}

	return result, nil
}

// UpdateBillingPackage is a method that updates a BillingPackage by ID.
func (handler *BillingPackageHandler) UpdateBillingPackage(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload, ok := p.(*model.BillingPackageUpdate)
	if !ok || payload == nil {
		return http.WithError(c, feeerrors.ValidateInternalError(nil, feeconstant.EntityBillingPackage))
	}

	result, err := handler.updateBillingPackage(ctx, organizationID, id, payload)
	if err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}

// updateBillingPackage is the transport-agnostic core of the update op, shared by the
// Fiber wrapper (UpdateBillingPackage) and the Huma shell. It owns the span, the
// merge-patch Validate() + ToMap() + empty-update (ErrNothingToUpdate) guard, and the
// service call; the caller resolves the org+package ids, decodes the payload, and
// renders the updated package/error.
func (handler *BillingPackageHandler) updateBillingPackage(ctx context.Context, organizationID, id uuid.UUID, payload *model.BillingPackageUpdate) (*model.BillingPackage, error) {
	_, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_billing_package")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.billing_package_id", id.String()),
		attribute.Bool("app.request.payload.has_label", payload.Label != nil),
		attribute.Bool("app.request.payload.has_description", payload.Description != nil),
		attribute.Bool("app.request.payload.has_enable", payload.Enable != nil),
	)

	if validationErr := payload.Validate(); validationErr != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid update payload", validationErr)

		return nil, validationErr
	}

	updates := payload.ToMap()
	if len(updates) == 0 {
		validationErr := feeerrors.ValidateBusinessError(feeconstant.ErrNothingToUpdate, "BillingPackage")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty update payload", validationErr)

		return nil, validationErr
	}

	result, errUpdate := handler.Service.UpdateBillingPackage(ctx, id, organizationID, updates)
	if errUpdate != nil {
		handleSpanByErrorClass(span, "Failed to update billing package", errUpdate)

		return nil, errUpdate
	}

	return result, nil
}

// DeleteBillingPackage is a method that soft-deletes a BillingPackage by ID.
func (handler *BillingPackageHandler) DeleteBillingPackage(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	if err := handler.deleteBillingPackage(ctx, organizationID, id); err != nil {
		return http.WithError(c, err)
	}

	return commonsHttp.RespondStatus(c, fiber.StatusNoContent)
}

// deleteBillingPackage is the transport-agnostic core of the delete op, shared by the
// Fiber wrapper (DeleteBillingPackage) and the Huma shell. It owns the span, the
// service call, and the error log-level branch; the caller resolves the org+package
// ids and renders the 204/error.
func (handler *BillingPackageHandler) deleteBillingPackage(ctx context.Context, organizationID, id uuid.UUID) error {
	logger, tracer, reqId, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_billing_package")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.billing_package_id", id.String()),
	)

	if errDelete := handler.Service.DeleteBillingPackage(ctx, id, organizationID); errDelete != nil {
		handleSpanByErrorClass(span, "Failed to delete billing package", errDelete)

		logLevel := libLog.LevelError
		if feeerrors.IsBusinessError(errDelete) {
			logLevel = libLog.LevelWarn
		}

		logger.Log(ctx, logLevel, "Failed to remove BillingPackage", libLog.String("billing_package_id", id.String()), libLog.Err(errDelete))

		return errDelete
	}

	return nil
}
