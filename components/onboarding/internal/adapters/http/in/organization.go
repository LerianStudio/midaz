package in

import (
	"fmt"
	"os"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
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

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_organization")
	defer span.End()

	payload := p.(*mmodel.CreateOrganizationInput)
	logger.Infof("Request to create an organization with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	organization, err := handler.Command.CreateOrganization(ctx, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to create organization on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created organization: %s", organization)

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

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_organization")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating update of Organization with ID: %s", id.String())

	payload := p.(*mmodel.UpdateOrganizationInput)
	logger.Infof("Request to update an organization with details: %#v", payload)

	err := libOpentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateOrganizationByID(ctx, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update organization on command", err)

		logger.Errorf("Failed to update Organization with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	organizations, err := handler.Query.GetOrganizationByID(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve organization on query", err)

		logger.Errorf("Failed to retrieve Organization with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Organization with ID: %s", id.String())

	return http.OK(c, organizations)
}

// GetOrganizationByID is a method that retrieves Organization information by a given id.
//
//	@Summary		Retrieve a specific organization
//	@Description	Returns detailed information about an organization identified by its UUID
//	@Tags			Organizations
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			id				path		string	true	"Organization ID in UUID format"
//	@Success		200				{object}	mmodel.Organization	"Successfully retrieved organization"
//	@Failure		401				{object}	mmodel.Error		"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error		"Forbidden access"
//	@Failure		404				{object}	mmodel.Error		"Organization not found"
//	@Failure		500				{object}	mmodel.Error		"Internal server error"
//	@Router			/v1/organizations/{id} [get]
func (handler *OrganizationHandler) GetOrganizationByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_organization_by_id")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating retrieval of Organization with ID: %s", id.String())

	organizations, err := handler.Query.GetOrganizationByID(ctx, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve organization on query", err)

		logger.Errorf("Failed to retrieve Organization with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Organization with ID: %s", id.String())

	return http.OK(c, organizations)
}

// GetAllOrganizations is a method that retrieves all Organizations.
//
//	@Summary		List all organizations
//	@Description	Returns a paginated list of organizations, optionally filtered by metadata, date range, and other criteria
//	@Tags			Organizations
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Param			metadata		query		string	false	"JSON string to filter organizations by metadata fields"
//	@Param			limit			query		int		false	"Maximum number of records to return per page"				default(10)	minimum(1)	maximum(100)
//	@Param			page			query		int		false	"Page number for pagination"									default(1)	minimum(1)
//	@Param			start_date		query		string	false	"Filter organizations created on or after this date (format: YYYY-MM-DD)"
//	@Param			end_date		query		string	false	"Filter organizations created on or before this date (format: YYYY-MM-DD)"
//	@Param			sort_order		query		string	false	"Sort direction for results based on creation date"			Enums(asc,desc)
//	@Success		200				{object}	libPostgres.Pagination{items=[]mmodel.Organization,page=int,limit=int}	"Successfully retrieved organizations list"
//	@Failure		400				{object}	mmodel.Error	"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations [get]
func (handler *OrganizationHandler) GetAllOrganizations(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_organizations")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	pagination := libPostgres.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Organizations by metadata")

		organizations, err := handler.Query.GetAllMetadataOrganizations(ctx, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all organizations by metadata", err)

			logger.Errorf("Failed to retrieve all Organizations, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Organizations by metadata")

		pagination.SetItems(organizations)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Organizations ")

	headerParams.Metadata = &bson.M{}

	organizations, err := handler.Query.GetAllOrganizations(ctx, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to retrieve all organizations", err)

		logger.Errorf("Failed to retrieve all Organizations, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Organizations")

	pagination.SetItems(organizations)

	return http.OK(c, pagination)
}

// DeleteOrganizationByID is a method that removes Organization information by a given id.
//
//	@Summary		Delete an organization
//	@Description	Permanently removes an organization identified by its UUID. Note: This operation is not available in production environments.
//	@Tags			Organizations
//	@Param			Authorization	header	string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header	string	false	"Request ID for tracing"
//	@Param			id				path	string	true	"Organization ID in UUID format"
//	@Success		204				{string}	string	"Organization successfully deleted"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden action or not permitted in production environment"
//	@Failure		404				{object}	mmodel.Error	"Organization not found"
//	@Failure		409				{object}	mmodel.Error	"Conflict: Cannot delete organization with dependent resources"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{id} [delete]
func (handler *OrganizationHandler) DeleteOrganizationByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_organization_by_id")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating removal of Organization with ID: %s", id.String())

	if os.Getenv("ENV_NAME") == "production" {
		libOpentelemetry.HandleSpanError(&span, "Failed to remove organization: "+constant.ErrActionNotPermitted.Error(), constant.ErrActionNotPermitted)

		logger.Errorf("Failed to remove Organization with ID: %s in ", id.String())

		err := pkg.ValidateBusinessError(constant.ErrActionNotPermitted, reflect.TypeOf(mmodel.Organization{}).Name())

		return http.WithError(c, err)
	}

	if err := handler.Command.DeleteOrganizationByID(ctx, id); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to remove organization on command", err)

		logger.Errorf("Failed to remove Organization with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Organization with ID: %s", id.String())

	return http.NoContent(c)
}

// CountOrganizations is a method that returns the total count of organizations.
//
//	@Summary		Count total organizations
//	@Description	Returns the total count of organizations as a header without a response body
//	@Tags			Organizations
//	@Param			Authorization	header		string	true	"Authorization Bearer Token with format: Bearer {token}"
//	@Param			X-Request-Id	header		string	false	"Request ID for tracing"
//	@Success		204				{string}	string	"No content with X-Total-Count header containing the count"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Organization not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/metrics/count [head]
func (handler *OrganizationHandler) CountOrganizations(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := libCommons.NewLoggerFromContext(ctx)
	tracer := libCommons.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_organizations")
	defer span.End()

	logger.Infof("Initiating count of all organizations")

	count, err := handler.Query.CountOrganizations(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to count organizations", err)
		logger.Errorf("Failed to count organizations, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully counted organizations: %d", count)

	c.Set(constant.XTotalCount, fmt.Sprintf("%d", count))
	c.Set(constant.ContentLength, "0")

	return http.NoContent(c)
}
