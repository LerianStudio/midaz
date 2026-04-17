// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"fmt"
	"os"

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
)

// OrganizationHandler struct contains an organization use case for managing organization related operations.
type OrganizationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateOrganization is a method that creates Organization information.
//
//	@Summary		Create a new organization
//	@Description	Creates a new organization with the provided details including legal name, legal document, and optional address information
//	@Tags			Organizations
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			organization	body		mmodel.CreateOrganizationInput	true	"Organization details including legal name, legal document, and optional address information"
//	@Success		201				{object}	mmodel.Organization				"Successfully created organization"
//	@Failure		400				{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations [post]
func (handler *OrganizationHandler) CreateOrganization(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_organization")
	defer span.End()

	payload := p.(*mmodel.CreateOrganizationInput)
	logSafePayload(ctx, logger, "Request to create an organization", payload)
	recordSafePayloadAttributes(span, payload)

	organization, err := handler.Command.CreateOrganization(ctx, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create organization on command", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create organization", libLog.Err(err))

		return http.WithError(c, err)
	}

	span.SetAttributes(attribute.String("app.organization.id", organization.ID))

	logger.Log(ctx, libLog.LevelInfo, "Successfully created organization with ID: ", libLog.String("id", organization.ID))

	return http.Created(c, organization)
}

// UpdateOrganization is a method that updates Organization information.
//
//	@Summary		Update an existing organization
//	@Description	Updates an organization's information such as legal name, address, or status. Only supplied fields will be updated.
//	@Tags			Organizations
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string							false	"Request ID for tracing"
//	@Param			id				path		string							true	"Organization ID in UUID format"
//	@Param			organization	body		mmodel.UpdateOrganizationInput	true	"Organization fields to update. Only supplied fields will be modified."
//	@Success		200				{object}	mmodel.Organization				"Successfully updated organization"
//	@Failure		400				{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		404				{object}	mmodel.Error					"Organization not found"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{id} [patch]
func (handler *OrganizationHandler) UpdateOrganization(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_organization")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := p.(*mmodel.UpdateOrganizationInput)
	logSafePayload(ctx, logger, "Request to update an organization", payload)
	recordSafePayloadAttributes(span, payload)

	organization, err := handler.Command.UpdateOrganizationByID(ctx, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update organization on command", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update organization", libLog.Err(err))

		return http.WithError(c, err)
	}

	return http.OK(c, organization)
}

// GetOrganizationByID is a method that retrieves Organization information by a given id.
//
//	@Summary		Retrieve a specific organization
//	@Description	Returns detailed information about an organization identified by its UUID
//	@Tags			Organizations
//	@Produce		json
//	@Param			Authorization	header		string				true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string				false	"Request ID for tracing"
//	@Param			id				path		string				true	"Organization ID in UUID format"
//	@Success		200				{object}	mmodel.Organization	"Successfully retrieved organization"
//	@Failure		401				{object}	mmodel.Error		"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error		"Forbidden access"
//	@Failure		404				{object}	mmodel.Error		"Organization not found"
//	@Failure		500				{object}	mmodel.Error		"Internal server error"
//	@Router			/v1/organizations/{id} [get]
func (handler *OrganizationHandler) GetOrganizationByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_organization_by_id")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating retrieval of Organization with ID: %s", id.String()))

	organizations, err := handler.Query.GetOrganizationByID(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve organization on query", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve Organization with ID: %s, Error: %s", id.String(), err.Error()))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully retrieved Organization with ID: %s", id.String()))

	return http.OK(c, organizations)
}

