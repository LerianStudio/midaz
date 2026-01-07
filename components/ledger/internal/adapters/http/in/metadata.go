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
//	@Description	Create a metadata index for the specified entity
//	@Tags			Metadata Indexes
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string							true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header		string							false	"Request ID"
//	@Param			entity_name		path		string							true	"Entity Name"	Enums(transaction, operation, operation_route, transaction_route)
//	@Param			metadata-index	body		mmodel.CreateMetadataIndexInput	true	"Metadata Index Input"
//	@Success		201				{object}	mmodel.MetadataIndex			"Successfully created metadata index"
//	@Failure		400				{object}	mmodel.Error					"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		409				{object}	mmodel.Error					"Conflict: Metadata index already exists"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/settings/metadata-indexes/entities/{entity_name} [post]
func (handler *MetadataIndexHandler) CreateMetadataIndex(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_metadata_index")
	defer span.End()

	// Extract entity_name from path parameter
	entityName := c.Params("entity_name")
	if entityName == "" {
		err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.MetadataIndex{}).Name(), "entity_name")

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

	payload, ok := p.(*mmodel.CreateMetadataIndexInput)
	if !ok {
		err := pkg.ValidateBusinessError(constant.ErrInvalidType, reflect.TypeOf(mmodel.CreateMetadataIndexInput{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to assert payload type", err)

		logger.Errorf("Failed to assert payload type, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Request to create a metadata index: entityName=%s, metadataKey=%s", entityName, payload.MetadataKey)

	metadataIndex, err := handler.MetadataIndexPort.CreateMetadataIndex(ctx, entityName, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create metadata index", err)

		logger.Errorf("Failed to create metadata index, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully created metadata index: entityName=%s, metadataKey=%s", entityName, payload.MetadataKey)

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
//	@Param			entity_name		query		string	false	"Entity Name"	Enums(transaction, operation, operation_route, transaction_route)
//	@Success		200				{object}	[]mmodel.MetadataIndex			"Successfully retrieved metadata indexes"
//	@Failure		400				{object}	mmodel.Error					"Invalid query parameters"
//	@Failure		401				{object}	mmodel.Error					"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error					"Forbidden access"
//	@Failure		500				{object}	mmodel.Error					"Internal server error"
//	@Router			/v1/settings/metadata-indexes [get]
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
//	@Description	Delete a metadata index by entity name and index key
//	@Tags			Metadata Indexes
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header	string	false	"Request ID"
//	@Param			entity_name		path	string	true	"Entity Name"	Enums(transaction, operation, operation_route, transaction_route)
//	@Param			index_key		path	string	true	"Index Key (metadata key, e.g., 'tier')"
//	@Success		204				"Metadata index successfully deleted"
//	@Failure		400				{object}	mmodel.Error	"Invalid input, validation errors"
//	@Failure		401				{object}	mmodel.Error	"Unauthorized access"
//	@Failure		403				{object}	mmodel.Error	"Forbidden access"
//	@Failure		404				{object}	mmodel.Error	"Metadata index not found"
//	@Failure		500				{object}	mmodel.Error	"Internal server error"
//	@Router			/v1/settings/metadata-indexes/entities/{entity_name}/key/{index_key} [delete]
func (handler *MetadataIndexHandler) DeleteMetadataIndex(c *fiber.Ctx) error {
	ctx := c.UserContext()

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_metadata_index")
	defer span.End()

	entityName := c.Params("entity_name")
	if entityName == "" {
		err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.MetadataIndex{}).Name(), "entity_name")

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

	indexKey := c.Params("index_key")
	if indexKey == "" {
		err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.MetadataIndex{}).Name(), "index_key")

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get index key", err)

		logger.Errorf("Failed to get index key, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	// Build full MongoDB index name from the metadata key
	indexName := "metadata." + indexKey + "_1"

	logger.Infof("Request to delete metadata index: entityName=%s, indexKey=%s, indexName=%s", entityName, indexKey, indexName)

	err := handler.MetadataIndexPort.DeleteMetadataIndex(ctx, entityName, indexName)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete metadata index", err)

		logger.Errorf("Failed to delete metadata index, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully deleted metadata index: entityName=%s, indexKey=%s", entityName, indexKey)

	return http.NoContent(c)
}
