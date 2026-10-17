// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libHTTP "github.com/LerianStudio/lib-commons/v7/commons/net/http"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// GetAllBalances lists a ledger's balances with their current state, cursor-paginated.
func (uc *UseCase) GetAllBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances")
	defer span.End()

	balances, cur, err := uc.BalanceRepo.ListAll(ctx, organizationID, ledgerID, filter.ToCursorPagination())
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to get balances on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(balances) == 0 {
		libOpentelemetry.HandleSpanEvent(span, "No balances found")

		return []*mmodel.Balance{}, libHTTP.CursorPagination{}, nil
	}

	if err := uc.applyBalanceState(ctx, span, organizationID, ledgerID, balances); err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	return balances, cur, nil
}

// GetAllBalancesByAlias lists an account's balances by alias with their current state.
func (uc *UseCase) GetAllBalancesByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) ([]*mmodel.Balance, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances_by_alias")
	defer span.End()

	balances, err := uc.BalanceRepo.ListByAliases(ctx, organizationID, ledgerID, []string{alias})
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to list balances by alias on balance database", err)

		return nil, err
	}

	if len(balances) == 0 {
		libOpentelemetry.HandleSpanEvent(span, "No balances found for alias")

		return nil, nil
	}

	if err := uc.applyBalanceState(ctx, span, organizationID, ledgerID, balances); err != nil {
		return nil, err
	}

	return balances, nil
}