// GetAllOrganizations is a method that retrieves all Organizations.
//
//	@Summary		List all organizations
//	@Description	Returns a paginated list of organizations, optionally filtered by metadata, date range, and other criteria
//	@Tags			Organizations
//	@Produce		json
//	@Param			Authorization	header		string																	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string																	false	"Request ID for tracing"
//	@Param			metadata		query		string																	false	"JSON string to filter organizations by metadata fields"
//	@Param			limit			query		int																		false	"Maximum number of records to return per page"	default(10)	minimum(1)	maximum(100)
//	@Param			page			query		int																		false	"Page number for pagination"					default(1)	minimum(1)
//	@Param			start_date		query		string																	false	"Filter organizations created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string																	false	"Filter organizations created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order			query		string																	false	"Sort direction for results based on creation date"	Enums(asc,desc)
//	@Param			legal_name			query		string																	false	"Filter organizations by legal name (case-insensitive, prefix match)"	maxLength(256)
//	@Param			doing_business_as	query		string																	false	"Filter organizations by doing business as name (case-insensitive, prefix match)"	maxLength(256)
//	@Param			status				query		string																	false	"Filter organizations by status"	Enums(ACTIVE, INACTIVE)
//	@Param			legal_document		query		string																	false	"Filter organizations by legal document (exact match)"
//	@Success		200					{object}	http.Pagination{items=[]mmodel.Organization}	"Successfully retrieved organizations list"
//	@Failure		400					{object}	mmodel.Error															"Invalid query parameters"
//	@Failure		401					{object}	mmodel.Error															"Unauthorized access"
//	@Failure		403					{object}	mmodel.Error															"Forbidden access"
//	@Failure		500					{object}	mmodel.Error															"Internal server error"
//	@Router			/v1/organizations [get]
func (handler *OrganizationHandler) GetAllOrganizations(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_organizations")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to validate query parameters, Error: %s", err.Error()))

		return http.WithError(c, err)
	}

	if headerParams.Status != nil {
		validStatuses := map[string]bool{"ACTIVE": true, "INACTIVE": true}
		if !validStatuses[*headerParams.Status] {
			err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityOrganization, "status")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid status value", err)

			return http.WithError(c, err)
		}
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		if headerParams.HasNameFilters() {
			err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityOrganization, "metadata cannot be combined with name filters (legal_name, doing_business_as)")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters: metadata and name filters are mutually exclusive", err)

			return http.WithError(c, err)
		}

		logger.Log(ctx, libLog.LevelInfo, "Initiating retrieval of all Organizations by metadata")

		organizations, err := handler.Query.GetAllMetadataOrganizations(ctx, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all organizations by metadata", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve all Organizations, Error: %s", err.Error()))

			return http.WithError(c, err)
		}

		logger.Log(ctx, libLog.LevelInfo, "Successfully retrieved all Organizations by metadata")

		pagination.SetItems(organizations)

		return http.OK(c, pagination)
	}

	logger.Log(ctx, libLog.LevelInfo, "Initiating retrieval of all Organizations ")

	headerParams.Metadata = &bson.M{}

	organizations, err := handler.Query.GetAllOrganizations(ctx, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all organizations", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to retrieve all Organizations, Error: %s", err.Error()))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, "Successfully retrieved all Organizations")

	pagination.SetItems(organizations)

	return http.OK(c, pagination)
}

// DeleteOrganizationByID is a method that removes Organization information by a given id.
//
//	@Summary		Delete an organization
//	@Description	Permanently removes an organization identified by its UUID. Note: This operation is not available in production environments.
//	@Tags			Organizations
//	@Param			Authorization	header		string			true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string			false	"Request ID for tracing"
//	@Param			id				path		string			true	"Organization ID in UUID format"
//	@Success		204				"Organization successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden action or not permitted in production environment"
//	@Failure		404				{object}	mmodel.Error	"Organization not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Cannot delete organization with dependent resources"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{id} [delete]
func (handler *OrganizationHandler) DeleteOrganizationByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_organization_by_id")
	defer span.End()

	id, err := http.GetUUIDFromLocals(c, "id")
	if err != nil {
		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Initiating removal of Organization with ID: %s", id.String()))

	if os.Getenv("ENV_NAME") == "production" {
		err := pkg.ValidateBusinessError(constant.ErrActionNotPermitted, constant.EntityOrganization)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to remove organization in production environment", err)

		logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Failed to remove Organization with ID: %s in production", id.String()))

		return http.WithError(c, err)
	}

	if err := handler.Command.DeleteOrganizationByID(ctx, id); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to remove organization on command", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to remove Organization with ID: %s, Error: %s", id.String(), err.Error()))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully removed Organization with ID: %s", id.String()))

	return http.NoContent(c)
}

// CountOrganizations is a method that returns the total count of organizations.
//
//	@Summary		Count total organizations
//	@Description	Returns the total count of organizations as a header without a response body
//	@Tags			Organizations
//	@Param			Authorization	header		string			true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string			false	"Request ID for tracing"
//	@Success		204				"No content with X-Total-Count header containing the count"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/metrics/count [head]
func (handler *OrganizationHandler) CountOrganizations(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_organizations")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Initiating count of all organizations")

	count, err := handler.Query.CountOrganizations(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count organizations", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to count organizations, Error: %s", err.Error()))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Successfully counted organizations: %d", count))

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
