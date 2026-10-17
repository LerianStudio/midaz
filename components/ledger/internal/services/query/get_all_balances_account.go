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

// GetAllBalancesByAccountID methods responsible to get all balances by account id from a database.
// This method is used to get all balances by account id from a database and return them in a cursor pagination format.
// It also validates if the balance is currently in the redis cache and if so, it uses the cached values instead of the database values.
func (uc *UseCase) GetAllBalancesByAccountID(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_all_balances_by_account_id")
	defer span.End()

	balance, cur, err := uc.BalanceRepo.ListAllByAccountID(ctx, organizationID, ledgerID, accountID, filter.ToCursorPagination())
	if err != nil {
		recordQueryError(ctx, span, logger, "Failed to get balances on repo", err)

		return nil, libHTTP.CursorPagination{}, err
	}

	if len(balance) == 0 {
		libOpentelemetry.HandleSpanEvent(span, "No balances found")

		return balance, cur, nil
	}

	if err := uc.applyBalanceState(ctx, span, organizationID, ledgerID, balance); err != nil {
		return nil, libHTTP.CursorPagination{}, err
	}

	return balance, cur, nil
}
