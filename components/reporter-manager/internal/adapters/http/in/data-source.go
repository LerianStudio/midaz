// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"errors"

	"github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v3/components/reporter-manager/internal/services"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
	_ "github.com/LerianStudio/midaz/v3/components/reporter/pkg/model"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/net/http"

	commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
)

// DataSourceHandler handles HTTP requests for data source operations.
type DataSourceHandler struct {
	service *services.UseCase
}

// NewDataSourceHandler creates a new DataSourceHandler with the given service dependency.
// It returns an error if service is nil.
func NewDataSourceHandler(service *services.UseCase) (*DataSourceHandler, error) {
	if service == nil {
		return nil, errors.New("service must not be nil for DataSourceHandler")
	}

	return &DataSourceHandler{service: service}, nil
}

// GetDataSourceInformation retrieves all data sources connected on reporter.
//
//	@Summary		Get all data sources connected on reporter
//	@Description	Retrieves all data sources connected on plugin with all information from the database
//	@Tags			Data source
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200				{object}	[]model.DataSourceInformation
//	@Failure		401				{object}	pkg.HTTPError
//	@Failure		403				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/data-sources [get]
func (ds *DataSourceHandler) GetDataSourceInformation(c *fiber.Ctx) error {
	ctx := c.UserContext()

	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := ds.service.Tracer.Start(ctx, "handler.data_source.get")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
	)
	ds.service.Logger.Log(ctx, log.LevelInfo, "Initiating retrieval data source information")

	dataSourceInfo := ds.service.GetDataSourceInformation(ctx)

	ds.service.Logger.Log(ctx, log.LevelInfo, "Successfully get all data source information.")

	return commonsHttp.Respond(c, fiber.StatusOK, dataSourceInfo)
}

// GetDataSourceInformationByID retrieves a data sources information with data source id passed
//
//	@Summary		Get a data sources information
//	@Description	Retrieves a data sources information with data source id passed
//	@Tags			Data source
//	@Produce		json
//	@Security		BearerAuth
//	@Param			dataSourceId	path		string	true	"Data source ID"
//	@Success		200				{object}	model.DataSourceDetails
//	@Failure		400				{object}	pkg.HTTPError
//	@Failure		401				{object}	pkg.HTTPError
//	@Failure		403				{object}	pkg.HTTPError
//	@Failure		404				{object}	pkg.HTTPError
//	@Failure		500				{object}	pkg.HTTPError
//	@Router			/v1/data-sources/{dataSourceId} [get]
func (ds *DataSourceHandler) GetDataSourceInformationByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := ds.service.Tracer.Start(ctx, "handler.data_source.get_details_by_id")
	defer span.End()

	dataSourceID, ok := c.Locals("dataSourceId").(string)
	if !ok || dataSourceID == "" {
		return http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, "", "dataSourceId"))
	}

	ds.service.Logger.Log(ctx, log.LevelInfo, "Initiating retrieval data source information", log.String("data_source_id", dataSourceID))

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.data_source_id", dataSourceID),
	)

	dataSourceInfo, err := ds.service.GetDataSourceDetailsByID(ctx, dataSourceID)
	if err != nil {
		if http.IsBusinessError(err) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve data source information on query", err)
		} else {
			libOpentelemetry.HandleSpanError(span, "Failed to retrieve data source information on query", err)
		}

		ds.service.Logger.Log(ctx, log.LevelError, "Failed to retrieve data source information", log.Err(err))

		return http.WithError(c, err)
	}

	ds.service.Logger.Log(ctx, log.LevelInfo, "Successfully retrieved all data source information")

	return commonsHttp.Respond(c, fiber.StatusOK, dataSourceInfo)
}
