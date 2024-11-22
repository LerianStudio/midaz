package http

import (
	"os"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/services/query"
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
func (handler *OrganizationHandler) CreateOrganization(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_organization")
	defer span.End()

	payload := p.(*mmodel.CreateOrganizationInput)
	logger.Infof("Request to create an organization with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return commonHTTP.WithError(c, err)
	}

	organization, err := handler.Command.CreateOrganization(ctx, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create organization on command", err)

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created organization: %s", organization)

	return commonHTTP.Created(c, organization)
}

// UpdateOrganization is a method that updates Organization information.
func (handler *OrganizationHandler) UpdateOrganization(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_organization")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating update of Organization with ID: %s", id.String())

	payload := p.(*mmodel.UpdateOrganizationInput)
	logger.Infof("Request to update an organization with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return commonHTTP.WithError(c, err)
	}

	_, err = handler.Command.UpdateOrganizationByID(ctx, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update organization on command", err)

		logger.Errorf("Failed to update Organization with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	organizations, err := handler.Query.GetOrganizationByID(ctx, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve organization on query", err)

		logger.Errorf("Failed to retrieve Organization with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Organization with ID: %s", id.String())

	return commonHTTP.OK(c, organizations)
}

// GetOrganizationByID is a method that retrieves Organization information by a given id.
func (handler *OrganizationHandler) GetOrganizationByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_organization_by_id")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating retrieval of Organization with ID: %s", id.String())

	organizations, err := handler.Query.GetOrganizationByID(ctx, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve organization on query", err)

		logger.Errorf("Failed to retrieve Organization with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Organization with ID: %s", id.String())

	return commonHTTP.OK(c, organizations)
}

// GetAllOrganizations is a method that retrieves all Organizations.
func (handler *OrganizationHandler) GetAllOrganizations(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_organizations")
	defer span.End()

	headerParams := commonHTTP.ValidateParameters(c.Queries())

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

			return commonHTTP.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Organizations by metadata")

		pagination.SetItems(organizations)

		return commonHTTP.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Organizations ")

	headerParams.Metadata = &bson.M{}

	organizations, err := handler.Query.GetAllOrganizations(ctx, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all organizations", err)

		logger.Errorf("Failed to retrieve all Organizations, Error: %s", err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Organizations")

	pagination.SetItems(organizations)

	return commonHTTP.OK(c, pagination)
}

// DeleteOrganizationByID is a method that removes Organization information by a given id.
func (handler *OrganizationHandler) DeleteOrganizationByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_organization_by_id")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating removal of Organization with ID: %s", id.String())

	if os.Getenv("ENV_NAME") == "production" {
		mopentelemetry.HandleSpanError(&span, "Failed to remove organization: "+cn.ErrActionNotPermitted.Error(), cn.ErrActionNotPermitted)

		logger.Errorf("Failed to remove Organization with ID: %s in ", id.String())

		err := common.ValidateBusinessError(cn.ErrActionNotPermitted, reflect.TypeOf(mmodel.Organization{}).Name())

		return commonHTTP.WithError(c, err)
	}

	if err := handler.Command.DeleteOrganizationByID(ctx, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove organization on command", err)

		logger.Errorf("Failed to remove Organization with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Organization with ID: %s", id.String())

	return commonHTTP.NoContent(c)
}
