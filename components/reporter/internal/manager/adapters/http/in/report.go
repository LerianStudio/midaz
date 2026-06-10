// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"bytes"
	"context"
	"errors"

	"github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/components/reporter/internal/manager/services"
	pkg "github.com/LerianStudio/midaz/v4/pkg"
	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	netHTTP "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	_ "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/report"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/net/http"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
)

// ReportHandler handles HTTP requests for report operations.
type ReportHandler struct {
	service *services.UseCase
}

// NewReportHandler creates a new ReportHandler with the given service dependency.
// It returns an error if service is nil.
func NewReportHandler(service *services.UseCase) (*ReportHandler, error) {
	if service == nil {
		return nil, errors.New("service must not be nil for ReportHandler")
	}

	return &ReportHandler{service: service}, nil
}

// CreateReport is a method that creates a report.
//
//	@Summary		Create a Report
//	@Description	Create a Report of existent template with the input payload
//	@Tags			Reports
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Idempotency	header		string					false	"Client-provided idempotency key to prevent duplicate report creation"
//	@Param			reports			body		model.CreateReportInput	true	"Report Input"
//	@Success		201				{object}	report.Report
//	@Header			201				{string}	X-Idempotency-Replayed	"Set to 'true' when the response is a replay of a previously processed idempotent request"
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		401				{object}	pkg.HTTPError
//	@Failure		403				{object}	pkg.HTTPError
//	@Failure		404				{object}	pkg.HTTPError
//	@Failure		409				{object}	pkg.HTTPError	"Duplicate request in flight (idempotency key conflict)"
//	@Failure		422				{object}	pkg.HTTPError	"Business rule violation (e.g. schema validation failed)"
//	@Failure		500				{object}	pkg.HTTPError
//	@Failure		503				{object}	pkg.HTTPError	"Data source or service unavailable"
//	@Router			/v1/reports [post]
func (rh *ReportHandler) CreateReport(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := rh.service.Tracer.Start(ctx, "handler.report.create")
	defer span.End()

	// Extract X-Idempotency header and inject into context for the service layer
	if idempotencyKey := c.Get(libConstants.IdempotencyKey); idempotencyKey != "" {
		ctx = context.WithValue(ctx, constant.IdempotencyKeyCtx, idempotencyKey)
	}

	replayed := false
	ctx = context.WithValue(ctx, constant.IdempotencyReplayedCtx, &replayed)

	c.SetUserContext(ctx)

	payload, ok := p.(*model.CreateReportInput)
	if !ok {
		return netHTTP.BadRequest(c, "invalid request body")
	}

	rh.service.Logger.Log(ctx, log.LevelDebug, "Request to create report",
		log.String("template_id", payload.TemplateID),
		log.Int("filter_datasource_count", len(payload.Filters)),
	)

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.template_id", payload.TemplateID),
		attribute.Int("app.request.filter_datasource_count", len(payload.Filters)),
	)

	reportOut, err := rh.service.CreateReport(ctx, payload)
	if err != nil {
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create report", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to create report", err)
		}

		return netHTTP.WithError(c, err)
	}

	rh.service.Logger.Log(ctx, log.LevelDebug, "Successfully created report",
		log.String("report_id", reportOut.ID.String()),
	)

	if replayed {
		c.Set(libConstants.IdempotencyReplayed, "true")
	}

	return commonsHttp.Respond(c, fiber.StatusCreated, reportOut)
}

// GetDownloadReport is a method to make download of a report.
//
//	@Summary		Download a Report
//	@Description	Make a download of a Report passing the ID
//	@Tags			Reports
//	@Accept			json
//	@Produce		json,octet-stream
//	@Security		BearerAuth
//	@Param			id				path		string	true	"Report ID"
//	@Success		200				{file}		any
//	@Header			200				{string}	Content-Disposition	"attachment; filename=report.pdf"
//	@Header			200				{string}	Content-Type	"application/pdf | text/html | application/xml | text/csv"
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		401				{object}	pkg.HTTPError
//	@Failure		403				{object}	pkg.HTTPError
//	@Failure		404				{object}	pkg.HTTPError
//	@Failure		422				{object}	pkg.HTTPError	"Business rule violation (e.g. report not finished yet)"
//	@Failure		500				{object}	pkg.HTTPError
//	@Failure		503				{object}	pkg.HTTPError	"Storage service unavailable"
//	@Router			/v1/reports/{id}/download [get]
func (rh *ReportHandler) GetDownloadReport(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := rh.service.Tracer.Start(ctx, "handler.report.get_download")
	defer span.End()

	id, ok := c.Locals("id").(uuid.UUID)
	if !ok {
		return netHTTP.WithError(c, pkg.ValidateBusinessError(cnErr.ErrInvalidPathParameter, "", "id"))
	}

	rh.service.Logger.Log(ctx, log.LevelDebug, "Initiating download of report", log.String("report_id", id.String()))

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.report_id", id.String()),
	)

	fileBytes, fileName, contentType, err := rh.service.DownloadReport(ctx, id)
	if err != nil {
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to download report", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to download report", err)
		}

		rh.service.Logger.Log(ctx, log.LevelError, "Failed to download report", log.String("report_id", id.String()), log.Err(err))

		return netHTTP.WithError(c, err)
	}

	c.Set("Content-Type", contentType)
	c.Set("Content-Disposition", "attachment; filename=\""+fileName+"\"")
	rh.service.Logger.Log(ctx, log.LevelDebug, "Successfully downloaded report", log.String("report_id", id.String()))

	return c.SendStream(bytes.NewReader(fileBytes))
}

