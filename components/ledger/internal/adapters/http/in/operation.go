// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// OperationHandler holds the CQRS use cases that serve the operation resource.
type OperationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The cores below own the span, imperative query validation, the
// metadata-vs-default branch, the service calls and the pagination assembly. They
// take primitive args (parsed UUIDs, the query map, the decoded payload); the Huma
// handlers in operation_handler.go pull those from the request envelope. Every
// canonical Midaz error a core returns is rendered by the caller through
// http.HumaProblem, so the code + HTTP status stay canonical.

// getAllOperationsByAccount binds the query imperatively (http.ValidateParameters),
// applies the metadata-vs-default branch, then returns the cursor-paginated envelope.
func (handler *OperationHandler) getAllOperationsByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_operations_by_account")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		recordSafeQueryAttributes(span, headerParams)

		trans, cur, err := handler.Query.GetAllMetadataOperations(ctx, organizationID, ledgerID, accountID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Operations by account and metadata", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(trans)
		pagination.SetCursor(cur.Next, cur.Prev)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	operations, cur, err := handler.Query.GetAllOperationsByAccount(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Operations by account", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(operations)
	pagination.SetCursor(cur.Next, cur.Prev)

	return pagination, nil
}

// getOperationByAccount retrieves a single operation scoped to an account.
func (handler *OperationHandler) getOperationByAccount(ctx context.Context, organizationID, ledgerID, accountID, operationID uuid.UUID) (*operation.Operation, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_operation_by_account")
	defer span.End()

	op, err := handler.Query.GetOperationByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve Operation by account", err)

		return nil, err
	}

	return op, nil
}

// updateOperation is the transport-neutral update core: it logs the safe payload,
// runs command.UpdateOperation, then re-reads the operation via query.GetOperationByID.
// Only metadata and description are mutable — amounts, accounts, direction and type
// are immutable.
func (handler *OperationHandler) updateOperation(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID, payload *operation.UpdateOperationInput) (*operation.Operation, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_operation")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	logSafePayload(ctx, logger, "Request to update an Operation", payload)

	_, err := handler.Command.UpdateOperation(ctx, organizationID, ledgerID, transactionID, operationID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update Operation on command", err)

		return nil, err
	}

	op, err := handler.Query.GetOperationByID(ctx, organizationID, ledgerID, transactionID, operationID)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Operation on query", err)

		return nil, err
	}

	return op, nil
}
