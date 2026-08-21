// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// AccountHandler struct contains an account use case for managing account related operations.
type AccountHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// The createAccount/updateAccount/... cores below own the span, the imperative
// query binding, the service call(s) and the success log/metric. They take
// primitive args — parsed UUIDs, the already-decoded payload, the query map — so
// nothing transport-shaped reaches them; the handlers in account_handler.go pull
// those out of the request envelope. Every canonical Midaz error a core returns is
// rendered by its caller via http.HumaProblem, which fixes the code + HTTP status.

// createAccount owns the span + service call + success log + created metric for an
// already-decoded payload. Body decode+validation happens BEFORE this core, in the
// handler, via http.DecodeAndValidate(RawBody). The RecordAccountCreated metric
// lives here, alongside the service call it describes.
func (handler *AccountHandler) createAccount(ctx context.Context, organizationID, ledgerID uuid.UUID, payload *mmodel.CreateAccountInput, token string) (*mmodel.Account, error) {
	logger, tracer, _, metricFactory := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_account")
	defer span.End()

	logSafePayload(ctx, logger, "Request to create an account", payload)
	recordSafePayloadAttributes(span, payload)

	account, err := handler.Command.CreateAccount(ctx, organizationID, ledgerID, payload, token)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create Account on command", err)

		return nil, err
	}

	if err := metricFactory.RecordAccountCreated(
		ctx,
		attribute.String("organization_id", organizationID.String()),
		attribute.String("ledger_id", ledgerID.String()),
	); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to record account created metric", err)
	}

	return account, nil
}

// getAllAccounts binds the query map imperatively via http.ValidateParameters,
// validates the account-specific status enum, resolves the optional
// portfolio_id/segment_id UUID filters, then branches on metadata. A bad query or
// status yields the canonical 400.
func (handler *AccountHandler) getAllAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_accounts")
	defer span.End()

	var (
		portfolioID *uuid.UUID
		segmentID   *uuid.UUID
	)

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	if headerParams.Status != nil && !isValidStatus(*headerParams.Status, accountAllowedStatuses) {
		err := pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityAccount, "status")

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters: invalid account status", err)

		logger.Log(ctx, libLog.LevelWarn, "Failed to validate account status query parameter", libLog.String("status", *headerParams.Status), libLog.Err(err))

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		Page:      headerParams.Page,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	if !libCommons.IsNilOrEmpty(&headerParams.PortfolioID) {
		parsedID := uuid.MustParse(headerParams.PortfolioID)
		portfolioID = &parsedID
	}

	if !libCommons.IsNilOrEmpty(&headerParams.SegmentID) {
		parsedID := uuid.MustParse(headerParams.SegmentID)
		segmentID = &parsedID
	}

	if headerParams.Metadata != nil {
		accounts, err := handler.Query.GetAllMetadataAccounts(ctx, organizationID, ledgerID, portfolioID, segmentID, *headerParams)
		if err != nil {
			handleSpanByErrorClass(span, "Failed to retrieve all Accounts on query", err)

			return http.Pagination{}, err
		}

		pagination.SetItems(accounts)

		return pagination, nil
	}

	headerParams.Metadata = &bson.M{}

	accounts, err := handler.Query.GetAllAccount(ctx, organizationID, ledgerID, portfolioID, segmentID, *headerParams)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve all Accounts on query", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(accounts)

	return pagination, nil
}

// getAccountByID retrieves a single account by its UUID.
func (handler *AccountHandler) getAccountByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_by_id")
	defer span.End()

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account on query", err)

		return nil, err
	}

	return account, nil
}

// getAccountByAlias retrieves a single account by its alias. The external-by-code
// path resolves the alias (DefaultExternalAccountAliasPrefix + code) BEFORE this
// core, so both the alias and external-code ops share one implementation. The span
// name carries the caller so the two callers stay distinguishable in traces.
func (handler *AccountHandler) getAccountByAlias(ctx context.Context, spanName string, organizationID, ledgerID uuid.UUID, alias string) (*mmodel.Account, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, spanName)
	defer span.End()

	account, err := handler.Query.GetAccountByAlias(ctx, organizationID, ledgerID, nil, alias)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account on query", err)

		return nil, err
	}

	return account, nil
}

// updateAccount owns the span + update-then-get flow for an already-decoded
// payload. It updates, then re-reads so the caller receives the freshly persisted
// account.
func (handler *AccountHandler) updateAccount(ctx context.Context, organizationID, ledgerID, id uuid.UUID, payload *mmodel.UpdateAccountInput) (*mmodel.Account, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_account")
	defer span.End()

	logSafePayload(ctx, logger, "Request to update account", payload)
	recordSafePayloadAttributes(span, payload)

	if _, err := handler.Command.UpdateAccount(ctx, organizationID, ledgerID, nil, id, payload); err != nil {
		handleSpanByErrorClass(span, "Failed to update Account on command", err)

		return nil, err
	}

	account, err := handler.Query.GetAccountByID(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to retrieve Account on query", err)

		return nil, err
	}

	return account, nil
}

// deleteAccount removes an account by its UUID.
func (handler *AccountHandler) deleteAccount(ctx context.Context, organizationID, ledgerID, id uuid.UUID, token string) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_account_by_id")
	defer span.End()

	if err := handler.Command.DeleteAccountByID(ctx, organizationID, ledgerID, nil, id, token); err != nil {
		handleSpanByErrorClass(span, "Failed to remove Account on command", err)

		return err
	}

	return nil
}

// countAccounts returns the total account count for the ledger.
func (handler *AccountHandler) countAccounts(ctx context.Context, organizationID, ledgerID uuid.UUID) (int64, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.count_accounts")
	defer span.End()

	count, err := handler.Query.CountAccounts(ctx, organizationID, ledgerID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to count accounts", err)

		return 0, err
	}

	return count, nil
}
