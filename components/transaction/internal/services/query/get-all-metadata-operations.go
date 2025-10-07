// Package query implements read operations (queries) for the transaction service.
// This file contains query implementation.

package query

import (
	"context"
	"errors"
	"reflect"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
)

// GetAllMetadataOperations retrieves operations filtered by metadata criteria.
//
// Metadata-first query: Searches MongoDB for matching metadata, then fetches operations
// from PostgreSQL. Returns only operations that match metadata filters.
//
// Query flow: MongoDB → PostgreSQL (filter by metadata first)
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - accountID: UUID of the account
//   - filter: Query parameters with metadata filters
//
// Returns:
//   - []*operation.Operation: Array of operations with metadata
//   - libHTTP.CursorPagination: Pagination cursor info
//   - error: Business error if query fails
//
// OpenTelemetry: Creates span "query.get_all_metadata_operations"
func (uc *UseCase) GetAllMetadataOperations(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_metadata_operations")
	defer span.End()

	logger.Infof("Retrieving operations")

	metadata, err := uc.MetadataRepo.FindList(ctx, reflect.TypeOf(operation.Operation{}).Name(), filter)
	if err != nil || metadata == nil {
		err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operations on repo by metadata", err)

		logger.Warnf("Error getting operations on repo by metadata: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	oper, cur, err := uc.OperationRepo.FindAllByAccount(ctx, organizationID, ledgerID, accountID, &filter.OperationType, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting operations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operations on repo", err)

			logger.Warnf("Error getting operations on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get operations on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	var operations []*operation.Operation

	for _, o := range oper {
		if data, ok := metadataMap[o.ID]; ok {
			o.Metadata = data
			operations = append(operations, o)
		}
	}

	return operations, cur, nil
}
