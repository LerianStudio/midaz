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

// GetAllOperations retrieves a paginated list of operations with metadata.
//
// Fetches operations from PostgreSQL with cursor pagination, then enriches with MongoDB metadata.
// Returns empty array if no operations found.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - organizationID: UUID of the organization
//   - ledgerID: UUID of the ledger
//   - transactionID: UUID of the transaction (filters operations by transaction)
//   - filter: Query parameters (cursor pagination, sorting, date range)
//
// Returns:
//   - []*operation.Operation: Array of operations with metadata
//   - libHTTP.CursorPagination: Pagination cursor info
//   - error: Business error if query fails
//
// OpenTelemetry: Creates span "query.get_all_operations"
func (uc *UseCase) GetAllOperations(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_operations")
	defer span.End()

	logger.Infof("Retrieving operations by account")

	op, cur, err := uc.OperationRepo.FindAll(ctx, organizationID, ledgerID, transactionID, filter.ToCursorPagination())
	if err != nil {
		logger.Errorf("Error getting all operations on repo: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get all operations on repo", err)

			logger.Warnf("Error getting all operations on repo: %v", err)

			return nil, libHTTP.CursorPagination{}, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get all operations on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(op) == 0 {
		return op, cur, nil
	}

	operationIDs := make([]string, len(op))
	for i, o := range op {
		operationIDs[i] = o.ID
	}

	metadata, err := uc.MetadataRepo.FindByEntityIDs(ctx, reflect.TypeOf(operation.Operation{}).Name(), operationIDs)
	if err != nil {
		err := pkg.ValidateBusinessError(constant.ErrNoOperationsFound, reflect.TypeOf(operation.Operation{}).Name())

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to get metadata on mongodb operation", err)

		logger.Warnf("Error getting metadata on mongodb operation: %v", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	metadataMap := make(map[string]map[string]any, len(metadata))

	for _, meta := range metadata {
		metadataMap[meta.EntityID] = meta.Data
	}

	for i := range op {
		if data, ok := metadataMap[op[i].ID]; ok {
			op[i].Metadata = data
		}
	}

	return op, cur, nil
}
