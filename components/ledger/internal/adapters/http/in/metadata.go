// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/mongo"
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
	OnboardingMongoManager  *tmmongo.Manager
	TransactionMongoManager *tmmongo.Manager
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

func (handler *MetadataIndexHandler) getMongoManager(entityName string) *tmmongo.Manager {
	if _, ok := onboardingEntities[entityName]; ok {
		return handler.OnboardingMongoManager
	}

	if _, ok := transactionEntities[entityName]; ok {
		return handler.TransactionMongoManager
	}

	return nil
}

func (handler *MetadataIndexHandler) contextForEntity(ctx context.Context, entityName string) (context.Context, error) {
	tenantID := tmcore.GetTenantIDContext(ctx)

	mongoManager := handler.getMongoManager(entityName)
	if tenantID == "" {
		if mongoManager != nil {
			return nil, fmt.Errorf("tenant id is required for entity %s", entityName)
		}

		return ctx, nil
	}

	if mongoManager == nil {
		return nil, fmt.Errorf("multi-tenant mongo manager not configured for entity %s", entityName)
	}

	tenantDB, err := mongoManager.GetDatabaseForTenant(ctx, tenantID)
	if err != nil {
		return nil, mapTenantError(err, tenantID)
	}

	// Store in both generic and module-specific context keys.
	ctx = tmcore.ContextWithMongo(ctx, tenantDB)

	// Determine module name based on entity type for module-specific injection.
	if _, ok := onboardingEntities[entityName]; ok {
		ctx = tmcore.ContextWithMB(ctx, constant.ModuleOnboarding, tenantDB)
	} else {
		ctx = tmcore.ContextWithMB(ctx, constant.ModuleTransaction, tenantDB)
	}

	return ctx, nil
}

func (handler *MetadataIndexHandler) contextForRepoGroup(ctx context.Context, onboardingRepo bool) (context.Context, error) {
	tenantID := tmcore.GetTenantIDContext(ctx)
	mongoManager := handler.TransactionMongoManager
	groupName := constant.ModuleTransaction

	if onboardingRepo {
		mongoManager = handler.OnboardingMongoManager
		groupName = constant.ModuleOnboarding
	}

	if tenantID == "" {
		if mongoManager != nil {
			return nil, fmt.Errorf("tenant id is required for %s metadata indexes", groupName)
		}

		return ctx, nil
	}

	if mongoManager == nil {
		return nil, fmt.Errorf("multi-tenant mongo manager not configured for %s metadata indexes", groupName)
	}

	tenantDB, err := mongoManager.GetDatabaseForTenant(ctx, tenantID)
	if err != nil {
		return nil, mapTenantError(err, tenantID)
	}

	// Store in both generic and module-specific context keys.
	ctx = tmcore.ContextWithMongo(ctx, tenantDB)
	ctx = tmcore.ContextWithMB(ctx, groupName, tenantDB)

	return ctx, nil
}

