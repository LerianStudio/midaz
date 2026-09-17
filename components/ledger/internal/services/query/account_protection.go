// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// protectBalanceSeedAdmission coordinates a cache-miss seed with closing.
//
// A cache hit never reaches here: it acquires no ownership and reads no account
// row, which is what keeps the cost of the protection on the miss alone. On a miss
// the account is owned for the authoritative read, so a closing that starts
// meanwhile cannot slip between reading closed_at and using the seed, and a seed
// loaded for a closed account never becomes a live balance.
//
// The ownership ends with this load. Holding it through the engine's own admission
// belongs with the script-side validation and is not part of this seam yet.
func (uc *UseCase) protectBalanceSeedAdmission(ctx context.Context, span trace.Span, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	if len(balances) == 0 {
		return nil
	}

	if uc.AccountRepo == nil || uc.TransactionRedisRepo == nil {
		return nil
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	accountIDs, err := seedAccountIDs(balances)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Invalid account ID on balance", err)
		logger.Log(ctx, libLog.LevelError, "Invalid account ID on balance", libLog.Err(err))

		return err
	}

	guard := accountprotection.NewGuard(uc.AccountRepo, uc.TransactionRedisRepo)

	admission, err := guard.AcquireAdmission(ctx, organizationID, ledgerID, accountIDs)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to protect the balance seed admission", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to protect the balance seed admission", libLog.Err(err))

		return err
	}

	defer admission.Release(ctx)

	if err := guard.EnsureOpen(ctx, organizationID, ledgerID, accountIDs); err != nil {
		if _, closed := accountprotection.AsClosedAccountError(err); closed {
			err = pkg.ValidateBusinessError(constant.ErrAccountClosed, constant.EntityAccount)
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Refused to admit a balance of a closed account", err)
		logger.Log(ctx, libLog.LevelWarn, "Refused to admit a balance of a closed account", libLog.Err(err))

		return err
	}

	return nil
}

// seedAccountIDs collects the owning accounts of the loaded balances. An
// unparsable account identifier fails the read: a seed whose owner cannot be
// named cannot be checked against a closing either.
func seedAccountIDs(balances []*mmodel.Balance) ([]uuid.UUID, error) {
	accountIDs := make([]uuid.UUID, 0, len(balances))

	for _, balance := range balances {
		accountID, err := uuid.Parse(balance.AccountID)
		if err != nil {
			return nil, err
		}

		accountIDs = append(accountIDs, accountID)
	}

	return accountIDs, nil
}
