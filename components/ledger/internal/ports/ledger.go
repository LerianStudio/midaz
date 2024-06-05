package ports

import (
	"os"

	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	l "github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
)

// LedgerHandler struct contains a ledger use case for managing ledger related operations.
type LedgerHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateLedger is a method that creates Ledger information.
func (handler *LedgerHandler) CreateLedger(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")

	payload := i.(*l.CreateLedgerInput)
	logger.Infof("Request to create an ledger with details: %#v", payload)

	ledger, err := handler.Command.CreateLedger(ctx, organizationID, payload)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created ledger")

	return commonHTTP.Created(c, ledger)
}

// GetLedgerByID is a method that retrieves Ledger information by a given id.
func (handler *LedgerHandler) GetLedgerByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := mlog.NewLoggerFromContext(ctx)

	id := c.Params("id")
	logger.Infof("Initiating retrieval of Ledger with ID: %s", id)

	organizationID := c.Params("organization_id")

	ledger, err := handler.Query.GetLedgerByID(ctx, organizationID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Ledger with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Ledger with ID: %s", id)

	return commonHTTP.OK(c, ledger)
}

// GetAllLedgers is a method that retrieves all ledgers.
func (handler *LedgerHandler) GetAllLedgers(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Ledgers by metadata")

		ledgers, err := handler.Query.GetAllMetadataLedgers(ctx, organizationID, *headerParams)
		if err != nil {
			logger.Errorf("Failed to retrieve all Ledgers, Error: %s", err.Error())
			return commonHTTP.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Ledgers by metadata")

		pagination.SetItems(ledgers)

		return commonHTTP.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Ledgers ")

	headerParams.Metadata = &bson.M{}

	ledgers, err := handler.Query.GetAllLedgers(ctx, organizationID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Ledgers, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Ledgers")

	pagination.SetItems(ledgers)

	return commonHTTP.OK(c, pagination)
}

// UpdateLedger is a method that updates Ledger information.
func (handler *LedgerHandler) UpdateLedger(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	id := c.Params("id")
	logger.Infof("Initiating update of Ledger with ID: %s", id)

	organizationID := c.Params("organization_id")

	payload := p.(*l.UpdateLedgerInput)
	logger.Infof("Request to update an Ledger with details: %#v", payload)

	_, err := handler.Command.UpdateLedgerByID(ctx, organizationID, id, payload)
	if err != nil {
		logger.Errorf("Failed to update Ledger with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	ledger, err := handler.Query.GetLedgerByID(ctx, organizationID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Ledger with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Ledger with ID: %s", id)

	return commonHTTP.OK(c, ledger)
}

// DeleteLedgerByID is a method that removes Ledger information by a given id.
func (handler *LedgerHandler) DeleteLedgerByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := mlog.NewLoggerFromContext(ctx)

	id := c.Params("id")
	logger.Infof("Initiating removal of Ledeger with ID: %s", id)

	organizationID := c.Params("organization_id")

	if os.Getenv("ENV_NAME") == "production" {
		logger.Errorf("Failed to remove Ledger with ID: %s in ", id)

		return commonHTTP.BadRequest(c, &fiber.Map{
			"code":    "0008",
			"message": "Action not allowed.",
		})
	}

	if err := handler.Command.DeleteLedgerByID(ctx, organizationID, id); err != nil {
		logger.Errorf("Failed to remove Ledeger with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Ledeger with ID: %s", id)

	return commonHTTP.NoContent(c)
}
