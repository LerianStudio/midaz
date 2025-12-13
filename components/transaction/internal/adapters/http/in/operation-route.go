package in

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/trace"
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

	logger, tracer, _, metricFactory := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_operation_route")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	payload := i.(*mmodel.CreateOperationRouteInput)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	logger.Infof("Request to create an operation route with details: %#v", payload)

	if err := handler.validateAccountRule(ctx, payload.Account); err != nil {
		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	operationRoute, err := handler.Command.CreateOperationRoute(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation route", err)

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	metricFactory.RecordOperationRouteCreated(ctx, organizationID.String(), ledgerID.String())

	logger.Infof("Successfully created operation route")

	if err := http.Created(c, operationRoute); err != nil {
		return fmt.Errorf("failed to send created operation route response: %w", err)
	}

	return nil
}

// GetOperationRouteByID is a method that retrieves Operation Route information by a given operation route id.
//
//	@Summary		Retrieve a specific operation route
//	@Description	Returns detailed information about an operation route identified by its UUID within the specified ledger
//	@Tags			Operation Route
//	@Produce		json
//	@Param			Authorization	header		string					true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string					false	"Request ID for tracing"
//	@Param			organization_id	path		string					true	"Organization ID in UUID format"
//	@Param			ledger_id		path		string					true	"Ledger ID in UUID format"
//	@Param			id				path		string					true	"Operation Route ID in UUID format"
//	@Success		200				{object}	mmodel.OperationRoute	"Successfully retrieved operation route"
//	@Failure		401				{object}	mmodel.Error			"Unauthorized access"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes/{id} [get]
func (handler *OperationRouteHandler) GetOperationRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_operation_route_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("operation_route_id").(uuid.UUID)

	logger.Infof("Initiating retrieval of Operation Route with Operation Route ID: %s", id.String())

	operationRoute, err := handler.Query.GetOperationRouteByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Operation Route on query", err)

		logger.Errorf("Failed to retrieve Operation Route with Operation Route ID: %s, Error: %s", id.String(), err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	logger.Infof("Successfully retrieved Operation Route with Operation Route ID: %s", id.String())

	if err := http.OK(c, operationRoute); err != nil {
		return fmt.Errorf("failed to send operation route response: %w", err)
	}

	return nil
}

// UpdateOperationRoute is a method that updates Operation Route information.
//
//	@Summary		Update an operation route
//	@Description	Updates an existing operation route's properties such as title, description, and type within the specified ledger
//	@Tags			Operation Route
//	@Accept			json
//	@Produce		json
//	@Param			Authorization		header		string								true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id		header		string								false	"Request ID for tracing"
//	@Param			organization_id		path		string								true	"Organization ID in UUID format"
//	@Param			ledger_id			path		string								true	"Ledger ID in UUID format"
//	@Param			operation_route_id	path		string								true	"Operation Route ID in UUID format"
//	@Param			operation-route		body		mmodel.UpdateOperationRouteInput	true	"Operation Route Input"
//	@Success		200					{object}	mmodel.OperationRoute				"Successfully updated operation route"
//	@Failure		400					{object}	mmodel.Error						"Invalid input, validation errors"
//	@Failure		401					{object}	mmodel.Error						"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error						"Forbidden access"
//	@Failure		404					{object}	mmodel.Error						"Operation Route not found"
//	@Failure		409					{object}	mmodel.Error						"Conflict: Operation Route with the same title already exists"
//	@Failure		500					{object}	mmodel.Error						"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes/{operation_route_id} [patch]
func (handler *OperationRouteHandler) UpdateOperationRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_operation_route")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("operation_route_id").(uuid.UUID)

	logger.Infof("Initiating update of Operation Route with Operation Route ID: %s", id.String())

	payload := i.(*mmodel.UpdateOperationRouteInput)
	logger.Infof("Request to update an Operation Route with details: %#v", payload)

	if err := handler.validateAccountRule(ctx, payload.Account); err != nil {
		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)
	}

	if err := handler.performOperationRouteUpdate(ctx, c, &span, organizationID, ledgerID, id, payload); err != nil {
		return err
	}

	operationRoute, err := handler.Query.GetOperationRouteByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve Operation Route on query", err)

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	logger.Infof("Successfully updated Operation Route with Operation Route ID: %s", id.String())

	if payload.Account != nil {
		if err := handler.Command.ReloadOperationRouteCache(ctx, organizationID, ledgerID, id); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to reload operation route cache", err)

			logger.Errorf("Failed to reload operation route cache: %v", err)
		}
	}

	if err := http.OK(c, operationRoute); err != nil {
		return fmt.Errorf("failed to send operation route response: %w", err)
	}

	return nil
}

