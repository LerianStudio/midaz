package in

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// OperationRouteHandler is a struct that contains the command and query use cases.
type OperationRouteHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// Create an Operation Route.
//
//	@Summary		Create Operation Route
//	@Description	Endpoint to create a new Operation Route.
//	@Tags			Operation Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			operation-route	body		mmodel.CreateOperationRouteInput	true	"Operation Route Input"
//	@Success		201				{object}	mmodel.OperationRoute				"Successfully created operation route"
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		404				{object}	mmodel.Error						"Ledger or organization not found"
//	@Failure		409				{object}	mmodel.Error						"Conflict: Operation Route with the same title already exists"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes [post]
func (handler *OperationRouteHandler) CreateOperationRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_operation_route")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	payload := i.(*mmodel.CreateOperationRouteInput)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	logger.Infof("Request to create an operation route with details: %#v", payload)

	operationRoute, err := handler.Command.CreateOperationRoute(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create operation route", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created operation route")

	return http.Created(c, operationRoute)
}

// GetOperationRouteByID is a method that retrieves Operation Route information by a given operation route id.
//
//	@Summary		Retrieve a specific operation route
//	@Description	Returns detailed information about an operation route identified by its UUID within the specified ledger
//	@Tags			Operation Route
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			id				path		string	true	"Operation Route ID in UUID format"
//	@Success		200				{object}	mmodel.OperationRoute	"Successfully retrieved operation route"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes/{id} [get]
func (handler *OperationRouteHandler) GetOperationRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_operation_route_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("operation_route_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Operation Route with Operation Route ID: %s", id.String())

	operationRoute, err := handler.Query.GetOperationRouteByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Operation Route on query", err)

		logger.Errorf("Failed to retrieve Operation Route with Operation Route ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Operation Route with Operation Route ID: %s", id.String())

	return http.OK(c, operationRoute)
}
