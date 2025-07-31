package in

import (
	"context"
	"reflect"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
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

	if err := handler.validateAccountRule(ctx, payload.Account); err != nil {
		return http.WithError(c, err)
	}

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

// UpdateOperationRoute is a method that updates Operation Route information.
//
//	@Summary		Update an operation route
//	@Description	Updates an existing operation route's properties such as title, description, and type within the specified ledger
//	@Tags			Operation Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string								false	"Request ID for tracing"
//	@Param			organization_id	path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string								true	"Ledger ID in UUID format"
//	@Param			operation_route_id	path		string								true	"Operation Route ID in UUID format"
//	@Param			operation-route	body		mmodel.UpdateOperationRouteInput	true	"Operation Route Input"
//	@Success		200				{object}	mmodel.OperationRoute				"Successfully updated operation route"
//	@Failure		400				{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error						"Forbidden access"
//	@Failure		404				{object}	mmodel.Error						"Operation Route not found"
//	@Failure		409				{object}	mmodel.Error						"Conflict: Operation Route with the same title already exists"
//	@Failure		500				{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes/{operation_route_id} [patch]
func (handler *OperationRouteHandler) UpdateOperationRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_operation_route")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("operation_route_id").(uuid.UUID)

	logger.Infof("Initiating update of Operation Route with Operation Route ID: %s", id.String())

	payload := i.(*mmodel.UpdateOperationRouteInput)
	logger.Infof("Request to update an Operation Route with details: %#v", payload)

	if err := handler.validateAccountRule(ctx, payload.Account); err != nil {
		return http.WithError(c, err)
	}

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateOperationRoute(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update Operation Route on command", err)

		return http.WithError(c, err)
	}

	operationRoute, err := handler.Query.GetOperationRouteByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve Operation Route on query", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Operation Route with Operation Route ID: %s", id.String())

	if payload.Account != nil {
		if err := handler.Command.ReloadOperationRouteCache(ctx, organizationID, ledgerID, id); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to reload operation route cache", err)

			logger.Errorf("Failed to reload operation route cache: %v", err)
		}
	}

	return http.OK(c, operationRoute)
}

// DeleteOperationRouteByID is a method that deletes Operation Route information.
//
//	@Summary		Delete an operation route
//	@Description	Deletes an existing operation route identified by its UUID within the specified ledger
//	@Tags			Operation Route
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			operation_route_id	path		string	true	"Operation Route ID in UUID format"
//	@Success		204				"Successfully deleted operation route"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		404				{object}	mmodel.Error	"Operation Route not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes/{operation_route_id} [delete]
func (handler *OperationRouteHandler) DeleteOperationRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_operation_route_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("operation_route_id").(uuid.UUID)

	logger.Infof("Initiating deletion of Operation Route with Operation Route ID: %s", id.String())

	if err := handler.Command.DeleteOperationRouteByID(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete Operation Route on command", err)

		logger.Errorf("Failed to delete Operation Route with Operation Route ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully deleted Operation Route with Operation Route ID: %s", id.String())

	return http.NoContent(c)
}

// GetAllOperationRoutes is a method that retrieves all Operation Routes information.
//
//	@Summary		Retrieve all operation routes
//	@Description	Returns a list of all operation routes within the specified ledger with cursor-based pagination
//	@Tags			Operation Route
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			organization_id	path		string	true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string	true	"Ledger ID in UUID format"
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example "2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example "2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.OperationRoute,next_cursor=string,prev_cursor=string,limit=int,page=nil}
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Operation Route not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes [get]
func (handler *OperationRouteHandler) GetAllOperationRoutes(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_operation_routes")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := libPostgres.Pagination{
		Limit:      headerParams.Limit,
		NextCursor: headerParams.Cursor,
		SortOrder:  headerParams.SortOrder,
		StartDate:  headerParams.StartDate,
		EndDate:    headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Operation Routes by metadata")

		err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to convert metadata headerParams to JSON string", err)

			return http.WithError(c, err)
		}

		operationRoutes, cur, err := handler.Query.GetAllMetadataOperationRoutes(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Operation Routes by metadata", err)

			logger.Errorf("Failed to retrieve all Operation Routes, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Operation Routes by metadata")

		pagination.SetItems(operationRoutes)
		pagination.SetCursor(cur.Next, cur.Prev)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Operation Routes")

	headerParams.Metadata = &bson.M{}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "headerParams", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert headerParams to JSON string", err)

		return http.WithError(c, err)
	}

	operationRoutes, cur, err := handler.Query.GetAllOperationRoutes(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all Operation Routes on query", err)

		logger.Errorf("Failed to retrieve all Operation Routes, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Operation Routes")

	pagination.SetItems(operationRoutes)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}

// validateAccountRule validates account rule configuration for operation routes.
// It ensures proper pairing of ruleType and validIf, and validates data types based on rule type.
func (handler *OperationRouteHandler) validateAccountRule(ctx context.Context, account *mmodel.AccountRule) error {
	tracer := libCommons.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.validate_account_rule")
	defer span.End()

	if account == nil {
		return nil
	}

	if account.RuleType != "" && account.ValidIf == nil {
		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "account.validIf")

		libOpentelemetry.HandleSpanError(&span, "Account rule type provided but validIf is missing", err)

		return err
	}

	if account.RuleType == "" && account.ValidIf != nil {
		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "account.ruleType")

		libOpentelemetry.HandleSpanError(&span, "Account validIf provided but rule type is missing", err)

		return err
	}

	if account.RuleType != "" && account.ValidIf != nil {
		switch strings.ToLower(account.RuleType) {
		case constant.AccountRuleTypeAlias:
			if _, ok := account.ValidIf.(string); !ok {
				err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleValue, reflect.TypeOf(mmodel.OperationRoute{}).Name())

				libOpentelemetry.HandleSpanError(&span, "Invalid ValidIf type for alias rule", err)

				return err
			}
		case constant.AccountRuleTypeAccountType:
			switch v := account.ValidIf.(type) {
			case []string:
			case []any:
				for _, item := range v {
					if _, ok := item.(string); !ok {
						err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleValue, reflect.TypeOf(mmodel.OperationRoute{}).Name())

						libOpentelemetry.HandleSpanError(&span, "Invalid ValidIf array element type", err)

						return err
					}
				}
			default:
				err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleValue, reflect.TypeOf(mmodel.OperationRoute{}).Name())

				libOpentelemetry.HandleSpanError(&span, "Invalid ValidIf type for account_type rule", err)

				return err
			}
		default:
			err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleType, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanError(&span, "Invalid account rule type", err)

			return err
		}
	}

	return nil
}
