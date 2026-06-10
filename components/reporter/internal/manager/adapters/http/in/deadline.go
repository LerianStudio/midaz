// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"

	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"

	"github.com/LerianStudio/midaz/v4/components/reporter/internal/manager/services"
	pkg "github.com/LerianStudio/midaz/v4/pkg"
	constant "github.com/LerianStudio/midaz/v4/pkg/constant"
	netHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/deadline"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// DeadlineHandler handles HTTP requests for deadline operations.
type DeadlineHandler struct {
	service *services.UseCase
}

// NewDeadlineHandler creates a new DeadlineHandler with the given service dependency.
func NewDeadlineHandler(service *services.UseCase) (*DeadlineHandler, error) {
	if service == nil {
		return nil, errors.New("service must not be nil for DeadlineHandler")
	}

	return &DeadlineHandler{service: service}, nil
}

// CreateDeadline is a method that creates a deadline.
//
//	@Summary		Create a Deadline
//	@Description	Create a Deadline with the input payload
//	@Tags			Deadlines
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			deadline	body		deadline.CreateDeadlineInput	true	"Deadline Input"
//	@Success		201			{object}	deadline.Deadline
//	@Failure		400			{object}	pkg.HTTPError
//	@Failure		401			{object}	pkg.HTTPError
//	@Failure		403			{object}	pkg.HTTPError
//	@Failure		500			{object}	pkg.HTTPError
//	@Router			/v1/deadlines [post]
func (dh *DeadlineHandler) CreateDeadline(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := dh.service.Tracer.Start(ctx, "handler.deadline.create")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqId))
	dh.service.Logger.Log(ctx, log.LevelDebug, "Request to create deadline")

	input, ok := p.(*deadline.CreateDeadlineInput)
	if !ok {
		return netHTTP.BadRequest(c, "invalid request body")
	}

	result, err := dh.service.CreateDeadline(ctx, input)
	if err != nil {
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create deadline", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to create deadline", err)
		}

		return netHTTP.WithError(c, err)
	}

	dh.service.Logger.Log(ctx, log.LevelDebug, "Successfully created deadline", log.String("deadline_id", result.ID.String()))

	return commonsHttp.Respond(c, fiber.StatusCreated, result)
}

// GetAllDeadlines is a method that retrieves all deadlines.
//
//	@Summary		Get all Deadlines
//	@Description	List all the deadlines
//	@Tags			Deadlines
//	@Produce		json
//	@Security		BearerAuth
//	@Param			status		query		string	false	"Deadline status"	Enums(pending, overdue, delivered)
//	@Param			limit		query		int		false	"Limit"		default(10)
//	@Param			page		query		int		false	"Page"		default(1)
//	@Success		200			{object}	model.Pagination{items=[]deadline.Deadline,page=int,limit=int,total=int}
//	@Failure		400			{object}	pkg.HTTPError
//	@Failure		401			{object}	pkg.HTTPError
//	@Failure		403			{object}	pkg.HTTPError
//	@Failure		500			{object}	pkg.HTTPError
//	@Router			/v1/deadlines [get]
func (dh *DeadlineHandler) GetAllDeadlines(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := dh.service.Tracer.Start(ctx, "handler.deadline.get_all")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)
		dh.service.Logger.Log(ctx, log.LevelError, "Failed to validate query parameters", log.Err(err))

		return netHTTP.WithError(c, err)
	}

	pagination := model.Pagination{Limit: headerParams.Limit, Page: headerParams.Page}

	dh.service.Logger.Log(ctx, log.LevelDebug, "Initiating retrieval of all deadlines")
	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.Int("app.request.limit", headerParams.Limit),
		attribute.Int("app.request.page", headerParams.Page),
	)

	deadlines, total, err := dh.service.GetAllDeadlines(ctx, *headerParams)
	if err != nil {
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all deadlines on query", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve all deadlines on query", err)
		}

		dh.service.Logger.Log(ctx, log.LevelError, "Failed to retrieve all deadlines", log.Err(err))

		return netHTTP.WithError(c, err)
	}

	dh.service.Logger.Log(ctx, log.LevelDebug, "Successfully retrieved all deadlines")
	pagination.SetItems(deadlines)
	pagination.SetTotal(int(total))

	return commonsHttp.Respond(c, fiber.StatusOK, pagination)
}