// GetReport is a method to get a report information.
//
//	@Summary		Get a Report
//	@Description	Get information of a Report passing the ID
//	@Tags			Reports
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id					path		string	true	"Report ID"
//	@Success		200					{object}	report.Report
//	@Failure		400					{object}	pkg.HTTPError
//	@Failure		401					{object}	pkg.HTTPError
//	@Failure		403					{object}	pkg.HTTPError
//	@Failure		404					{object}	pkg.HTTPError
//	@Failure		500					{object}	pkg.HTTPError
//	@Router			/v1/reports/{id} [get]
func (rh *ReportHandler) GetReport(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := rh.service.Tracer.Start(ctx, "handler.report.get")
	defer span.End()

	id, ok := c.Locals("id").(uuid.UUID)
	if !ok {
		return netHTTP.WithError(c, pkg.ValidateBusinessError(cnErr.ErrInvalidPathParameter, "", "id"))
	}

	rh.service.Logger.Log(ctx, log.LevelDebug, "Initiating get report", log.String("report_id", id.String()))

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.report_id", id.String()),
	)

	reportModel, err := rh.service.GetReportByID(ctx, id)
	if err != nil {
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve report on query", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve report on query", err)
		}

		rh.service.Logger.Log(ctx, log.LevelError, "Failed to retrieve report", log.String("report_id", id.String()), log.Err(err))

		return netHTTP.WithError(c, err)
	}

	return commonsHttp.Respond(c, fiber.StatusOK, reportModel)
}

// GetAllReports is a method that recovery all Reports information.
//
//	@Summary		Get all reports
//	@Description	List all the reports
//	@Tags			Reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			status			query		string	false	"Report status"	Enums(Processing, Finished, Error, PendingExtraction, Partial)
//	@Param			template_id		query		string	false	"Template UUID (also accepts templateId)"	format(uuid)
//	@Param			created_at		query		string	false	"Created at date, YYYY-MM-DD (also accepts createdAt)"	format(date)
//	@Param			start_date		query		string	false	"Filter window start, YYYY-MM-DD"	format(date)
//	@Param			end_date		query		string	false	"Filter window end, YYYY-MM-DD"	format(date)
//	@Param			description		query		string	false	"Filter by description"
//	@Param			output_format	query		string	false	"Filter by output format"	Enums(PDF, HTML, XML, TXT, CSV)
//	@Param			active			query		boolean	false	"Filter by active flag"
//	@Param			type			query		string	false	"Filter by type"
//	@Param			cursor			query		string	false	"Opaque cursor for cursor-based pagination"
//	@Param			sort_order		query		string	false	"Sort order"	Enums(asc, desc)	default(desc)
//	@Param			limit			query		int		false	"Page size"	default(10)
//	@Param			page			query		int		false	"Page number"	default(1)
//	@Success		200				{object}	model.Pagination{items=[]report.Report}
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		401				{object}	pkg.HTTPError
//	@Failure		403				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/reports [get]
func (rh *ReportHandler) GetAllReports(c *fiber.Ctx) error {
	ctx := c.UserContext()
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := rh.service.Tracer.Start(ctx, "handler.report.get_all")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)
		rh.service.Logger.Log(ctx, log.LevelError, "Failed to validate query parameters", log.Err(err))

		return netHTTP.WithError(c, err)
	}

	pagination := model.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	rh.service.Logger.Log(ctx, log.LevelDebug, "Initiating retrieval all reports")

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)

	span.SetAttributes(
		attribute.Int("app.request.page", headerParams.Page),
		attribute.Int("app.request.limit", headerParams.Limit),
	)

	reports, err := rh.service.GetAllReports(ctx, *headerParams)
	if err != nil {
		if pkg.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Reports on query", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve all Reports on query", err)
		}

		rh.service.Logger.Log(ctx, log.LevelError, "Failed to retrieve all reports", log.Err(err))

		return netHTTP.WithError(c, err)
	}

	rh.service.Logger.Log(ctx, log.LevelDebug, "Successfully retrieved all reports")

	pagination.SetItems(reports)
	pagination.SetTotal(len(reports))

	return commonsHttp.Respond(c, fiber.StatusOK, pagination)
}
