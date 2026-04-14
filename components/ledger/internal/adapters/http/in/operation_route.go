// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := i.(*mmodel.CreateOperationRouteInput)

	recordSafePayloadAttributes(span, payload)
	logSafePayload(ctx, logger, "Request to create an operation route", payload)

	if err := handler.validateAccountRule(ctx, payload.Account); err != nil {
		return http.WithError(c, err)
	}

	if err := handler.validateAccountingEntries(ctx, payload.AccountingEntries); err != nil {
		return http.WithError(c, err)
	}

	if err := handler.validateAccountingRulesMatrix(ctx, payload.OperationType, payload.AccountingEntries); err != nil {
		return http.WithError(c, err)
	}

	// Reject unknown keys inside accountingEntries (e.g., "foobar") that Go's
	// json.Unmarshal silently ignores but could confuse clients into thinking
	// their data was accepted.
	if payload.AccountingEntries != nil {
		var rawBody map[string]json.RawMessage

		if err := json.Unmarshal(c.Body(), &rawBody); err == nil {
			if raw, ok := rawBody["accountingEntries"]; ok {
				if unknowns := findUnknownAccountingEntryKeys(raw); len(unknowns) > 0 {
					return http.WithError(c, pkg.ValidateBadRequestFieldsError(
						pkg.FieldValidations{}, pkg.FieldValidations{}, "",
						map[string]any{"accountingEntries": unknowns},
					))
				}
			}
		}
	}

	operationRoute, err := handler.Command.CreateOperationRoute(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create operation route", err)

		return http.WithError(c, err)
	}

	if err := metricFactory.RecordOperationRouteCreated(
		ctx,
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to record operation route created metric", err)
	}

	logger.Log(ctx, libLog.LevelInfo, "Successfully created operation route")

	return http.Created(c, operationRoute)
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

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "operation_route_id")
	if err != nil {
		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating retrieval of Operation Route with Operation Route ID: %s", id.String()))

	operationRoute, err := handler.Query.GetOperationRouteByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Operation Route on query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve Operation Route with Operation Route ID: %s, Error: %s", id.String(), err.Error()))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully retrieved Operation Route with Operation Route ID: %s", id.String()))

	return http.OK(c, operationRoute)
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

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "operation_route_id")
	if err != nil {
		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating update of Operation Route with Operation Route ID: %s", id.String()))

	payload := i.(*mmodel.UpdateOperationRouteInput)
	logSafePayload(ctx, logger, "Request to update an operation route", payload)

	if err := handler.validateAccountRule(ctx, payload.Account); err != nil {
		return http.WithError(c, err)
	}

	if err := handler.validateAccountingEntries(ctx, payload.AccountingEntries); err != nil {
		return http.WithError(c, err)
	}

	// Extract the raw JSON for accountingEntries from the request body to preserve
	// explicit null values for RFC 7396 JSON Merge Patch semantics. This allows the
	// repository to distinguish "field absent" (keep existing) from "field: null" (remove).
	if payload.AccountingEntries != nil {
		var rawBody map[string]json.RawMessage

		if err := json.Unmarshal(c.Body(), &rawBody); err == nil {
			if raw, ok := rawBody["accountingEntries"]; ok {
				if unknowns := findUnknownAccountingEntryKeys(raw); len(unknowns) > 0 {
					return http.WithError(c, pkg.ValidateBadRequestFieldsError(
						pkg.FieldValidations{}, pkg.FieldValidations{}, "",
						map[string]any{"accountingEntries": unknowns},
					))
				}

				payload.AccountingEntriesRaw = raw
			}
		}
	}

	// Validate accounting rules matrix for PATCH operations
	// We need to fetch the existing route to get operation type and merge entries
	// Validation runs when accountingEntries is present (even if removing entries via explicit null)
	if payload.AccountingEntries != nil || len(payload.AccountingEntriesRaw) > 0 {
		existingRoute, err := handler.Query.GetOperationRouteByID(ctx, organizationID, ledgerID, nil, id)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve existing Operation Route for validation", err)
			return http.WithError(c, err)
		}

		// Handle explicit top-level null for accountingEntries (RFC 7396: clear all)
		var mergedEntries *mmodel.AccountingEntries

		rawTrimmed := strings.TrimSpace(string(payload.AccountingEntriesRaw))
		if rawTrimmed == "null" {
			// Explicit null at top level - clear all entries
			mergedEntries = nil
		} else {
			// Merge incoming entries with existing to get final state
			// Pass raw JSON to properly handle explicit null removals (RFC 7396)
			mergedEntries = mergeAccountingEntries(existingRoute.AccountingEntries, payload.AccountingEntries, payload.AccountingEntriesRaw)
		}

		// Validate the merged entries against the direction×scenario matrix
		if err := handler.validateAccountingRulesMatrix(ctx, existingRoute.OperationType, mergedEntries); err != nil {
			return http.WithError(c, err)
		}
	}

	recordSafePayloadAttributes(span, payload)

	_, err = handler.Command.UpdateOperationRoute(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update Operation Route on command", err)

		return http.WithError(c, err)
	}

	operationRoute, err := handler.Query.GetOperationRouteByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Operation Route on query", err)

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully updated Operation Route with Operation Route ID: %s", id.String()))

	if payload.Account != nil {
		if err := handler.Command.ReloadOperationRouteCache(ctx, organizationID, ledgerID, id); err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to reload operation route cache", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to reload operation route cache: %v", err))
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

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "operation_route_id")
	if err != nil {
		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating deletion of Operation Route with Operation Route ID: %s", id.String()))

	if err := handler.Command.DeleteOperationRouteByID(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete Operation Route on command", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to delete Operation Route with Operation Route ID: %s, Error: %s", id.String(), err.Error()))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully deleted Operation Route with Operation Route ID: %s", id.String()))

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
//	@Param			start_date		query		string	false	"Start Date"	example	"2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example	"2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	Enums(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Param			type			query		string	false	"Filter by operation type"	Enums(source,destination,bidirectional)
//	@Success		200				{object}	http.Pagination{items=[]mmodel.OperationRoute}
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

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to validate query parameters, Error: %s", err.Error()))

		return http.WithError(c, err)
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Log(ctx, libLog.LevelInfo, "Initiating retrieval of all Operation Routes by metadata")

		operationRoutes, cur, err := handler.Query.GetAllMetadataOperationRoutes(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Operation Routes by metadata", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve all Operation Routes, Error: %s", err.Error()))

			return http.WithError(c, err)
		}

		logger.Log(ctx, libLog.LevelInfo, "Successfully retrieved all Operation Routes by metadata")

		pagination.SetItems(operationRoutes)
		pagination.SetCursor(cur.Next, cur.Prev)

		return http.OK(c, pagination)
	}

	logger.Log(ctx, libLog.LevelInfo, "Initiating retrieval of all Operation Routes")

	headerParams.Metadata = &bson.M{}

	operationRoutes, cur, err := handler.Query.GetAllOperationRoutes(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Operation Routes on query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve all Operation Routes, Error: %s", err.Error()))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, "Successfully retrieved all Operation Routes")

	pagination.SetItems(operationRoutes)
	pagination.SetCursor(cur.Next, cur.Prev)

	return http.OK(c, pagination)
}

