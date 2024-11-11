package http

import (
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"os"
	"reflect"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/common"

	cn "github.com/LerianStudio/midaz/common/constant"
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

	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_ledger")
	defer span.End()

	logger := common.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)

	payload := i.(*l.CreateLedgerInput)
	logger.Infof("Request to create an ledger with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return commonHTTP.WithError(c, err)
	}

	ledger, err := handler.Command.CreateLedger(ctx, organizationID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create ledger on command", err)

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully created ledger")

	return commonHTTP.Created(c, ledger)
}

// GetLedgerByID is a method that retrieves Ledger information by a given id.
func (handler *LedgerHandler) GetLedgerByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_ledger_by_id")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating retrieval of Ledger with ID: %s", id.String())

	organizationID := c.Locals("organization_id").(uuid.UUID)

	ledger, err := handler.Query.GetLedgerByID(ctx, organizationID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve ledger on query", err)

		logger.Errorf("Failed to retrieve Ledger with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Ledger with ID: %s", id.String())

	return commonHTTP.OK(c, ledger)
}

// GetAllLedgers is a method that retrieves all ledgers.
func (handler *LedgerHandler) GetAllLedgers(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_ledgers")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)

	headerParams := commonHTTP.ValidateParameters(c.Queries())

	pagination := mpostgres.Pagination{
		Limit: headerParams.Limit,
		Page:  headerParams.Page,
	}

	if headerParams.Metadata != nil {
		logger.Infof("Initiating retrieval of all Ledgers by metadata")

		ledgers, err := handler.Query.GetAllMetadataLedgers(ctx, organizationID, *headerParams)
		if err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to retrieve all ledgers by metadata", err)

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
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all ledgers on query", err)

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

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_ledger")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating update of Ledger with ID: %s", id.String())

	organizationID := c.Locals("organization_id").(uuid.UUID)

	payload := p.(*l.UpdateLedgerInput)
	logger.Infof("Request to update an Ledger with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return commonHTTP.WithError(c, err)
	}

	_, err = handler.Command.UpdateLedgerByID(ctx, organizationID, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update ledger on command", err)

		logger.Errorf("Failed to update Ledger with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	ledger, err := handler.Query.GetLedgerByID(ctx, organizationID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve ledger on query", err)

		logger.Errorf("Failed to retrieve Ledger with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully updated Ledger with ID: %s", id.String())

	return commonHTTP.OK(c, ledger)
}

// DeleteLedgerByID is a method that removes Ledger information by a given id.
func (handler *LedgerHandler) DeleteLedgerByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_ledger_by_id")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating removal of Ledeger with ID: %s", id.String())

	organizationID := c.Locals("organization_id").(uuid.UUID)

	if os.Getenv("ENV_NAME") == "production" {
		mopentelemetry.HandleSpanError(&span, "Failed to remove ledger on command", cn.ErrActionNotPermitted)

		logger.Errorf("Failed to remove Ledger with ID: %s in ", id.String())

		err := common.ValidateBusinessError(cn.ErrActionNotPermitted, reflect.TypeOf(l.Ledger{}).Name())

		return commonHTTP.WithError(c, err)
	}

	if err := handler.Command.DeleteLedgerByID(ctx, organizationID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove ledger on command", err)

		logger.Errorf("Failed to remove Ledeger with ID: %s, Error: %s", id.String(), err.Error())

		return commonHTTP.WithError(c, err)
	}

	logger.Infof("Successfully removed Ledeger with ID: %s", id.String())

	return commonHTTP.NoContent(c)
}
