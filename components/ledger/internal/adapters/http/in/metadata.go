// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"fmt"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/mongo"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
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
		return nil, mapTenantError(ctx, err, tenantID)
	}

	// Store in both generic and module-specific context keys.
	ctx = tmcore.ContextWithMB(ctx, tenantDB)

	// Determine module name based on entity type for module-specific injection.
	if _, ok := onboardingEntities[entityName]; ok {
		ctx = tmcore.ContextWithMB(ctx, tenantDB, constant.ModuleOnboarding)
	} else {
		ctx = tmcore.ContextWithMB(ctx, tenantDB, constant.ModuleTransaction)
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
		return nil, mapTenantError(ctx, err, tenantID)
	}

	// Store in both generic and module-specific context keys.
	ctx = tmcore.ContextWithMB(ctx, tenantDB)
	ctx = tmcore.ContextWithMB(ctx, tenantDB, groupName)

	return ctx, nil
}

// mapTenantError converts tenant-manager errors into Midaz-specific error types
// so that the caller's HumaProblem can map them to the correct HTTP status codes.
func mapTenantError(ctx context.Context, err error, tenantID string) error {
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

	// err is deliberately not interpolated: this is a 503, and the ledger publishes
	// >=500 message text to clients. The tenant ID stays because it is the caller's
	// own. The cause is recorded on the span so it is not lost — without this the
	// only record of WHY tenant resolution failed would be gone.
	libOpentelemetry.HandleSpanError(trace.SpanFromContext(ctx), "Failed to resolve tenant database", err)

	return pkg.ServiceUnavailableError{
		Code:    constant.ErrTenantServiceUnavailable.Error(),
		Title:   "Tenant Service Unavailable",
		Message: fmt.Sprintf("failed to resolve tenant %s", tenantID),
	}
}

// isValidEntity checks if the entity name is valid for metadata index operations.
func isValidEntity(entityName string) bool {
	_, onboarding := onboardingEntities[entityName]
	_, transaction := transactionEntities[entityName]

	return onboarding || transaction
}

// createMetadataIndex is the transport-agnostic core behind the create terminal in
// metadata_handler.go. ctx must already carry the tenant id (the auth+tenant
// middleware chain populated it); this core resolves the tenant db, validates the
// entity name and query params, and creates the index. Every error it returns is a
// canonical Midaz error, which HumaProblem renders at a fixed code and HTTP status.
func (handler *MetadataIndexHandler) createMetadataIndex(ctx context.Context, entityName string, queries map[string]string, payload *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_metadata_index")
	defer span.End()

	if entityName == "" {
		err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityMetadataIndex, "entity_name")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get entity name", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get entity name", libLog.Err(err))

		return nil, err
	}

	if !isValidEntity(entityName) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidEntityName, constant.EntityMetadataIndex)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid entity name", err)
		logger.Log(ctx, libLog.LevelError, "Invalid entity name", libLog.Err(err))

		return nil, err
	}

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)
		logger.Log(ctx, libLog.LevelError, "Failed to validate query parameters", libLog.Err(err))

		return nil, err
	}

	span.SetAttributes(
		attribute.String("app.request.entity_name", entityName),
		attribute.Bool("app.request.query_params.has_metadata", headerParams.Metadata != nil),
	)

	repo, collection := handler.getRepoAndCollection(entityName)
	if repo == nil {
		err := fmt.Errorf("metadata index repository not configured for entity %s", entityName)
		libOpentelemetry.HandleSpanError(span, "Metadata repository not configured", err)
		logger.Log(ctx, libLog.LevelError, "Metadata repository not configured", libLog.Err(err))

		return nil, err
	}

	repoCtx, err := handler.contextForEntity(ctx, entityName)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant metadata context", err)
		logger.Log(ctx, libLog.LevelError, "Failed to resolve tenant metadata context", libLog.Err(err))

		return nil, err
	}

	metadataIndex, err := repo.CreateIndex(repoCtx, collection, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create metadata index", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create metadata index", libLog.Err(err))

		return nil, err
	}

	// Set the entity name in the response (repo returns collection name)
	metadataIndex.EntityName = entityName

	return metadataIndex, nil
}

