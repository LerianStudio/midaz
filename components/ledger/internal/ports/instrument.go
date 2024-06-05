package ports

import (
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mpostgres"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	i "github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/instrument"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// InstrumentHandler struct contains an instrument use case for managing instrument related operations.
type InstrumentHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateInstrument is a method that creates instrument information.
func (handler *InstrumentHandler) CreateInstrument(a any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	logger.Infof("Initiating create of Instrument with organization ID: %s", organizationID)

	ledgerID := c.Params("ledger_id")
	logger.Infof("Initiating create of Instrument with ledger ID: %s", ledgerID)

	payload := a.(*i.CreateInstrumentInput)
	logger.Infof("Request to create a Instrument with details: %#v", payload)

	instrument, err := handler.Command.CreateInstrument(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), payload)
	if err != nil {
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created Instrument")

	return commonHTTP.Created(c, instrument)
}

// GetAllInstruments is a method that retrieves all Instruments.
func (handler *InstrumentHandler) GetAllInstruments(c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	logger.Infof("Initiating create of Instrument with organization ID: %s", organizationID)

	ledgerID := c.Params("ledger_id")
	logger.Infof("Initiating create of Instrument with ledger ID: %s", ledgerID)

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Instruments by metadata")

		instruments, err := handler.Query.GetAllMetadataInstruments(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			logger.Errorf("Failed to retrieve all Instruments, Error: %s", err.Error())
			return commonHTTP.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Instruments by metadata")

		pagination.SetItems(instruments)

		return commonHTTP.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Instruments ")

	headerParams.Metadata = &bson.M{}

	instruments, err := handler.Query.GetAllInstruments(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		logger.Errorf("Failed to retrieve all Instruments, Error: %s", err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Instruments")

	pagination.SetItems(instruments)

	return commonHTTP.OK(c, pagination)
}

// GetInstrumentByID is a method that retrieves Instrument information by a given id.
func (handler *InstrumentHandler) GetInstrumentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	id := c.Params("id")

	logger := mlog.NewLoggerFromContext(ctx)

	logger.Infof("Initiating retrieval of Instrument with Ledger ID: %s and Instrument ID: %s", ledgerID, id)

	instrument, err := handler.Query.GetInstrumentByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Failed to retrieve Instrument with Ledger ID: %s and Instrument ID: %s, Error: %s", ledgerID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Instrument with Ledger ID: %s and Instrument ID: %s", ledgerID, id)

	return commonHTTP.OK(c, instrument)
}

// UpdateInstrument is a method that updates Instrument information.
func (handler *InstrumentHandler) UpdateInstrument(a any, c *fiber.Ctx) error {
	ctx := c.UserContext()
	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	id := c.Params("id")

	logger.Infof("Initiating update of Instrument with Ledger ID: %s and Instrument ID: %s", ledgerID, id)

	payload := a.(*i.UpdateInstrumentInput)
	logger.Infof("Request to update an Instrument with details: %#v", payload)

	_, err := handler.Command.UpdateInstrumentByID(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id), payload)
	if err != nil {
		logger.Errorf("Failed to update Instrument with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	instrument, err := handler.Query.GetInstrumentByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		logger.Errorf("Failed to get update Instrument with ID: %s, Error: %s", id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Instrument with Ledger ID: %s and Instrument ID: %s", ledgerID, id)

	return commonHTTP.OK(c, instrument)
}

// DeleteInstrumentByID is a method that removes Instrument information by a given ids.
func (handler *InstrumentHandler) DeleteInstrumentByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := mlog.NewLoggerFromContext(ctx)

	organizationID := c.Params("organization_id")
	ledgerID := c.Params("ledger_id")
	id := c.Params("id")

	logger.Infof("Initiating removal of Instrument with Ledger ID: %s and Instrument ID: %s", ledgerID, id)

	if err := handler.Command.DeleteInstrumentByID(ctx, uuid.MustParse(organizationID), uuid.MustParse(ledgerID), uuid.MustParse(id)); err != nil {
		logger.Errorf("Failed to remove Instrument with Ledger ID: %s and Instrument ID: %s, Error: %s", ledgerID, id, err.Error())
		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Instrument with Ledger ID: %s and Instrument ID: %s", ledgerID, id)

	return commonHTTP.NoContent(c)
}
