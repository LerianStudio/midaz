// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	cn "github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

type BalanceHandler struct {
	Command *command.UseCase
	Query   *query.UseCase
}

// --- Transport-agnostic cores -------------------------------------------------
//
// Each core below owns the span, imperative query/date validation and the service
// call. They take primitive args — parsed UUIDs, raw path strings, the query map —
// so nothing transport-shaped reaches them; the handlers
// in balance_handler.go pull those out of the request envelope. Every canonical
// Midaz error a core returns is rendered by its caller via http.HumaProblem, which
// fixes the code + HTTP status. The three write cores (update / create-additional /
// delete) are MONEY-adjacent: they own no money logic of their own, the command use
// cases they call do.

// getAllBalances binds the query imperatively via http.ValidateParameters then
// returns the cursor-paginated envelope.
func (handler *BalanceHandler) getAllBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_balances")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	headerParams.Metadata = &bson.M{}

	balances, cur, err := handler.Query.GetAllBalances(ctx, organizationID, ledgerID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Balances", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(balances)
	pagination.SetCursor(cur.Next, cur.Prev)

	return pagination, nil
}

// getAllBalancesByAccountID mirrors getAllBalances scoped to a single account.
func (handler *BalanceHandler) getAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, queries map[string]string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_all_balances_by_account_id")
	defer span.End()

	headerParams, err := http.ValidateParameters(queries)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate query parameters", err)

		return http.Pagination{}, err
	}

	recordSafeQueryAttributes(span, headerParams)

	pagination := http.Pagination{
		Limit:     headerParams.Limit,
		SortOrder: headerParams.SortOrder,
		StartDate: headerParams.StartDate,
		EndDate:   headerParams.EndDate,
	}

	headerParams.Metadata = &bson.M{}

	balances, cur, err := handler.Query.GetAllBalancesByAccountID(ctx, organizationID, ledgerID, accountID, *headerParams)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve all Balances by account id", err)

		return http.Pagination{}, err
	}

	pagination.SetItems(balances)
	pagination.SetCursor(cur.Next, cur.Prev)

	return pagination, nil
}

// getBalanceByID retrieves a single balance.
func (handler *BalanceHandler) getBalanceByID(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (*mmodel.Balance, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balance_by_id")
	defer span.End()

	op, err := handler.Query.GetBalanceByID(ctx, organizationID, ledgerID, balanceID)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve balance by id", err)

		return nil, err
	}

	return op, nil
}

// deleteBalance removes a balance (MONEY-adjacent; command core untouched).
func (handler *BalanceHandler) deleteBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.delete_balance_by_id")
	defer span.End()

	if err := handler.Command.DeleteBalance(ctx, organizationID, ledgerID, balanceID); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to delete balance by id", err)

		return err
	}

	return nil
}

// updateBalance owns the span + service call for an already-decoded
// payload (MONEY-adjacent; command core untouched). Body decode+validation happens
// BEFORE this core (Fiber WithBody decorator or Huma http.DecodeAndValidate).
func (handler *BalanceHandler) updateBalance(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, payload *mmodel.UpdateBalance) (*mmodel.Balance, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.update_balance")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	balance, err := handler.Command.Update(ctx, organizationID, ledgerID, balanceID, *payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to update Balance on command", err)

		return nil, err
	}

	return balance, nil
}

// createAdditionalBalance owns the span + service call for an
// already-decoded payload (MONEY-adjacent; command core untouched).
func (handler *BalanceHandler) createAdditionalBalance(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, payload *mmodel.CreateAdditionalBalance) (*mmodel.Balance, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.create_additional_balance")
	defer span.End()

	recordSafePayloadAttributes(span, payload)

	balance, err := handler.Command.CreateAdditionalBalance(ctx, organizationID, ledgerID, accountID, payload)
	if err != nil {
		handleSpanByErrorClass(span, "Failed to create additional balance on command", err)

		return nil, err
	}

	return balance, nil
}