// validateAccountRule validates account rule configuration for operation routes.
// It ensures proper pairing of ruleType and validIf, and validates data types based on rule type.
func (handler *OperationRouteHandler) validateAccountRule(ctx context.Context, account *mmodel.AccountRule) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.validate_account_rule")
	defer span.End()

	recordSafePayloadAttributes(span, account)

	if account == nil {
		return nil
	}

	if account.RuleType != "" && account.ValidIf == nil {
		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "account.validIf")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account rule type provided but validIf is missing", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Account rule type provided but validIf is missing, Error: %s", err.Error()))

		return err
	}

	if account.RuleType == "" && account.ValidIf != nil {
		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, reflect.TypeOf(mmodel.OperationRoute{}).Name(), "account.ruleType")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Account validIf provided but rule type is missing", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Account validIf provided but rule type is missing, Error: %s", err.Error()))

		return err
	}

	if account.RuleType != "" && account.ValidIf != nil {
		switch strings.ToLower(account.RuleType) {
		case constant.AccountRuleTypeAlias:
			if _, ok := account.ValidIf.(string); !ok {
				err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleValue, reflect.TypeOf(mmodel.OperationRoute{}).Name())

				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ValidIf type for alias rule", err)

				logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Invalid ValidIf type for alias rule, Error: %s", err.Error()))

				return err
			}
		case constant.AccountRuleTypeAccountType:
			switch v := account.ValidIf.(type) {
			case []string:
			case []any:
				for _, item := range v {
					if _, ok := item.(string); !ok {
						err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleValue, reflect.TypeOf(mmodel.OperationRoute{}).Name())

						libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ValidIf array element type", err)

						logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Invalid ValidIf array element type, Error: %s", err.Error()))

						return err
					}
				}
			default:
				err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleValue, reflect.TypeOf(mmodel.OperationRoute{}).Name())

				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid ValidIf type for account_type rule", err)

				logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Invalid ValidIf type for account_type rule, Error: %s", err.Error()))

				return err
			}
		default:
			err := pkg.ValidateBusinessError(constant.ErrInvalidAccountRuleType, reflect.TypeOf(mmodel.OperationRoute{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid account rule type", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Invalid account rule type, Error: %s", err.Error()))

			return err
		}
	}

	return nil
}

