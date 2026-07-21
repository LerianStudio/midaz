// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	// OperationHandler struct contains a cqrs use case for managing operations.
)

type OperationHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The two read cores below own the span, imperative query validation, the
// metadata-vs-default branch, the service call and the pagination assembly. They
// take primitive args (parsed UUIDs + the query map) so BOTH transports feed them:
// the Fiber wrappers pull those from *fiber.Ctx (Locals + c.Queries) and the Huma
// handlers (operation_handler_huma.go) pull them from the request envelope. Every
// canonical Midaz error the cores return is rendered by the caller — http.WithError
// on the Fiber path, http.HumaProblem on the Huma path — so the code + HTTP status
// are identical across both transports. Reads only; the command use case is
// untouched.

// getAllOperationsByAccount binds the query imperatively (http.ValidateParameters —
// the SAME binder the Fiber path used), preserves the metadata-vs-default branch,
// then returns the cursor-paginated envelope.
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

// GetAllOperationsByAccount retrieves all operations by account.
func (handler *OperationHandler) GetAllOperationsByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	accountID, err := http.GetUUIDFromLocals(c, "account_id")
	if err != nil {
		return http.WithError(c, err)
	}

	pagination, err := handler.getAllOperationsByAccount(ctx, organizationID, ledgerID, accountID, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}

// GetOperationByAccount retrieves an operation by account.
func (handler *OperationHandler) GetOperationByAccount(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	accountID, err := http.GetUUIDFromLocals(c, "account_id")
	if err != nil {
		return http.WithError(c, err)
	}

	operationID, err := http.GetUUIDFromLocals(c, "operation_id")
	if err != nil {
		return http.WithError(c, err)
	}

	op, err := handler.getOperationByAccount(ctx, organizationID, ledgerID, accountID, operationID)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, op)
}

// UpdateOperation method that patch operation created before
func (handler *OperationHandler) UpdateOperation(p any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	transactionID, err := http.GetUUIDFromLocals(c, "transaction_id")
	if err != nil {
		return http.WithError(c, err)
	}

	operationID, err := http.GetUUIDFromLocals(c, "operation_id")
	if err != nil {
		return http.WithError(c, err)
	}

	payload := p.(*operation.UpdateOperationInput)

	op, err := handler.updateOperation(ctx, organizationID, ledgerID, transactionID, operationID, payload)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, op)
}

// updateOperation is the transport-neutral update core: it logs the safe payload,
// runs command.UpdateOperation, then re-reads the operation via query.GetOperationByID
// (mutable metadata/description only — amounts/accounts/direction/type are immutable).
// Called by BOTH the Fiber wrapper and the Huma shell (operation_handler_huma.go). The
// command use case is untouched (transport-only extraction).
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
