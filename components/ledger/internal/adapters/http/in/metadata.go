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

// onboardingEntities maps onboarding entity names to their MongoDB collection names.
var onboardingEntities = map[string]string{
	"organization": "organization",
	"ledger":       "ledger",
	"segment":      "segment",
	"account":      "account",
	"portfolio":    "portfolio",
	"asset":        "asset",
	"account_type": "account_type",
}

// transactionEntities maps transaction entity names to their MongoDB collection names.
var transactionEntities = map[string]string{
	"transaction":       "transaction",
	"operation":         "operation",
	"operation_route":   "operation_route",
	"transaction_route": "transaction_route",
}

// MetadataIndexHandler handles HTTP requests for metadata index operations.
// It routes requests to the appropriate repository based on the entity type.
type MetadataIndexHandler struct {
	OnboardingMetadataRepo  mbootstrap.MetadataIndexRepository
	TransactionMetadataRepo mbootstrap.MetadataIndexRepository
}

// getRepoAndCollection returns the appropriate repository and collection name for an entity.
// Returns nil repository if entity is not valid.
func (handler *MetadataIndexHandler) getRepoAndCollection(entityName string) (mbootstrap.MetadataIndexRepository, string) {
	if collection, ok := onboardingEntities[entityName]; ok {
		return handler.OnboardingMetadataRepo, collection
	}

	if collection, ok := transactionEntities[entityName]; ok {
		return handler.TransactionMetadataRepo, collection
	}

	return nil, ""
}

// isValidEntity checks if the entity name is valid for metadata index operations.
func isValidEntity(entityName string) bool {
	_, onboarding := onboardingEntities[entityName]
	_, transaction := transactionEntities[entityName]

	return onboarding || transaction
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
//	@Param			entity_name		path		string							true	"Entity Name"	Enums(organization, ledger, segment, account, portfolio, asset, account_type, transaction, operation, operation_route, transaction_route)
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

	if !isValidEntity(entityName) {
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

	repo, collection := handler.getRepoAndCollection(entityName)

	metadataIndex, err := repo.CreateIndex(ctx, collection, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create metadata index", err)

		logger.Errorf("Failed to create metadata index, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	// Set the entity name in the response (repo returns collection name)
	metadataIndex.EntityName = entityName

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
//	@Param			entity_name		query		string	false	"Entity Name"	Enums(organization, ledger, segment, account, portfolio, asset, account_type, transaction, operation, operation_route, transaction_route)
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

	// Check if filtering by entity name
	if headerParams.EntityName != nil && *headerParams.EntityName != "" {
		if !isValidEntity(*headerParams.EntityName) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidEntityName, "MetadataIndex")

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Invalid entity name", err)

			logger.Errorf("Invalid entity name, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		// Return indexes for specific entity
		repo, collection := handler.getRepoAndCollection(*headerParams.EntityName)

		indexes, err := repo.FindAllIndexes(ctx, collection)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata indexes", err)

			logger.Errorf("Failed to get metadata indexes, Error: %s", err.Error())

			return http.WithError(c, err)
		}

		// Set entity name in response
		for _, idx := range indexes {
			idx.EntityName = *headerParams.EntityName
		}

		logger.Infof("Successfully retrieved metadata indexes for entity: %s", *headerParams.EntityName)

		return http.OK(c, indexes)
	}

	// Return indexes from all entities
	var allIndexes []*mmodel.MetadataIndex

	// Fetch from onboarding entities
	for entityName, collection := range onboardingEntities {
		indexes, err := handler.OnboardingMetadataRepo.FindAllIndexes(ctx, collection)
		if err != nil {
			logger.Warnf("Failed to get indexes for %s: %v", entityName, err)

			continue
		}

		for _, idx := range indexes {
			idx.EntityName = entityName
			allIndexes = append(allIndexes, idx)
		}
	}

	// Fetch from transaction entities
	for entityName, collection := range transactionEntities {
		indexes, err := handler.TransactionMetadataRepo.FindAllIndexes(ctx, collection)
		if err != nil {
			logger.Warnf("Failed to get indexes for %s: %v", entityName, err)

			continue
		}

		for _, idx := range indexes {
			idx.EntityName = entityName
			allIndexes = append(allIndexes, idx)
		}
	}

	logger.Infof("Successfully retrieved all metadata indexes")

	return http.OK(c, allIndexes)
}

// DeleteMetadataIndex deletes a metadata index.
//
//	@Summary		Delete Metadata Index
//	@Description	Delete a metadata index by entity name and index key
//	@Tags			Metadata Indexes
//	@Produce		json
//	@Param			Authorization	header	string	true	"Authorization Bearer Token"
//	@Param			X-Request-Id	header	string	false	"Request ID"
//	@Param			entity_name		path	string	true	"Entity Name"	Enums(organization, ledger, segment, account, portfolio, asset, account_type, transaction, operation, operation_route, transaction_route)
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

	if !isValidEntity(entityName) {
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

	repo, collection := handler.getRepoAndCollection(entityName)

	err := repo.DeleteIndex(ctx, collection, indexName)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to delete metadata index", err)

		logger.Errorf("Failed to delete metadata index, Error: %s", err.Error())

		return http.WithError(c, err)
	}

	logger.Infof("Successfully deleted metadata index: entityName=%s, indexKey=%s", entityName, indexKey)

	return http.NoContent(c)
}