// getBalancesByAlias resolves balances by a raw alias path string (no UUID parse).
func (handler *BalanceHandler) getBalancesByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balances_by_alias")
	defer span.End()

	balances, err := handler.Query.GetAllBalancesByAlias(ctx, organizationID, ledgerID, alias)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve balances by alias", err)

		return http.Pagination{}, err
	}

	if len(balances) == 0 {
		balances = []*mmodel.Balance{}
	}

	return http.Pagination{Limit: 10, Items: balances}, nil
}

// getBalancesExternalByCode resolves external balances by a raw code path string,
// derived into the external-account alias (no UUID parse on code).
func (handler *BalanceHandler) getBalancesExternalByCode(ctx context.Context, organizationID, ledgerID uuid.UUID, code string) (http.Pagination, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balances_external_by_code")
	defer span.End()

	alias := cn.DefaultExternalAccountAliasPrefix + code

	balances, err := handler.Query.GetAllBalancesByAlias(ctx, organizationID, ledgerID, alias)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to retrieve balances by code", err)

		return http.Pagination{}, err
	}

	if len(balances) == 0 {
		balances = []*mmodel.Balance{}
	}

	return http.Pagination{Limit: 10, Items: balances}, nil
}

// getBalanceAtTimestamp validates the date query imperatively then returns the
// historical balance. The date param carries no native validation (see the Huma
// handler); this core is its sole validator.
func (handler *BalanceHandler) getBalanceAtTimestamp(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID, dateStr string) (*mmodel.BalanceHistory, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_balance_at_timestamp")
	defer span.End()

	date, err := parseBalanceHistoryDate(ctx, span, logger, dateStr)
	if err != nil {
		return nil, err
	}

	balance, err := handler.Query.GetBalanceAtTimestamp(ctx, organizationID, ledgerID, balanceID, date)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve balance at date", err)

		return nil, err
	}

	return balance.ToHistoryResponse(), nil
}

// getAccountBalancesAtTimestamp validates the date query imperatively then returns
// all historical balances for an account (see getBalanceAtTimestamp for the date
// validation split).
func (handler *BalanceHandler) getAccountBalancesAtTimestamp(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, dateStr string) ([]*mmodel.BalanceHistory, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "handler.get_account_balances_at_timestamp")
	defer span.End()

	date, err := parseBalanceHistoryDate(ctx, span, logger, dateStr)
	if err != nil {
		return nil, err
	}

	balances, err := handler.Query.GetAccountBalancesAtTimestamp(ctx, organizationID, ledgerID, accountID, date)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve account balances at date", err)

		return nil, err
	}

	if len(balances) == 0 {
		err := pkg.ValidateBusinessError(cn.ErrNoBalanceDataAtTimestamp, date.Format("2006-01-02 15:04:05"))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No balance data available for the specified timestamp", err)

		return nil, err
	}

	historyBalances := make([]*mmodel.BalanceHistory, len(balances))
	for i := range balances {
		historyBalances[i] = balances[i].ToHistoryResponse()
	}

	return historyBalances, nil
}

// parseBalanceHistoryDate is the shared imperative validator for the `date` query
// param the two history cores use: present, parseable, and carrying a time
// component (yyyy-mm-dd hh:mm:ss). Every rejection is a canonical 400.
func parseBalanceHistoryDate(ctx context.Context, span trace.Span, logger libLog.Logger, dateStr string) (time.Time, error) {
	if dateStr == "" {
		err := pkg.ValidateBusinessError(cn.ErrMissingRequiredQueryParameter, "Balance", "date")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Missing date parameter", err)
		logger.Log(ctx, libLog.LevelWarn, "Missing date query parameter")

		return time.Time{}, err
	}

	date, hasTime, err := libCommons.ParseDateTime(dateStr, false)
	if err != nil {
		validationErr := pkg.ValidateBusinessError(cn.ErrInvalidDatetimeFormat, "Balance", "date", "yyyy-mm-dd hh:mm:ss")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Invalid date format", validationErr)

		return time.Time{}, validationErr
	}

	if !hasTime {
		validationErr := pkg.ValidateBusinessError(cn.ErrInvalidDatetimeFormat, "Balance", "date", "yyyy-mm-dd hh:mm:ss")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Time component is required", validationErr)

		return time.Time{}, validationErr
	}

	return date, nil
}