// validateAccountingEntries validates the structure of accounting entries.
// It ensures that any present rubric (debit/credit) has non-empty code and description.
// Note: This function validates STRUCTURE only. Field REQUIREMENTS (which fields are mandatory
// based on direction+scenario) are validated by validateAccountingRulesMatrix.
func (handler *OperationRouteHandler) validateAccountingEntries(ctx context.Context, entries *mmodel.AccountingEntries) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.validate_accounting_entries")
	defer span.End()

	recordSafePayloadAttributes(span, entries)

	if entries == nil {
		return nil
	}

	entityName := reflect.TypeOf(mmodel.OperationRoute{}).Name()

	actions := []struct {
		name  string
		entry *mmodel.AccountingEntry
	}{
		{"direct", entries.Direct},
		{"hold", entries.Hold},
		{"commit", entries.Commit},
		{"cancel", entries.Cancel},
		{"revert", entries.Revert},
	}

	for _, action := range actions {
		if action.entry == nil {
			continue
		}

		// An entry with neither debit nor credit is invalid structure
		if action.entry.Debit == nil && action.entry.Credit == nil {
			fieldPath := "accountingEntries." + action.name + ".debit, accountingEntries." + action.name + ".credit"

			err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, entityName, fieldPath)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Accounting entry missing both debit and credit", err)

			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Accounting entry %s missing both debit and credit, Error: %s", action.name, err.Error()))

			return err
		}

		// Validate debit rubric if present
		if action.entry.Debit != nil {
			if err := handler.validateRubricStructure(ctx, span, logger, entityName, action.name, "debit", action.entry.Debit); err != nil {
				return err
			}
		}

		// Validate credit rubric if present
		if action.entry.Credit != nil {
			if err := handler.validateRubricStructure(ctx, span, logger, entityName, action.name, "credit", action.entry.Credit); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateRubricStructure validates that a rubric has non-empty code and description.
func (handler *OperationRouteHandler) validateRubricStructure(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	entityName, actionName, side string,
	rubric *mmodel.AccountingRubric,
) error {
	if strings.TrimSpace(rubric.Code) == "" {
		fieldPath := "accountingEntries." + actionName + "." + side + ".code"

		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, entityName, fieldPath)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Accounting rubric code is empty", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Accounting entry %s %s code is empty, Error: %s", actionName, side, err.Error()))

		return err
	}

	if strings.TrimSpace(rubric.Description) == "" {
		fieldPath := "accountingEntries." + actionName + "." + side + ".description"

		err := pkg.ValidateBusinessError(constant.ErrMissingFieldsInRequest, entityName, fieldPath)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Accounting rubric description is empty", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Accounting entry %s %s description is empty, Error: %s", actionName, side, err.Error()))

		return err
	}

	return nil
}