// mapTenantError converts tenant-manager errors into Midaz-specific error types
// so that the caller's http.WithError can map them to the correct HTTP status codes.
func mapTenantError(err error, tenantID string) error {
	var suspErr *tmcore.TenantSuspendedError
	if errors.As(err, &suspErr) {
		return pkg.ForbiddenError{
			Code:    constant.ErrTenantServiceSuspended.Error(),
			Title:   "Service Suspended",
			Message: fmt.Sprintf("service is %s for tenant %s", suspErr.Status, tenantID),
		}
	}

	if errors.Is(err, tmcore.ErrTenantNotFound) {
		return pkg.EntityNotFoundError{
			Code:    constant.ErrTenantNotFound.Error(),
			Title:   "Tenant Not Found",
			Message: fmt.Sprintf("tenant not found: %s", tenantID),
		}
	}

	if tmcore.IsTenantNotProvisionedError(err) {
		return pkg.UnprocessableOperationError{
			Code:    constant.ErrTenantNotProvisioned.Error(),
			Title:   "Tenant Not Provisioned",
			Message: "Database schema not initialized for this tenant. Contact your administrator.",
		}
	}

	return pkg.ServiceUnavailableError{
		Code:    constant.ErrTenantServiceUnavailable.Error(),
		Title:   "Tenant Service Unavailable",
		Message: fmt.Sprintf("failed to resolve tenant %s: %s", tenantID, err.Error()),
	}
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

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get entity name", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get entity name, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	if !isValidEntity(entityName) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidEntityName, "MetadataIndex")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid entity name", err)

		logger.Log(ctx, libLog.LevelError, "Invalid entity name, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	headerParams, err := http.ValidateParameters(c.Queries())
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelError, "Failed to validate query parameters, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.query_params", headerParams, nil)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to set span attributes", err)

		logger.Log(ctx, libLog.LevelError, "Failed to set span attributes, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	payload, ok := p.(*mmodel.CreateMetadataIndexInput)
	if !ok {
		err := pkg.ValidateBusinessError(constant.ErrInvalidType, reflect.TypeOf(mmodel.CreateMetadataIndexInput{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to assert payload type", err)

		logger.Log(ctx, libLog.LevelError, "Failed to assert payload type, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, "Request to create metadata index", libLog.String("entity_name", entityName), libLog.String("metadata_key", payload.MetadataKey))

	repo, collection := handler.getRepoAndCollection(entityName)
	if repo == nil {
		err := fmt.Errorf("metadata index repository not configured for entity %s", entityName)
		libOpentelemetry.HandleSpanError(span, "Metadata repository not configured", err)
		logger.Log(ctx, libLog.LevelError, "Metadata repository not configured, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	repoCtx, err := handler.contextForEntity(ctx, entityName)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant metadata context", err)

		logger.Log(ctx, libLog.LevelError, "Failed to resolve tenant metadata context, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	metadataIndex, err := repo.CreateIndex(repoCtx, collection, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create metadata index", err)

		logger.Log(ctx, libLog.LevelError, "Failed to create metadata index, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	// Set the entity name in the response (repo returns collection name)
	metadataIndex.EntityName = entityName

	logger.Log(ctx, libLog.LevelInfo, "Successfully created metadata index", libLog.String("entity_name", entityName), libLog.String("metadata_key", payload.MetadataKey))

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
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		logger.Log(ctx, libLog.LevelError, "Failed to validate query parameters, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	err = libOpentelemetry.SetSpanAttributesFromValue(span, "app.request.query_params", headerParams, nil)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to set span attributes", err)

		logger.Log(ctx, libLog.LevelError, "Failed to set span attributes, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	// Check if filtering by entity name
	if headerParams.EntityName != nil && *headerParams.EntityName != "" {
		if !isValidEntity(*headerParams.EntityName) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidEntityName, "MetadataIndex")

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid entity name", err)

			logger.Log(ctx, libLog.LevelError, "Invalid entity name, Error: %s", libLog.Err(err))

			return http.WithError(c, err)
		}

		// Return indexes for specific entity
		repo, collection := handler.getRepoAndCollection(*headerParams.EntityName)
		if repo == nil {
			err := fmt.Errorf("metadata index repository not configured for entity %s", *headerParams.EntityName)
			libOpentelemetry.HandleSpanError(span, "Metadata repository not configured", err)
			logger.Log(ctx, libLog.LevelError, "Metadata repository not configured, Error: %s", libLog.Err(err))

			return http.WithError(c, err)
		}

		repoCtx, err := handler.contextForEntity(ctx, *headerParams.EntityName)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant metadata context", err)

			logger.Log(ctx, libLog.LevelError, "Failed to resolve tenant metadata context, Error: %s", libLog.Err(err))

			return http.WithError(c, err)
		}

		indexes, err := repo.FindAllIndexes(repoCtx, collection)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata indexes", err)

			logger.Log(ctx, libLog.LevelError, "Failed to get metadata indexes, Error: %s", libLog.Err(err))

			return http.WithError(c, err)
		}

		// Set entity name in response
		for _, idx := range indexes {
			idx.EntityName = *headerParams.EntityName
		}

		logger.Log(ctx, libLog.LevelInfo, "Successfully retrieved metadata indexes for entity", libLog.String("entity_name", *headerParams.EntityName))

		return http.OK(c, indexes)
	}

	// Return indexes from all entities
	var allIndexes []*mmodel.MetadataIndex

	onboardingCtx, err := handler.contextForRepoGroup(ctx, true)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve onboarding tenant metadata context", err)

		logger.Log(ctx, libLog.LevelError, "Failed to resolve onboarding tenant metadata context, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	transactionCtx, err := handler.contextForRepoGroup(ctx, false)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve transaction tenant metadata context", err)

		logger.Log(ctx, libLog.LevelError, "Failed to resolve transaction tenant metadata context, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	// Fetch from onboarding entities
	for entityName, collection := range onboardingEntities {
		indexes, err := handler.OnboardingMetadataRepo.FindAllIndexes(onboardingCtx, collection)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to get indexes for entity", libLog.String("entity_name", entityName), libLog.Err(err))

			continue
		}

		for _, idx := range indexes {
			idx.EntityName = entityName
			allIndexes = append(allIndexes, idx)
		}
	}

	// Fetch from transaction entities
	for entityName, collection := range transactionEntities {
		indexes, err := handler.TransactionMetadataRepo.FindAllIndexes(transactionCtx, collection)
		if err != nil {
			logger.Log(ctx, libLog.LevelWarn, "Failed to get indexes for entity", libLog.String("entity_name", entityName), libLog.Err(err))

			continue
		}

		for _, idx := range indexes {
			idx.EntityName = entityName
			allIndexes = append(allIndexes, idx)
		}
	}

	logger.Log(ctx, libLog.LevelInfo, "Successfully retrieved all metadata indexes")

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

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get entity name", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get entity name, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	if !isValidEntity(entityName) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidEntityName, "MetadataIndex")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid entity name", err)

		logger.Log(ctx, libLog.LevelError, "Invalid entity name, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	indexKey := c.Params("index_key")
	if indexKey == "" {
		err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, reflect.TypeOf(mmodel.MetadataIndex{}).Name(), "index_key")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get index key", err)

		logger.Log(ctx, libLog.LevelError, "Failed to get index key, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	// Build full MongoDB index name from the metadata key
	indexName := "metadata." + indexKey + "_1"

	logger.Log(ctx, libLog.LevelInfo, "Request to delete metadata index", libLog.String("entity_name", entityName), libLog.String("index_key", indexKey), libLog.String("index_name", indexName))

	repo, collection := handler.getRepoAndCollection(entityName)
	if repo == nil {
		err := fmt.Errorf("metadata index repository not configured for entity %s", entityName)
		libOpentelemetry.HandleSpanError(span, "Metadata repository not configured", err)
		logger.Log(ctx, libLog.LevelError, "Metadata repository not configured, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	repoCtx, err := handler.contextForEntity(ctx, entityName)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant metadata context", err)

		logger.Log(ctx, libLog.LevelError, "Failed to resolve tenant metadata context, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	err = repo.DeleteIndex(repoCtx, collection, indexName)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete metadata index", err)

		logger.Log(ctx, libLog.LevelError, "Failed to delete metadata index, Error: %s", libLog.Err(err))

		return http.WithError(c, err)
	}

	logger.Log(ctx, libLog.LevelInfo, "Successfully deleted metadata index", libLog.String("entity_name", entityName), libLog.String("index_key", indexKey))

	return http.NoContent(c)
}
