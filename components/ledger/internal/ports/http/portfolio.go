package http

import (
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	p "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// PortfolioHandler struct contains a portfolio use case for managing portfolio related operations.
type PortfolioHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreatePortfolio is a method that creates portfolio information.
func (handler *PortfolioHandler) CreatePortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)

	logger.Infof("Initiating create of Portfolio with ledger ID: %s", ledgerID.String())

	payload := i.(*p.CreatePortfolioInput)

	logger.Infof("Request to create a Portfolio with details: %#v", payload)

	portfolio, err := handler.Command.CreatePortfolio(ctx, organizationID, ledgerID, payload)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created Portfolio")

	return commonHTTP.Created(c, portfolio)
}

// GetAllPortfolios is a method that retrieves all Portfolios.
func (handler *PortfolioHandler) GetAllPortfolios(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	logger.Infof("Get Portfolios with Organization: %s and Ledger ID: %s", organizationID.String(), ledgerID.String())

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Portfolios by metadata")

		portfolios, err := handler.Query.GetAllMetadataPortfolios(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			logger.Errorf("Failed to retrieve all Portfolios, Error: %s", err.Error())
			return commonHTTP.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Portfolios by metadata")

		pagination.SetItems(portfolios)

		return commonHTTP.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Portfolios")

	headerParams.Metadata = &bson.M{}

	portfolios, err := handler.Query.GetAllPortfolio(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Portfolios, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Portfolios")

	pagination.SetItems(portfolios)

	return commonHTTP.OK(c, pagination)
}

// GetPortfolioByID is a method that retrieves Portfolio information by a given id.
func (handler *PortfolioHandler) GetPortfolioByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger := mlog.NewLoggerFromContext(ctx)

	logger.Infof("Initiating retrieval of Portfolio with Organization: %s Ledger ID: %s and Portfolio ID: %s", organizationID.String(), ledgerID.String(), id.String())

	portfolio, err := handler.Query.GetPortfolioByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Portfolio with Ledger ID: %s and Portfolio ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Portfolio with Ledger ID: %s and Portfolio ID: %s", ledgerID.String(), id.String())

	return commonHTTP.OK(c, portfolio)
}

// UpdatePortfolio is a method that updates Portfolio information.
func (handler *PortfolioHandler) UpdatePortfolio(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating update of Portfolio with Organization: %s Ledger ID: %s and Portfolio ID: %s", organizationID.String(), ledgerID.String(), id.String())

	payload := i.(*p.UpdatePortfolioInput)
	logger.Infof("Request to update an Portfolio with details: %#v", payload)

	_, err := handler.Command.UpdatePortfolioByID(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		logger.Errorf("Failed to update Portfolio with ID: %s, Error: %s", id.String(), err.Error())
		return commonHTTP.WithError(c, err)
	}

	portfolio, err := handler.Query.GetPortfolioByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Portfolio with Ledger ID: %s and Portfolio ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Portfolio with Ledger ID: %s and Portfolio ID: %s", ledgerID.String(), id.String())

	return commonHTTP.OK(c, portfolio)
}

// DeletePortfolioByID is a method that removes Portfolio information by a given ids.
func (handler *PortfolioHandler) DeletePortfolioByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)
	ledgerID := c.Locals("ledger_id").(uuid.UUID)
	id := c.Locals("id").(uuid.UUID)

	logger.Infof("Initiating removal of Portfolio with Organization: %s Ledger ID: %s and Portfolio ID: %s", organizationID.String(), ledgerID.String(), id.String())

	if err := handler.Command.DeletePortfolioByID(ctx, organizationID, ledgerID, id); err != nil {
		logger.Errorf("Failed to remove Portfolio with Ledger ID: %s and Portfolio ID: %s, Error: %s", ledgerID.String(), id.String(), err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Portfolio with Ledger ID: %s and Portfolio ID: %s", ledgerID.String(), id.String())

	return commonHTTP.NoContent(c)
}