// validAccountingEntryKeys defines the allowed top-level keys inside accountingEntries.
var validAccountingEntryKeys = map[string]struct{}{
	"direct": {},
	"hold":   {},
	"commit": {},
	"cancel": {},
	"revert": {},
}

// findUnknownAccountingEntryKeys parses the raw JSON for accountingEntries and returns
// a map of keys that are not in the allowed set (direct, hold, commit, cancel, revert).
// Returns nil when all keys are valid. The returned map is suitable for use with
// ValidateBadRequestFieldsError to produce a standard 0053 "Unexpected Fields" error.
func findUnknownAccountingEntryKeys(raw json.RawMessage) map[string]any {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}

	unknowns := make(map[string]any)

	for key := range fields {
		if _, ok := validAccountingEntryKeys[key]; !ok {
			unknowns[key] = string(fields[key])
		}
	}

	if len(unknowns) == 0 {
		return nil
	}

	return unknowns
}

// fieldRequirement defines which fields (debit/credit) are required for a direction+scenario.
type fieldRequirement struct {
	debitRequired  bool
	creditRequired bool
}

// getFieldRequirements returns the field requirements based on operationType and scenario.
// This implements the direction × scenario matrix from accounting-rules.md:
//
//	source:      direct[D], hold[D][C], commit[D], cancel[D][C]
//	destination: direct[C], commit[C]
//	bidirectional: all scenarios require [D][C]
//
// Note: Blocked scenarios (source/revert, destination/hold, etc.) are validated
// separately by validateDirectionScenarioMatrix. This function assumes the
// scenario is allowed for the direction.
func getFieldRequirements(operationType, scenario string) fieldRequirement {
	// Bidirectional always requires both
	if operationType == constant.OperationRouteTypeBidirectional {
		return fieldRequirement{debitRequired: true, creditRequired: true}
	}

	// Source direction requirements
	if operationType == constant.OperationRouteTypeSource {
		switch scenario {
		case constant.ActionDirect, constant.ActionCommit:
			// Unilateral operations at origin - only debit
			return fieldRequirement{debitRequired: true, creditRequired: false}
		case constant.ActionHold, constant.ActionCancel:
			// Move between available ↔ on_hold in same account - both required
			return fieldRequirement{debitRequired: true, creditRequired: true}
		}
	}

	// Destination direction requirements
	if operationType == constant.OperationRouteTypeDestination {
		switch scenario {
		case constant.ActionDirect, constant.ActionCommit:
			// Unilateral operations at destination - only credit
			return fieldRequirement{debitRequired: false, creditRequired: true}
		}
	}

	// Default: require both (safe fallback)
	return fieldRequirement{debitRequired: true, creditRequired: true}
}

// validateAccountingRulesMatrix validates that the accounting entries comply with
// the direction × scenario matrix defined in accounting-rules.md.
//
// Matrix rules:
//   - source: direct[D], hold[D][C], commit[D], cancel[D][C], revert[✗]
//   - destination: direct[C], hold[✗], commit[C], cancel[✗], revert[✗]
//   - bidirectional: all scenarios allowed with [D][C]
//
// Additional rules:
//   - Reserve group (hold, commit, cancel) must be atomic for source/bidirectional
//   - direct is mandatory when any other scenario is present
//   - revert is only allowed for bidirectional
func (handler *OperationRouteHandler) validateAccountingRulesMatrix(
	ctx context.Context,
	operationType string,
	entries *mmodel.AccountingEntries,
) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.validate_accounting_rules_matrix")
	defer span.End()

	// If no entries, nothing to validate
	if entries == nil {
		return nil
	}

	entityName := reflect.TypeOf(mmodel.OperationRoute{}).Name()

	// Check direction × scenario matrix (which scenarios are allowed)
	if err := handler.validateDirectionScenarioMatrix(ctx, operationType, entries, entityName); err != nil {
		return err
	}

	// Check field requirements per direction+scenario (which debit/credit are required)
	if err := handler.validateEntryFieldRequirements(ctx, operationType, entries, entityName); err != nil {
		return err
	}

	// Check reserve group atomicity (hold requires commit and cancel)
	if err := handler.validateReserveGroupAtomicity(ctx, operationType, entries, entityName); err != nil {
		return err
	}

	// Check direct is mandatory when other scenarios exist
	if err := handler.validateDirectMandatory(ctx, entries, entityName); err != nil {
		return err
	}

	logger.Log(ctx, libLog.LevelDebug, "Accounting rules matrix validation passed")

	return nil
}

