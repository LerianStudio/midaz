// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

type TransactionRouteHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The create/get/update/delete/getAll methods below own the span, the service call,
// the transaction-route side-effects (accounting-route cache write on create/update,
// cache delete on delete, the created metric) and the success/failure logs. They take
// primitive args (parsed UUIDs, the decoded *Input, the query map) so BOTH transports
// feed them: the Fiber wrappers pull those from *fiber.Ctx (Locals + the WithBody-
// decoded payload + c.Queries()) and the Huma handlers (transaction_route_handler_huma.go)
// pull them from the request envelope. Every canonical Midaz error the cores return is
// rendered by the caller — http.WithError on the Fiber path, http.HumaProblem on the
// Huma path — so code + HTTP status are identical across both transports. Unlike
// operation-route there is NO merge-patch landmine: the body is a normal typed decode,
// so the cores take the decoded *Input, no rawBody.

// createTransactionRoute owns the span + service call + cache write + created metric
// for an already-decoded payload.
func (handler *TransactionRouteHandler) createTransactionRoute(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, metricFactory := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_transaction_route")
	defer span.End()

	recordSafePayloadAttributes(span, payload)
	logSafePayload(ctx, logger, "Request to create a transaction route", payload)

	transactionRoute, err := handler.Command.CreateTransactionRoute(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction route", err)

		return nil, err
	}

	if err := handler.Command.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction route cache", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create transaction route cache", libLog.Err(err), libLog.String("transaction_route_id", transactionRoute.ID.String()))
	}

	if err := metricFactory.RecordTransactionRouteCreated(
		ctx,
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to record transaction route created metric", err)
	}

	return transactionRoute, nil
}

// getTransactionRouteByID retrieves a single transaction route.
func (handler *TransactionRouteHandler) getTransactionRouteByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_transaction_route_by_id")
	defer span.End()

	transactionRoute, err := handler.Query.GetTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to get transaction route", err)
		logger.Log(ctx, libLog.LevelError, "Failed to get transaction route", libLog.Err(err), libLog.String("transaction_route_id", id.String()))

		return nil, err
	}

	return transactionRoute, nil
}

// updateTransactionRoute owns the span + service call + cache write for an
// already-decoded payload.
func (handler *TransactionRouteHandler) updateTransactionRoute(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdateTransactionRouteInput) (*mmodel.TransactionRoute, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_transaction_route")
	defer span.End()

	recordSafePayloadAttributes(span, payload)
	logSafePayload(ctx, logger, "Request to update a transaction route", payload)

	transactionRoute, err := handler.Command.UpdateTransactionRoute(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update transaction route", err)
		logger.Log(ctx, libLog.LevelError, "Failed to update transaction route", libLog.Err(err), libLog.String("transaction_route_id", id.String()))

		return nil, err
	}

	if err := handler.Command.CreateAccountingRouteCache(ctx, transactionRoute); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create transaction route cache", err)
		logger.Log(ctx, libLog.LevelError, "Failed to create transaction route cache", libLog.Err(err), libLog.String("transaction_route_id", id.String()))
	}

	return transactionRoute, nil
}

// deleteTransactionRouteByID owns the span + service call + cache delete.
func (handler *TransactionRouteHandler) deleteTransactionRouteByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_transaction_route_by_id")
	defer span.End()

	if err := handler.Command.DeleteTransactionRouteByID(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete transaction route", err)
		logger.Log(ctx, libLog.LevelError, "Failed to delete transaction route", libLog.Err(err), libLog.String("transaction_route_id", id.String()))

		return err
	}

	if err := handler.Command.DeleteTransactionRouteCache(ctx, organizationID, ledgerID, id); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete transaction route cache", err)
		logger.Log(ctx, libLog.LevelError, "Failed to delete transaction route cache", libLog.Err(err), libLog.String("transaction_route_id", id.String()))
	}

	return nil
}

// getAllTransactionRoutes binds the query map imperatively (http.ValidateParameters
// — the SAME binder the Fiber path used) so a bad query yields the canonical 400,
// then returns the assembled cursor-pagination envelope.
func (handler *TransactionRouteHandler) getAllTransactionRoutes(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_transaction_routes")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)
		logger.Log(ctx, libLog.LevelError, "Failed to validate query parameters", libLog.Err(err))

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		transactionRoutes, cur, err := handler.Query.GetAllMetadataTransactionRoutes(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all transaction routes by metadata", err)
			logger.Log(ctx, libLog.LevelError, "Failed to retrieve all transaction routes by metadata", libLog.Err(err))

			return http.Pagination{}, err
		}

		pagination.SetItems(transactionRoutes)
		pagination.SetCursor(cur.Next, cur.Prev)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	recordSafeQueryAttributes(span, headerParams)

	transactionRoutes, cur, err := handler.Query.GetAllTransactionRoutes(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all transaction routes", err)
		logger.Log(ctx, libLog.LevelError, "Failed to retrieve all transaction routes", libLog.Err(err))

		return http.Pagination{}, err
	}

	pagination.SetItems(transactionRoutes)
	pagination.SetCursor(cur.Next, cur.Prev)

	return pagination, nil
}

// --- Fiber wrappers (thin) ----------------------------------------------------
//
// These stay so the legacy Fiber unit/integration tests keep exercising the handler
// methods directly; each pulls the transport inputs from *fiber.Ctx (Locals set by
// ParseUUIDPathParameters, the WithBody-decoded payload as `i`) and delegates to the
// shared core.

// Create a Transaction Route.
func (handler *TransactionRouteHandler) CreateTransactionRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	transactionRoute, err := handler.createTransactionRoute(ctx, organizationID, ledgerID, i.(*mmodel.CreateTransactionRouteInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.Created(c, transactionRoute)
}

// Get a Transaction Route by ID.
func (handler *TransactionRouteHandler) GetTransactionRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "transaction_route_id")
	if err != nil {
		return http.WithError(c, err)
	}

	transactionRoute, err := handler.getTransactionRouteByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, transactionRoute)
}

// Update a Transaction Route.
func (handler *TransactionRouteHandler) UpdateTransactionRoute(i any, c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "transaction_route_id")
	if err != nil {
		return http.WithError(c, err)
	}

	transactionRoute, err := handler.updateTransactionRoute(ctx, organizationID, ledgerID, id, i.(*mmodel.UpdateTransactionRouteInput))
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, transactionRoute)
}

// Delete a Transaction Route by ID.
func (handler *TransactionRouteHandler) DeleteTransactionRouteByID(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	id, err := http.GetUUIDFromLocals(c, "transaction_route_id")
	if err != nil {
		return http.WithError(c, err)
	}

	if err := handler.deleteTransactionRouteByID(ctx, organizationID, ledgerID, id); err != nil {
		return http.WithError(c, err)
	}

	return http.NoContent(c)
}

// Get all Transaction Routes.
func (handler *TransactionRouteHandler) GetAllTransactionRoutes(c *fiber.Ctx) error {
	ctx := c.UserContext()

	organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
	if err != nil {
		return http.WithError(c, err)
	}

	ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
	if err != nil {
		return http.WithError(c, err)
	}

	pagination, err := handler.getAllTransactionRoutes(ctx, organizationID, ledgerID, c.Queries())
	if err != nil {
		return http.WithError(c, err)
	}

	return http.OK(c, pagination)
}
