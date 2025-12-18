package in

import (
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
)

// MetadataIndexHandler handles HTTP requests for metadata index operations.
type MetadataIndexHandler struct {
	MetadataIndexPort mbootstrap.MetadataIndexPort
}

// CreateMetadataIndex creates a new metadata index.
//
//	@Summary		Create Metadata Index
//	@Description	Create a metadata index with the input payload
//	@Tags			Metadata Indexes
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string							false	"Request ID"
//	@Param			organization_id	path		string							true	"Organization ID"
//	@Param			ledger_id		path		string							true	"Ledger ID"
//	@Param			metadata-index	body		mmodel.CreateMetadataIndexInput	true	"Metadata Index Input"
//	@Success		201				{object}	mmodel.MetadataIndex			"Successfully created metadata index"
//	@Failure		400				{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		409				{object}	mmodel.Error					"Conflict: Metadata index already exists"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/metadata-indexes [post]
func (handler *MetadataIndexHandler) CreateMetadataIndex(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_metadata_index")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to set span attributes", err)

		logger.Errorf("Failed to set span attributes, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	payload := p.(*mmodel.CreateMetadataIndexInput)
	logger.Infof("Request to create a metadata index: entityName=%s, metadataKey=%s", payload.EntityName, payload.MetadataKey)

	metadataIndex, err := handler.MetadataIndexPort.CreateMetadataIndex(ctx, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create metadata index", err)

		logger.Errorf("Failed to create metadata index, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created metadata index")

	return http.Created(c, metadataIndex)
}

// GetAllMetadataIndexes retrieves all metadata indexes.
//
//	@Summary		Get all Metadata Indexes
//	@Description	Get all metadata indexes, optionally filtered by entity name
//	@Tags			Metadata Indexes
//	@Produce		json
//	@Param			Authorization	header		string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string	false	"Request ID"
//	@Param			organization_id	path		string	true	"Organization ID"
//	@Param			ledger_id		path		string	true	"Ledger ID"
//	@Param			entity_name		query		string	false	"Entity Name"	Enums(transaction, operation, operation_route, transaction_route)
//	@Param			limit			query		int		false	"Limit"			default(10)
//	@Param			start_date		query		string	false	"Start Date"	example "2021-01-01"
//	@Param			end_date		query		string	false	"End Date"		example "2021-01-01"
//	@Param			sort_order		query		string	false	"Sort Order"	enum(asc,desc)
//	@Param			cursor			query		string	false	"Cursor"
//	@Success		200				{object}	[]mmodel.MetadataIndex			"Successfully retrieved metadata indexes"
//	@Failure		400				{object}	mmodel.Error					"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/metadata-indexes [get]
func (handler *MetadataIndexHandler) GetAllMetadataIndexes(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_metadata_indexes")
	defer span.End()

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate query parameters", err)

		logger.Errorf("Failed to validate query parameters, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromStruct(&span, "app.request.query_params", headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to set span attributes", err)

		logger.Errorf("Failed to set span attributes, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	if headerParams.EntityName != nil && *headerParams.EntityName != "" {
		if !mmodel.IsValidMetadataIndexEntity(*headerParams.EntityName) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidEntityName, "MetadataIndex")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid entity name", err)

			logger.Errorf("Invalid entity name, Error: %s", err.Error())

			return http.WithError(c, err)
		}
	}

	metadataIndexes, err := handler.MetadataIndexPort.GetAllMetadataIndexes(ctx, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get all metadata indexes", err)

		logger.Errorf("Failed to get all metadata indexes, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully retrieved all metadata indexes")

	return http.OK(c, metadataIndexes)
}

// DeleteMetadataIndex deletes a metadata index.
//
//	@Summary		Delete Metadata Index
//	@Description	Delete a metadata index with the input payload
//	@Tags			Metadata Indexes
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header	string	false	"Request ID"
//	@Param			organization_id	path	string	true	"Organization ID"
//	@Param			ledger_id		path	string	true	"Ledger ID"
//	@Param			index_name		path	string	true	"Index Name"
//	@Param			entity_name		query	string	true	"Entity Name"	Enums(transaction, operation, operation_route, transaction_route)
//	@Success		204				{string}	string			"Metadata index successfully deleted"
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Metadata index not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/organizations/{organization_id}/ledgers/{ledger_id}/metadata-indexes/{index_name} [delete]
func (handler *MetadataIndexHandler) DeleteMetadataIndex(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_metadata_index")
	defer span.End()

	indexName := c.Locals("index_name").(string)
	entityName := c.Query("entity_name")

	if indexName == "" {
		err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.MetadataIndex{}).Name(), "index_name")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get index name", err)

		logger.Errorf("Failed to get index name, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	if entityName == "" {
		err := pkg.ValidateBusinessError(constant.ErrInvalidEntityName, reflect.TypeOf(mmodel.MetadataIndex{}).Name(), "entity_name")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get entity name", err)

		logger.Errorf("Failed to get entity name, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	if !mmodel.IsValidMetadataIndexEntity(entityName) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidEntityName, "MetadataIndex")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid entity name", err)

		logger.Errorf("Invalid entity name, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	err := handler.MetadataIndexPort.DeleteMetadataIndex(ctx, entityName, indexName)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete metadata index", err)

		logger.Errorf("Failed to delete metadata index, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	return http.NoContent(c)
}
