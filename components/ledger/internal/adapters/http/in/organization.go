package in

import (
	"go.mongodb.org/mongo-driver/bson"
	"os"
	"reflect"

	"github.com/LerianStudio/midaz/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/mmodel"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"github.com/LerianStudio/midaz/pkg/mpostgres"
	"github.com/LerianStudio/midaz/pkg/net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// OrganizationHandler struct contains an organization use case for managing organization related operations.
type OrganizationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateOrganization is a method that creates Organization information.
//
//	@Summary		Create an Organization
//	@Description	Create an Organization with the input payload
//	@Tags			Organizations
//	@Accept			json
//	@Produce		json
//	@Param			organization	body		mmodel.CreateOrganizationInput	true	"Organization Input"
//	@Param			Midaz-Id		header		string							false	"Request ID"
//	@Success		200				{object}	mmodel.Organization
//	@Router			/v1/organizations [post]
func (handler *OrganizationHandler) CreateOrganization(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_organization")
	defer span.End()

	payload := p.(*mmodel.CreateOrganizationInput)
	logger.Infof("Request to create an organization with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	organization, err := handler.Command.CreateOrganization(ctx, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create organization on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created organization: %s", organization)

	return http.Created(c, organization)
}

// UpdateOrganization is a method that updates Organization information.
//
//	@Summary		Update an Organization
//	@Description	Update an Organization with the input payload
//	@Tags			Organizations
//	@Accept			json
//	@Produce		json
//	@Param			id				path		string							true	"Organization ID"
//	@Param			organization	body		mmodel.UpdateOrganizationInput	true	"Organization Input"
//	@Param			Midaz-Id		header		string							false	"Request ID"
//	@Success		200				{object}	mmodel.Organization
//	@Router			/v1/organizations/{id} [patch]
func (handler *OrganizationHandler) UpdateOrganization(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_organization")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating update of Organization with ID: %s", id.String())

	payload := p.(*mmodel.UpdateOrganizationInput)
	logger.Infof("Request to update an organization with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateOrganizationByID(ctx, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update organization on command", err)

		logger.Errorf("Failed to update Organization with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	organizations, err := handler.Query.GetOrganizationByID(ctx, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve organization on query", err)

		logger.Errorf("Failed to retrieve Organization with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Organization with ID: %s", id.String())

	return http.OK(c, organizations)
}

// GetOrganizationByID is a method that retrieves Organization information by a given id.
//
//	@Summary		Get an Organization by ID
//	@Description	Get an Organization with the input ID
//	@Tags			Organizations
//	@Produce		json
//	@Param			id			path		string	true	"Organization ID"
//	@Param			Midaz-Id	header		string	false	"Request ID"
//	@Success		200			{object}	mmodel.Organization
//	@Router			/v1/organizations/{id} [get]
func (handler *OrganizationHandler) GetOrganizationByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_organization_by_id")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating retrieval of Organization with ID: %s", id.String())

	organizations, err := handler.Query.GetOrganizationByID(ctx, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve organization on query", err)

		logger.Errorf("Failed to retrieve Organization with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Organization with ID: %s", id.String())

	return http.OK(c, organizations)
}

// GetAllOrganizations is a method that retrieves all Organizations.
//
//	@Summary		Get all Organizations
//	@Description	Get all Organizations with the input metadata or without metadata
//	@Tags			Organizations
//	@Produce		json
//	@Param			metadata	query		string	false	"Metadata"
//	@Param			Midaz-Id	header		string	false	"Request ID"
//	@Success		200			{object}	mpostgres.Pagination{items=[]mmodel.Organization}
//	@Router			/v1/organizations [get]
func (handler *OrganizationHandler) GetAllOrganizations(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_organizations")
	defer span.End()

	headerParams := http.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Organizations by metadata")

		organizations, err := handler.Query.GetAllMetadataOrganizations(ctx, *headerParams)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve all organizations by metadata", err)

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
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all organizations", err)

		logger.Errorf("Failed to retrieve all Organizations, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Organizations")

	pagination.SetItems(organizations)

	return http.OK(c, pagination)
}

// DeleteOrganizationByID is a method that removes Organization information by a given id.
//
//	@Summary		Delete an Organization by ID
//	@Description	Delete an Organization with the input ID
//	@Tags			Organizations
//	@Param			id			path	string	true	"Organization ID"
//	@Param			Midaz-Id	header	string	false	"Request ID"
//	@Success		204
//	@Router			/v1/organizations/{id} [delete]
func (handler *OrganizationHandler) DeleteOrganizationByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := pkg.NewLoggerFromContext(ctx)
	tracer := pkg.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_organization_by_id")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating removal of Organization with ID: %s", id.String())

	if os.Getenv("ENV_NAME") == "production" {
		mopentelemetry.HandleSpanError(&span, "Failed to remove organization: "+constant.ErrActionNotPermitted.Error(), constant.ErrActionNotPermitted)

		logger.Errorf("Failed to remove Organization with ID: %s in ", id.String())

		err := pkg.ValidateBusinessError(constant.ErrActionNotPermitted, reflect.TypeOf(mmodel.Organization{}).Name())

		return http.WithError(c, err)
	}

	if err := handler.Command.DeleteOrganizationByID(ctx, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove organization on command", err)

		logger.Errorf("Failed to remove Organization with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Organization with ID: %s", id.String())

	return http.NoContent(c)
}
