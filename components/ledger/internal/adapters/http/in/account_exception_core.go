// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v2"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// AccountExceptionHandler struct contains the account-exception use cases for managing the
// exceptions that carve auditable holes in an account block.
type AccountExceptionHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createAccountException/updateAccountException/... cores below own the span and the
// service call. They take primitive args — parsed UUIDs, the already-decoded payload, the
// query map — so nothing transport-shaped reaches them; the handlers in
// account_exception_handler.go pull those out of the request envelope. Every canonical Midaz
// error a core returns is rendered by its caller via http.HumaProblem, which fixes the code +
// HTTP status.

// createAccountException owns the span + service call for an already-decoded payload. Body
// decode+validation happens BEFORE this core, in the handler, via http.DecodeAndValidate.
func (handler *AccountExceptionHandler) createAccountException(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, payload *mmodel.CreateAccountExceptionInput) (*mmodel.AccountException, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account_exception")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	exception, err := handler.Command.CreateAccountException(ctx, organizationID, ledgerID, accountID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create account exception on command", err)

		return nil, err
	}

	return exception, nil
}

// getAllAccountExceptions binds the query map imperatively via http.ValidateParameters, then
// delegates to the page-based listing use case, which owns the empty-page 0504 decision. A bad
// query yields the canonical 400.
func (handler *AccountExceptionHandler) getAllAccountExceptions(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_account_exceptions")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	_, pagination, err := handler.Query.GetAllAccountExceptions(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all account exceptions on query", err)

		return http.Pagination{}, err
	}

	return pagination, nil
}

// getAccountExceptionByID retrieves a single account exception by its scoped UUID.
func (handler *AccountExceptionHandler) getAccountExceptionByID(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) (*mmodel.AccountException, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_exception_by_id")
	defer span.End()

	exception, err := handler.Query.GetAccountExceptionByID(ctx, organizationID, ledgerID, accountID, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve account exception on query", err)

		return nil, err
	}

	return exception, nil
}

// updateAccountException owns the span + service call for an already-decoded PATCH payload.
func (handler *AccountExceptionHandler) updateAccountException(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID, payload *mmodel.UpdateAccountExceptionInput) (*mmodel.AccountException, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_account_exception")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	exception, err := handler.Command.UpdateAccountException(ctx, organizationID, ledgerID, accountID, id, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update account exception on command", err)

		return nil, err
	}

	return exception, nil
}

// deleteAccountException soft-deletes an account exception by its scoped UUID.
func (handler *AccountExceptionHandler) deleteAccountException(ctx context.Context, organizationID, ledgerID, accountID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_account_exception")
	defer span.End()

	if err := handler.Command.DeleteAccountException(ctx, organizationID, ledgerID, accountID, id); err != nil {
		handleSpanByErrorClass(span, "Failed to delete account exception on command", err)

		return err
	}

	return nil
}