// UpdateDeadlineByID is a method that updates a deadline by ID.
//
//	@Summary		Update a Deadline
//	@Description	Update a Deadline by its ID
//	@Tags			Deadlines
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id			path		string							true	"Deadline ID"
//	@Param			deadline	body		deadline.UpdateDeadlineInput		true	"Deadline Update Input"
//	@Success		200			{object}	deadline.Deadline
//	@Failure		400			{object}	pkg.HTTPError
//	@Failure		401			{object}	pkg.HTTPError
//	@Failure		403			{object}	pkg.HTTPError
//	@Failure		404			{object}	pkg.HTTPError
//	@Failure		500			{object}	pkg.HTTPError
//	@Router			/v1/deadlines/{id} [patch]
func (dh *DeadlineHandler) UpdateDeadlineByID(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := dh.service.Tracer.Start(ctx, "handler.deadline.update")
	defer span.End()

	id, ok := c.Locals("id").(uuid.UUID)
	if !ok {
		return netHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", "id"))
	}

	dh.service.Logger.Log(ctx, log.LevelDebug, "Initiating deadline update", log.String("deadline_id", id.String()))
	span.SetAttributes(attribute.String("app.request.request_id", reqId), attribute.String("app.request.deadline_id", id.String()))

	input, ok := p.(*deadline.UpdateDeadlineInput)
	if !ok {
		return netHTTP.BadRequest(c, "invalid request body")
	}

	result, errUpdate := dh.service.UpdateDeadlineByID(ctx, id, input)
	if errUpdate != nil {
		if pkg.IsBusinessError(errUpdate) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update deadline", errUpdate)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to update deadline", errUpdate)
		}

		dh.service.Logger.Log(ctx, log.LevelError, "Failed to update deadline", log.String("deadline_id", id.String()), log.Err(errUpdate))

		return netHTTP.WithError(c, errUpdate)
	}

	dh.service.Logger.Log(ctx, log.LevelDebug, "Successfully updated deadline", log.String("deadline_id", id.String()))

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}

// DeleteDeadlineByID is a method that deletes a deadline by ID.
//
//	@Summary		Delete a Deadline
//	@Description	Delete a Deadline by its ID
//	@Tags			Deadlines
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Deadline ID"
//	@Success		204
//	@Failure		400		{object}	pkg.HTTPError
//	@Failure		401		{object}	pkg.HTTPError
//	@Failure		403		{object}	pkg.HTTPError
//	@Failure		404		{object}	pkg.HTTPError
//	@Failure		500		{object}	pkg.HTTPError
//	@Router			/v1/deadlines/{id} [delete]
func (dh *DeadlineHandler) DeleteDeadlineByID(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := dh.service.Tracer.Start(ctx, "handler.deadline.delete")
	defer span.End()

	id, ok := c.Locals("id").(uuid.UUID)
	if !ok {
		return netHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", "id"))
	}

	dh.service.Logger.Log(ctx, log.LevelDebug, "Initiating removal of deadline", log.String("deadline_id", id.String()))
	span.SetAttributes(attribute.String("app.request.request_id", reqId), attribute.String("app.request.deadline_id", id.String()))

	if err := dh.service.DeleteDeadlineByID(ctx, id); err != nil {
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to remove deadline on database", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to remove deadline on database", err)
		}

		dh.service.Logger.Log(ctx, log.LevelError, "Failed to remove deadline", log.String("deadline_id", id.String()), log.Err(err))

		return netHTTP.WithError(c, err)
	}

	dh.service.Logger.Log(ctx, log.LevelDebug, "Successfully removed deadline", log.String("deadline_id", id.String()))

	return commonsHttp.RespondStatus(c, fiber.StatusNoContent)
}

// DeliverDeadline is a method that marks a deadline as delivered.
//
//	@Summary		Deliver a Deadline
//	@Description	Mark a Deadline as delivered by its ID
//	@Tags			Deadlines
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id			path		string								true	"Deadline ID"
//	@Param			deadline	body		deadline.DeliverDeadlineInput		true	"Deliver Deadline Input"
//	@Success		200			{object}	deadline.Deadline
//	@Failure		400			{object}	pkg.HTTPError
//	@Failure		401			{object}	pkg.HTTPError
//	@Failure		403			{object}	pkg.HTTPError
//	@Failure		404			{object}	pkg.HTTPError
//	@Failure		500			{object}	pkg.HTTPError
//	@Router			/v1/deadlines/{id}/deliver [patch]
func (dh *DeadlineHandler) DeliverDeadline(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := dh.service.Tracer.Start(ctx, "handler.deadline.deliver")
	defer span.End()

	id, ok := c.Locals("id").(uuid.UUID)
	if !ok {
		return netHTTP.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", "id"))
	}

	dh.service.Logger.Log(ctx, log.LevelDebug, "Initiating deadline delivery", log.String("deadline_id", id.String()))
	span.SetAttributes(attribute.String("app.request.request_id", reqId), attribute.String("app.request.deadline_id", id.String()))

	input, ok := p.(*deadline.DeliverDeadlineInput)
	if !ok {
		return netHTTP.BadRequest(c, "invalid request body")
	}

	result, errDeliver := dh.service.DeliverDeadline(ctx, id, input)
	if errDeliver != nil {
		if pkg.IsBusinessError(errDeliver) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to deliver deadline", errDeliver)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to deliver deadline", errDeliver)
		}

		dh.service.Logger.Log(ctx, log.LevelError, "Failed to deliver deadline", log.String("deadline_id", id.String()), log.Err(errDeliver))

		return netHTTP.WithError(c, errDeliver)
	}

	dh.service.Logger.Log(ctx, log.LevelDebug, "Successfully delivered deadline", log.String("deadline_id", id.String()))

	return commonsHttp.Respond(c, fiber.StatusOK, result)
}