// validateDirectionScenarioMatrix checks that scenarios are valid for the given direction.
func (handler *OperationRouteHandler) validateDirectionScenarioMatrix(
	ctx context.Context,
	operationType string,
	entries *mmodel.AccountingEntries,
	entityName string,
) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.validate_direction_scenario_matrix")
	defer span.End()

	switch operationType {
	case constant.OperationRouteTypeSource:
		// source: revert is NOT allowed
		if entries.Revert != nil {
			err := pkg.ValidateBusinessError(
				constant.ErrRevertOnlyBidirectional,
				entityName,
				"revert is only allowed for bidirectional operation routes",
			)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Revert not allowed for source direction", err)
			logger.Log(ctx, libLog.LevelWarn, "Revert scenario not allowed for source direction")

			return err
		}

	case constant.OperationRouteTypeDestination:
		// destination: hold, cancel are NOT allowed (return 0162)
		invalidScenarios := []struct {
			name  string
			entry *mmodel.AccountingEntry
		}{
			{constant.ActionHold, entries.Hold},
			{constant.ActionCancel, entries.Cancel},
		}

		for _, scenario := range invalidScenarios {
			if scenario.entry != nil {
				err := pkg.ValidateBusinessError(
					constant.ErrScenarioNotAllowedForDirection,
					entityName,
					fmt.Sprintf("%s scenario is not allowed for destination direction", scenario.name),
				)

				libOpentelemetry.HandleSpanBusinessErrorEvent(span, fmt.Sprintf("%s not allowed for destination", scenario.name), err)
				logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("%s scenario not allowed for destination direction", scenario.name))

				return err
			}
		}

		// destination: revert is NOT allowed (return 0165 - same as source)
		if entries.Revert != nil {
			err := pkg.ValidateBusinessError(
				constant.ErrRevertOnlyBidirectional,
				entityName,
				"revert is only allowed for bidirectional operation routes",
			)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Revert not allowed for destination direction", err)
			logger.Log(ctx, libLog.LevelWarn, "Revert scenario not allowed for destination direction")

			return err
		}

	case constant.OperationRouteTypeBidirectional:
		// bidirectional: all scenarios allowed - no restrictions

	default:
		// Invalid operation type - should be caught by struct validation
	}

	return nil
}

// validateReserveGroupAtomicity ensures that hold, commit, and cancel form an atomic group.
// For source/bidirectional: if hold exists, commit AND cancel must also exist.
// For destination: reserve group is not applicable (hold/cancel not allowed).
func (handler *OperationRouteHandler) validateReserveGroupAtomicity(
	ctx context.Context,
	operationType string,
	entries *mmodel.AccountingEntries,
	entityName string,
) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.validate_reserve_group_atomicity")
	defer span.End()

	// Reserve group only applies to source and bidirectional
	if operationType == constant.OperationRouteTypeDestination {
		return nil
	}

	hasHold := entries.Hold != nil
	hasCommit := entries.Commit != nil
	hasCancel := entries.Cancel != nil

	// If hold exists, both commit and cancel must exist
	if hasHold {
		var missing []string

		if !hasCommit {
			missing = append(missing, "commit")
		}

		if !hasCancel {
			missing = append(missing, "cancel")
		}

		if len(missing) > 0 {
			err := pkg.ValidateBusinessError(
				constant.ErrReserveGroupIncomplete,
				entityName,
				fmt.Sprintf("reserve group incomplete: hold requires %s", strings.Join(missing, " and ")),
			)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reserve group incomplete", err)
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Reserve group incomplete: missing %v", missing))

			return err
		}
	}

	// If commit or cancel exists without hold, that's also incomplete
	if (hasCommit || hasCancel) && !hasHold {
		err := pkg.ValidateBusinessError(
			constant.ErrReserveGroupIncomplete,
			entityName,
			"reserve group incomplete: commit and cancel require hold",
		)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Reserve group incomplete - missing hold", err)
		logger.Log(ctx, libLog.LevelWarn, "Reserve group incomplete: commit/cancel without hold")

		return err
	}

	return nil
}

