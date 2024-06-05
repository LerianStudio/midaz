package ports

import (
	"os"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	o "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	"github.com/gofiber/fiber/v2"
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

	logger := mlog.NewLoggerFromContext(ctx)

	payload := p.(*o.CreateOrganizationInput)
	logger.Infof("Request to create an organization with details: %#v", payload)

	organization, err := handler.Command.CreateOrganization(ctx, payload)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created organization: %s", organization)

	return commonHTTP.Created(c, organization)
}

// UpdateOrganization is a method that updates Organization information.
func (handler *OrganizationHandler) UpdateOrganization(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	id := c.Params("id")
	logger.Infof("Initiating update of Organization with ID: %s", id)

	payload := p.(*o.UpdateOrganizationInput)
	logger.Infof("Request to update an organization with details: %#v", payload)

	_, err := handler.Command.UpdateOrganizationByID(ctx, id, payload)
	if err != nil {
		logger.Errorf("Failed to update Organization with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	organizations, err := handler.Query.GetOrganizationByID(ctx, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Organization with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Organization with ID: %s", id)

	return commonHTTP.OK(c, organizations)
}

// GetOrganizationByID is a method that retrieves Organization information by a given id.
func (handler *OrganizationHandler) GetOrganizationByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	id := c.Params("id")

	logger := mlog.NewLoggerFromContext(ctx)

	logger.Infof("Initiating retrieval of Organization with ID: %s", id)

	organizations, err := handler.Query.GetOrganizationByID(ctx, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Organization with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Organization with ID: %s", id)

	return commonHTTP.OK(c, organizations)
}

// GetAllOrganizations is a method that retrieves all Organizations.
func (handler *OrganizationHandler) GetAllOrganizations(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Organizations by metadata")

		organizations, err := handler.Query.GetAllMetadataOrganizations(ctx, *headerParams)
		if err != nil {
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

	logger := mlog.NewLoggerFromContext(ctx)

	id := c.Params("id")
	logger.Infof("Initiating removal of Organization with ID: %s", id)

	if os.Getenv("ENV_NAME") == "production" {
		logger.Errorf("Failed to remove Organization with ID: %s in ", id)

		return commonHTTP.BadRequest(c, &fiber.Map{
			"code":    "0008",
			"message": "Action not allowed.",
		})
	}

	if err := handler.Command.DeleteOrganizationByID(ctx, id); err != nil {
		logger.Errorf("Failed to remove Organization with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Organization with ID: %s", id)

	return commonHTTP.NoContent(c)
}