// performOperationRouteUpdate executes the update command for an operation route.
func (handler *OperationRouteHandler) performOperationRouteUpdate(ctx context.Context, c *fiber.Ctx, span *trace.Span, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdateOperationRouteInput) error {
	_, err := handler.Command.UpdateOperationRoute(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update Operation Route on command", err)

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	return nil
}

// DeleteOperationRouteByID is a method that deletes Operation Route information.
//
//	@Summary		Delete an operation route
//	@Description	Deletes an existing operation route identified by its UUID within the specified ledger
//	@Tags			Operation Route
//	@Produce		json
//	@Param			Authorization		header	string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id		header	string	false	"Request ID for tracing"
//	@Param			organization_id		path	string	true	"Organization ID in UUID format"
//	@Param			ledger_id			path	string	true	"Ledger ID in UUID format"
//	@Param			operation_route_id	path	string	true	"Operation Route ID in UUID format"
//	@Success		204					"Successfully deleted operation route"
//	@Failure		401					{object}	mmodel.Error	"Unauthorized access"
//	@Failure		404					{object}	mmodel.Error	"Operation Route not found"
//	@Failure		500					{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/operation-routes/{operation_route_id} [delete]
func (handler *OperationRouteHandler) DeleteOperationRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_operation_route_by_id")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("operation_route_id").(uuid.UUID)

	logger.Infof("Initiating deletion of Operation Route with Operation Route ID: %s", id.String())

	if err := handler.Command.DeleteOperationRouteByID(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete Operation Route on command", err)

		logger.Errorf("Failed to delete Operation Route with Operation Route ID: %s, Error: %s", id.String(), err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	logger.Infof("Successfully deleted Operation Route with Operation Route ID: %s", id.String())

	if err := http.NoContent(c); err != nil {
		return fmt.Errorf("failed to send no content response: %w", err)
	}

	return nil
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
//	@Param			start_date		query		string	false	"Start Date"	example	"2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example	"2021-01-01"
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

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_operation_routes")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert metadata headerParams to JSON string", err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		return handler.retrieveOperationRoutesByMetadata(ctx, c, &span, logger, organizationID, ledgerID, headerParams, pagination)
	}

	logger.Infof("Initiating retrieval of all Operation Routes")

	headerParams.Metadata = &bson.M{}

	operationRoutes, cur, err := handler.Query.GetAllOperationRoutes(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to retrieve all Operation Routes on query", err)

		logger.Errorf("Failed to retrieve all Operation Routes, Error: %s", err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	logger.Infof("Successfully retrieved all Operation Routes")

	pagination.SetItems(operationRoutes)
	pagination.SetCursor(cur.Next, cur.Prev)

	if err := http.OK(c, pagination); err != nil {
		return fmt.Errorf("failed to send operation routes pagination response: %w", err)
	}

	return nil
}

// retrieveOperationRoutesByMetadata retrieves operation routes filtered by metadata.
func (handler *OperationRouteHandler) retrieveOperationRoutesByMetadata(ctx context.Context, c *fiber.Ctx, span *trace.Span, logger libLog.Logger, organizationID, ledgerID uuid.UUID, headerParams *http.QueryHeader, pagination libPostgres.Pagination) error {
	logger.Infof("Initiating retrieval of all Operation Routes by metadata")

	operationRoutes, cur, err := handler.Query.GetAllMetadataOperationRoutes(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Operation Routes by metadata", err)

		logger.Errorf("Failed to retrieve all Operation Routes, Error: %s", err.Error())

		if err := http.WithError(c, err); err != nil {
			return fmt.Errorf("failed to send error response: %w", err)
		}

		return nil
	}

	logger.Infof("Successfully retrieved all Operation Routes by metadata")

	pagination.SetItems(operationRoutes)
	pagination.SetCursor(cur.Next, cur.Prev)

	if err := http.OK(c, pagination); err != nil {
		return fmt.Errorf("failed to send operation routes pagination response: %w", err)
	}

	return nil
}

// validateAccountRule validates account rule configuration for operation routes.
// It ensures proper pairing of ruleType and validIf, and validates data types based on rule type.
func (handler *OperationRouteHandler) validateAccountRule(ctx context.Context, account *mmodel.AccountRule) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.validate_account_rule")
	defer span.End()

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.account", account)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert account to JSON string", err)
	}

	if account == nil {
		return nil
	}

	if err := handler.validateAccountRuleFields(logger, &span, account); err != nil {
		return err
	}

	if account.RuleType != "" && account.ValidIf != nil {
		return handler.validateAccountRuleType(logger, &span, account)
	}

	return nil
}