// validateDirectMandatory ensures that direct scenario is present when other scenarios exist.
func (handler *OperationRouteHandler) validateDirectMandatory(
	ctx context.Context,
	entries *mmodel.AccountingEntries,
	entityName string,
) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.validate_direct_mandatory")
	defer span.End()

	hasDirect := entries.Direct != nil
	hasOtherScenarios := entries.Hold != nil || entries.Commit != nil ||
		entries.Cancel != nil || entries.Revert != nil

	// If any other scenario exists, direct must also exist
	if hasOtherScenarios && !hasDirect {
		err := pkg.ValidateBusinessError(
			constant.ErrDirectScenarioRequired,
			entityName,
			"direct scenario is required when other scenarios are present",
		)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Direct scenario required", err)
		logger.Log(ctx, libLog.LevelWarn, "Direct scenario is required when other scenarios are present")

		return err
	}

	return nil
}

// validateEntryFieldRequirements validates that required fields (debit/credit) are present
// based on the direction × scenario matrix. This is a permissive validation:
// - Required fields MUST be present
// - Non-required fields MAY be present (accepted but not enforced)
func (handler *OperationRouteHandler) validateEntryFieldRequirements(
	ctx context.Context,
	operationType string,
	entries *mmodel.AccountingEntries,
	entityName string,
) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	_, span := tracer.Start(ctx, "handler.validate_entry_field_requirements")
	defer span.End()

	// If no entries, nothing to validate
	if entries == nil {
		return nil
	}

	actions := []struct {
		name  string
		entry *mmodel.AccountingEntry
	}{
		{constant.ActionDirect, entries.Direct},
		{constant.ActionHold, entries.Hold},
		{constant.ActionCommit, entries.Commit},
		{constant.ActionCancel, entries.Cancel},
		{constant.ActionRevert, entries.Revert},
	}

	for _, action := range actions {
		if action.entry == nil {
			continue
		}

		req := getFieldRequirements(operationType, action.name)

		// Check debit requirement
		if req.debitRequired && action.entry.Debit == nil {
			fieldPath := fmt.Sprintf("accountingEntries.%s.debit", action.name)

			err := pkg.ValidateBusinessError(
				constant.ErrAccountingEntryFieldRequired,
				entityName,
				fmt.Sprintf("%s is required for %s/%s", "debit", operationType, action.name),
			)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, fmt.Sprintf("Debit required for %s/%s", operationType, action.name), err)
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Missing required field %s, Error: %s", fieldPath, err.Error()))

			return err
		}

		// Check credit requirement
		if req.creditRequired && action.entry.Credit == nil {
			fieldPath := fmt.Sprintf("accountingEntries.%s.credit", action.name)

			err := pkg.ValidateBusinessError(
				constant.ErrAccountingEntryFieldRequired,
				entityName,
				fmt.Sprintf("%s is required for %s/%s", "credit", operationType, action.name),
			)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, fmt.Sprintf("Credit required for %s/%s", operationType, action.name), err)
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Missing required field %s, Error: %s", fieldPath, err.Error()))

			return err
		}
	}

	logger.Log(ctx, libLog.LevelDebug, "Entry field requirements validation passed")

	return nil
}