// getAllMetadataIndexes is the transport-agnostic core behind the list terminal in
// metadata_handler.go. It validates the query params, optionally filters by
// entity_name, and returns the flat index slice verbatim — a JSON array, not a
// pagination envelope. Every error it returns is a canonical Midaz error.
func (handler *MetadataIndexHandler) getAllMetadataIndexes(ctx context.Context, queries map[string]string) ([]*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_metadata_indexes")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)
		logger.Log(ctx, libLog.LevelError, "Failed to validate query parameters", libLog.Err(err))

		return nil, err
	}

	span.SetAttributes(
		attribute.Bool("app.request.query_params.has_entity_name", headerParams.EntityName != nil),
		attribute.Bool("app.request.query_params.has_metadata", headerParams.Metadata != nil),
	)

	// Check if filtering by entity name
	if headerParams.EntityName != nil && *headerParams.EntityName != "" {
		if !isValidEntity(*headerParams.EntityName) {
			err := pkg.ValidateBusinessError(constant.ErrInvalidEntityName, constant.EntityMetadataIndex)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid entity name", err)

			logger.Log(ctx, libLog.LevelError, "Invalid entity name", libLog.Err(err))

			return nil, err
		}

		// Return indexes for specific entity
		repo, collection := handler.getRepoAndCollection(*headerParams.EntityName)
		if repo == nil {
			err := fmt.Errorf("metadata index repository not configured for entity %s", *headerParams.EntityName)
			libOpentelemetry.HandleSpanError(span, "Metadata repository not configured", err)
			logger.Log(ctx, libLog.LevelError, "Metadata repository not configured", libLog.Err(err))

			return nil, err
		}

		repoCtx, err := handler.contextForEntity(ctx, *headerParams.EntityName)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant metadata context", err)

			logger.Log(ctx, libLog.LevelError, "Failed to resolve tenant metadata context", libLog.Err(err))

			return nil, err
		}

		indexes, err := repo.FindAllIndexes(repoCtx, collection)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get metadata indexes", err)

			logger.Log(ctx, libLog.LevelError, "Failed to get metadata indexes", libLog.Err(err))

			return nil, err
		}

		// Set entity name in response
		for _, idx := range indexes {
			idx.EntityName = *headerParams.EntityName
		}

		return indexes, nil
	}

	// Return indexes from all entities
	var allIndexes []*mmodel.MetadataIndex

	onboardingCtx, err := handler.contextForRepoGroup(ctx, true)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve onboarding tenant metadata context", err)

		logger.Log(ctx, libLog.LevelError, "Failed to resolve onboarding tenant metadata context", libLog.Err(err))

		return nil, err
	}

	transactionCtx, err := handler.contextForRepoGroup(ctx, false)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve transaction tenant metadata context", err)

		logger.Log(ctx, libLog.LevelError, "Failed to resolve transaction tenant metadata context", libLog.Err(err))

		return nil, err
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

	return allIndexes, nil
}

// deleteMetadataIndex is the transport-agnostic core behind the delete terminal in
// metadata_handler.go. ctx must already carry the tenant id. Every error it returns
// is a canonical Midaz error, which HumaProblem renders at a fixed code and HTTP
// status.
func (handler *MetadataIndexHandler) deleteMetadataIndex(ctx context.Context, entityName, indexKey string) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_metadata_index")
	defer span.End()

	if entityName == "" {
		err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityMetadataIndex, "entity_name")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get entity name", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get entity name", libLog.Err(err))

		return err
	}

	if !isValidEntity(entityName) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidEntityName, constant.EntityMetadataIndex)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid entity name", err)
		logger.Log(ctx, libLog.LevelError, "Invalid entity name", libLog.Err(err))

		return err
	}

	if indexKey == "" {
		err := pkg.ValidateBusinessError(constant.ErrInvalidPathParameter, constant.EntityMetadataIndex, "index_key")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get index key", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get index key", libLog.Err(err))

		return err
	}

	// Build full MongoDB index name from the metadata key
	indexName := "metadata." + indexKey + "_1"

	repo, collection := handler.getRepoAndCollection(entityName)
	if repo == nil {
		err := fmt.Errorf("metadata index repository not configured for entity %s", entityName)
		libOpentelemetry.HandleSpanError(span, "Metadata repository not configured", err)
		logger.Log(ctx, libLog.LevelError, "Metadata repository not configured", libLog.Err(err))

		return err
	}

	repoCtx, err := handler.contextForEntity(ctx, entityName)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant metadata context", err)
		logger.Log(ctx, libLog.LevelError, "Failed to resolve tenant metadata context", libLog.Err(err))

		return err
	}

	if err := repo.DeleteIndex(repoCtx, collection, indexName); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete metadata index", err)
		logger.Log(ctx, libLog.LevelError, "Failed to delete metadata index", libLog.Err(err))

		return err
	}

	return nil
}