// validateAccountRuleFields validates that ruleType and validIf are properly paired.
func (handler *OperationRouteHandler) validateAccountRuleFields(logger libLog.Logger, span *trace.Span, account *mmodel.AccountRule) error {
	if account.RuleType != "" && account.ValidIf == nil {
		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "account.validIf")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account rule type provided but validIf is missing", err)
		logger.Warnf("Account rule type provided but validIf is missing, Error: %s", err.Error())

		return fmt.Errorf("account rule type validation failed: %w", err)
	}

	if account.RuleType == "" && account.ValidIf != nil {
		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "account.ruleType")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account validIf provided but rule type is missing", err)
		logger.Warnf("Account validIf provided but rule type is missing, Error: %s", err.Error())

		return fmt.Errorf("account validIf validation failed: %w", err)
	}

	return nil
}

// validateAccountRuleType validates the account rule based on its type.
func (handler *OperationRouteHandler) validateAccountRuleType(logger libLog.Logger, span *trace.Span, account *mmodel.AccountRule) error {
	switch strings.ToLower(account.RuleType) {
	case constant.AccountRuleTypeAlias:
		return handler.validateAliasRule(logger, span, account.ValidIf)
	case constant.AccountRuleTypeAccountType:
		return handler.validateAccountTypeRule(logger, span, account.ValidIf)
	default:
		err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleType, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account rule type", err)
		logger.Warnf("Invalid account rule type, Error: %s", err.Error())

		return fmt.Errorf("invalid account rule type: %w", err)
	}
}

// validateAliasRule validates that validIf is a string for alias rules.
func (handler *OperationRouteHandler) validateAliasRule(logger libLog.Logger, span *trace.Span, validIf any) error {
	if _, ok := validIf.(string); !ok {
		err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleValue, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ValidIf type for alias rule", err)
		logger.Warnf("Invalid ValidIf type for alias rule, Error: %s", err.Error())

		return fmt.Errorf("invalid validIf type for alias rule: %w", err)
	}

	return nil
}

// validateAccountTypeRule validates that validIf is a string array for account_type rules.
func (handler *OperationRouteHandler) validateAccountTypeRule(logger libLog.Logger, span *trace.Span, validIf any) error {
	switch v := validIf.(type) {
	case []string:
		return nil
	case []any:
		return handler.validateAccountTypeArray(logger, span, v)
	default:
		err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleValue, reflect.TypeOf(mmodel.OperationRoute{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ValidIf type for account_type rule", err)
		logger.Warnf("Invalid ValidIf type for account_type rule, Error: %s", err.Error())

		return fmt.Errorf("invalid validIf type for account_type rule: %w", err)
	}
}

// validateAccountTypeArray validates that all elements in the array are strings.
func (handler *OperationRouteHandler) validateAccountTypeArray(logger libLog.Logger, span *trace.Span, items []any) error {
	for _, item := range items {
		if _, ok := item.(string); !ok {
			err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleValue, reflect.TypeOf(mmodel.OperationRoute{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ValidIf array element type", err)
			logger.Warnf("Invalid ValidIf array element type, Error: %s", err.Error())

			return fmt.Errorf("account type array validation failed: %w", err)
		}
	}

	return nil
}
