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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

type AccountTypeHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createAccountType/updateAccountType/... methods below own the span, the
// service call and the success log. They take primitive args — parsed UUIDs, the
// already-decoded payload, the query map — so nothing transport-shaped reaches them;
// the handlers in accounttype_handler.go pull those out of the request envelope.
// Every canonical Midaz error a core returns is rendered by its caller via
// http.HumaProblem, which fixes the code + HTTP status.

// createAccountType owns the span + service call + success log for an already-decoded
// payload. Body decode+validation happens BEFORE this core, in the handler, via
// http.DecodeAndValidate(RawBody).
func (handler *AccountTypeHandler) createAccountType(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAccountTypeInput) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account_type")
	defer span.End()

	recordSafePayloadAttributes(span, payload)
	logSafePayload(ctx, logger, "Request to create an account type", payload)

	accountType, err := handler.Command.CreateAccountType(ctx, organizationID, ledgerID, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create account type", err)

		return nil, err
	}

	return accountType, nil
}

// getAccountTypeByID retrieves a single account type.
func (handler *AccountTypeHandler) getAccountTypeByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_type_by_id")
	defer span.End()

	accountType, err := handler.Query.GetAccountTypeByID(ctx, organizationID, ledgerID, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account Type on query", err)

		return nil, err
	}

	return accountType, nil
}

// updateAccountType owns the span + service call + success log for an already-decoded
// payload (see createAccountType for where the decode happens).
func (handler *AccountTypeHandler) updateAccountType(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdateAccountTypeInput) (*mmodel.AccountType, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_account_type")
	defer span.End()

	recordSafePayloadAttributes(span, payload)
	logSafePayload(ctx, logger, "Request to update account type", payload)

	accountType, err := handler.Command.UpdateAccountType(ctx, organizationID, ledgerID, id, payload)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update account type", err)

		return nil, err
	}

	return accountType, nil
}

// deleteAccountType removes an account type.
func (handler *AccountTypeHandler) deleteAccountType(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_account_type_by_id")
	defer span.End()

	if err := handler.Command.DeleteAccountTypeByID(ctx, organizationID, ledgerID, id); err != nil {
		handleSpanByErrorClass(span, "Failed to delete Account Type on command", err)

		return err
	}

	return nil
}

// getAllAccountTypes binds the query map imperatively via http.ValidateParameters so a
// bad query yields the canonical 400, then returns the assembled cursor-paginated
// envelope.
func (handler *AccountTypeHandler) getAllAccountTypes(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_account_types")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		Cursor:    headerParams.Cursor,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if headerParams.Metadata != nil {
		accountTypes, cur, err := handler.Query.GetAllMetadataAccountType(ctx, organizationID, ledgerID, *headerParams)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve all Account Types on query", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(accountTypes)
		pagination.SetCursor(cur.Next, cur.Prev)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	accountTypes, cur, err := handler.Query.GetAllAccountType(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account Types on query", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(accountTypes)
	pagination.SetCursor(cur.Next, cur.Prev)

	return pagination, nil
}
