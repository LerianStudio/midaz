package in

import (
	"os"
	"reflect"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/constant"
	"github.com/LerianStudio/midaz/common/mmodel"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/common/net/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/query"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
)

// LedgerHandler struct contains a ledger use case for managing ledger related operations.
type LedgerHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// CreateLedger is a method that creates Ledger information.
//
//	@Summary		Create a Ledger
//	@Description	Create a Ledger with the input payload
//	@Tags			Ledgers
//	@Accept			json
//	@Produce		json
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			ledger			body		mmodel.CreateLedgerInput	true	"Ledger Input"
//	@Param			Midaz-Id		header		string						false	"Request ID"
//	@Success		200				{object}	mmodel.Ledger
//	@Router			/v1/organizations/{organization_id}/ledgers [post]
func (handler *LedgerHandler) CreateLedger(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_ledger")
	defer span.End()

	logger := common.NewLoggerFromContext(ctx)

	organizationID := c.Locals("organization_id").(uuid.UUID)

	payload := i.(*mmodel.CreateLedgerInput)
	logger.Infof("Request to create an ledger with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	ledger, err := handler.Command.CreateLedger(ctx, organizationID, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to create ledger on command", err)

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created ledger")

	return http.Created(c, ledger)
}

// GetLedgerByID is a method that retrieves Ledger information by a given id.
//
//	@Summary		Get a Ledger by ID
//	@Description	Get a Ledger with the input ID
//	@Tags			Ledgers
//	@Produce		json
//	@Param			id			path		string	true	"Ledger ID"
//	@Param			Midaz-Id	header		string	false	"Request ID"
//	@Success		200			{object}	mmodel.Ledger
//	@Router			/v1/organizations/{organization_id}/ledgers/{id} [get]
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

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved Ledger with ID: %s", id.String())

	return http.OK(c, ledger)
}

// GetAllLedgers is a method that retrieves all ledgers.
//
//	@Summary		Get all Ledgers
//	@Description	Get all Ledgers with the input metadata or without metadata
//	@Tags			Ledgers
//	@Produce		json
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			id				path		string	true	"Ledger ID"
//	@Param			Midaz-Id		header		string	false	"Request ID"
//	@Success		200				{object}	mpostgres.Pagination{items=[]mmodel.Ledger}
//	@Router			/v1/organizations/{organization_id}/ledgers [get]
func (handler *LedgerHandler) GetAllLedgers(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_ledgers")
	defer span.End()

	organizationID := c.Locals("organization_id").(uuid.UUID)

	headerParams := http.ValidateParameters(c.Queries())

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

			return http.WithError(c, err)
		}

		logger.Infof("Successfully retrieved all Ledgers by metadata")

		pagination.SetItems(ledgers)

		return http.OK(c, pagination)
	}

	logger.Infof("Initiating retrieval of all Ledgers ")

	headerParams.Metadata = &bson.M{}

	ledgers, err := handler.Query.GetAllLedgers(ctx, organizationID, *headerParams)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve all ledgers on query", err)

		logger.Errorf("Failed to retrieve all Ledgers, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all Ledgers")

	pagination.SetItems(ledgers)

	return http.OK(c, pagination)
}

// UpdateLedger is a method that updates Ledger information.
//
//	@Summary		Update a Ledger
//	@Description	Update a Ledger with the input payload
//	@Tags			Ledgers
//	@Accept			json
//	@Produce		json
//	@Param			organization_id	path		string						true	"Organization ID"
//	@Param			id				path		string						true	"Ledger ID"
//	@Param			ledger			body		mmodel.UpdateLedgerInput	true	"Ledger Input"
//	@Param			Midaz-Id		header		string						false	"Request ID"
//	@Success		200				{object}	mmodel.Ledger
//	@Router			/v1/organizations/{organization_id}/ledgers/{id} [patch]
func (handler *LedgerHandler) UpdateLedger(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_ledger")
	defer span.End()

	id := c.Locals("id").(uuid.UUID)
	logger.Infof("Initiating update of Ledger with ID: %s", id.String())

	organizationID := c.Locals("organization_id").(uuid.UUID)

	payload := p.(*mmodel.UpdateLedgerInput)
	logger.Infof("Request to update a Ledger with details: %#v", payload)

	err := mopentelemetry.SetSpanAttributesFromStruct(&span, "payload", payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert payload to JSON string", err)

		return http.WithError(c, err)
	}

	_, err = handler.Command.UpdateLedgerByID(ctx, organizationID, id, payload)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to update ledger on command", err)

		logger.Errorf("Failed to update Ledger with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	ledger, err := handler.Query.GetLedgerByID(ctx, organizationID, id)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to retrieve ledger on query", err)

		logger.Errorf("Failed to retrieve Ledger with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully updated Ledger with ID: %s", id.String())

	return http.OK(c, ledger)
}

// DeleteLedgerByID is a method that removes Ledger information by a given id.
//
//	@Summary		Delete a Ledger by ID
//	@Description	Delete a Ledger with the input ID
//	@Tags			Ledgers
//	@Param			organization_id	path	string	true	"Organization ID"
//	@Param			id				path	string	true	"Ledger ID"
//	@Param			Midaz-Id		header	string	false	"Request ID"
//	@Success		204
//	@Router			/v1/organizations/{organization_id}/ledgers/{id} [delete]
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
		mopentelemetry.HandleSpanError(&span, "Failed to remove ledger on command", constant.ErrActionNotPermitted)

		logger.Errorf("Failed to remove Ledger with ID: %s in ", id.String())

		err := common.ValidateBusinessError(constant.ErrActionNotPermitted, reflect.TypeOf(mmodel.Ledger{}).Name())

		return http.WithError(c, err)
	}

	if err := handler.Command.DeleteLedgerByID(ctx, organizationID, id); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to remove ledger on command", err)

		logger.Errorf("Failed to remove Ledeger with ID: %s, Error: %s", id.String(), err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully removed Ledeger with ID: %s", id.String())

	return http.NoContent(c)
}