// mergeAccountingEntries creates a merged view of existing and incoming accounting entries.
// Used for PATCH operations where only partial updates are provided.
//
// This function implements RFC 7396 JSON Merge Patch semantics:
//   - Field absent in rawUpdates: keep existing value
//   - Field explicitly set to null in rawUpdates: remove entry (set to nil)
//   - Field set to a value in rawUpdates: use new value
//
// The rawUpdates parameter is required to distinguish between "field omitted" and
// "field: null" since Go's json.Unmarshal sets both to nil.
func mergeAccountingEntries(existing, incoming *mmodel.AccountingEntries, rawUpdates json.RawMessage) *mmodel.AccountingEntries {
	if existing == nil && incoming == nil {
		return nil
	}

	if existing == nil {
		return incoming
	}

	// If no raw updates provided, fall back to simple merge (incoming wins if non-nil)
	if len(rawUpdates) == 0 {
		return mergeAccountingEntriesSimple(existing, incoming)
	}

	// Parse raw JSON to detect which fields are explicitly present
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(rawUpdates, &rawFields); err != nil {
		// If parsing fails, fall back to simple merge
		return mergeAccountingEntriesSimple(existing, incoming)
	}

	merged := &mmodel.AccountingEntries{}

	// Helper to apply merge logic for each field
	applyMerge := func(fieldName string, existingEntry, incomingEntry *mmodel.AccountingEntry) *mmodel.AccountingEntry {
		raw, fieldPresent := rawFields[fieldName]
		if !fieldPresent {
			// Field not in update - keep existing
			return existingEntry
		}

		// Field is present in raw update
		if string(raw) == "null" {
			// Explicit null - remove entry
			return nil
		}

		// Field has a value - use incoming (which was unmarshaled from the same JSON)
		if incomingEntry != nil {
			return incomingEntry
		}

		// Incoming is nil but raw wasn't null - keep existing as fallback
		return existingEntry
	}

	var incomingDirect, incomingHold, incomingCommit, incomingCancel, incomingRevert *mmodel.AccountingEntry
	if incoming != nil {
		incomingDirect = incoming.Direct
		incomingHold = incoming.Hold
		incomingCommit = incoming.Commit
		incomingCancel = incoming.Cancel
		incomingRevert = incoming.Revert
	}

	merged.Direct = applyMerge("direct", existing.Direct, incomingDirect)
	merged.Hold = applyMerge("hold", existing.Hold, incomingHold)
	merged.Commit = applyMerge("commit", existing.Commit, incomingCommit)
	merged.Cancel = applyMerge("cancel", existing.Cancel, incomingCancel)
	merged.Revert = applyMerge("revert", existing.Revert, incomingRevert)

	// Check if all entries are nil - return nil instead of empty struct
	if merged.Direct == nil && merged.Hold == nil && merged.Commit == nil &&
		merged.Cancel == nil && merged.Revert == nil {
		return nil
	}

	return merged
}

// mergeAccountingEntriesSimple performs a simple merge where incoming non-nil values win.
// Used as fallback when raw JSON is not available.
func mergeAccountingEntriesSimple(existing, incoming *mmodel.AccountingEntries) *mmodel.AccountingEntries {
	if incoming == nil {
		return existing
	}

	merged := &mmodel.AccountingEntries{}

	if incoming.Direct != nil {
		merged.Direct = incoming.Direct
	} else if existing != nil {
		merged.Direct = existing.Direct
	}

	if incoming.Hold != nil {
		merged.Hold = incoming.Hold
	} else if existing != nil {
		merged.Hold = existing.Hold
	}

	if incoming.Commit != nil {
		merged.Commit = incoming.Commit
	} else if existing != nil {
		merged.Commit = existing.Commit
	}

	if incoming.Cancel != nil {
		merged.Cancel = incoming.Cancel
	} else if existing != nil {
		merged.Cancel = existing.Cancel
	}

	if incoming.Revert != nil {
		merged.Revert = incoming.Revert
	} else if existing != nil {
		merged.Revert = existing.Revert
	}

	return merged
}
