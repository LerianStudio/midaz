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

// accountProtectionGuard builds the guard over the repositories this use case
// already holds. A deployment without one of them carries no protection surface
// and the guard stays inert.
func (uc *UseCase) accountProtectionGuard() *accountprotection.Guard {
	if uc.AccountRepo == nil || uc.TransactionRedisRepo == nil {
		return nil
	}

	return accountprotection.NewGuard(uc.AccountRepo, uc.TransactionRedisRepo)
}

// protectBalanceSeedAdmission takes the administrative ownership of the accounts a
// cache-miss load is about to seed, and proves none of them is closed.
//
// A cache hit never reaches here: it acquires no ownership and reads no account
// row, which is what keeps the cost of the protection on the miss alone.
//
// The balances handed in only name the accounts. They are the resolution step —
// aliases become account identifiers only by reading them — and they are NOT the
// seeds: rows read before this ownership existed may already describe a closed
// account. The caller re-reads them under the returned ownership and uses those
// rows instead.
//
// The returned admission is nil when the deployment carries no protection surface
// or there is nothing to protect; Release handles that, and so does
// confirmSeedAdmissionCoverage.
//
// The ownership is held through the seed load and its rebuild. Where the caller
// installed an admission sink, it is handed over and stays alive through the
// accounting execution that admits the seed; otherwise the caller releases it when
// the load ends.
func (uc *UseCase) protectBalanceSeedAdmission(ctx context.Context, span trace.Span, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) (*accountprotection.Admission, error) {
	if len(balances) == 0 {
		return nil, nil
	}

	guard := uc.accountProtectionGuard()
	if guard == nil {
		return nil, nil
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	accountIDs, err := seedAccountIDs(balances)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Invalid account ID on balance", err)
		logger.Log(ctx, libLog.LevelError, "Invalid account ID on balance", libLog.Err(err))

		return nil, err
	}

	admission, err := guard.AcquireAdmission(ctx, organizationID, ledgerID, accountIDs)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to protect the balance seed admission", err)
		logger.Log(ctx, libLog.LevelWarn, "Failed to protect the balance seed admission", libLog.Err(err))

		return nil, err
	}

	if err := guard.EnsureOpen(ctx, organizationID, ledgerID, accountIDs); err != nil {
		if _, closed := accountprotection.AsClosedAccountError(err); closed {
			err = pkg.ValidateBusinessError(constant.ErrAccountClosed, constant.EntityAccount)
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Refused to admit a balance of a closed account", err)
		logger.Log(ctx, libLog.LevelWarn, "Refused to admit a balance of a closed account", libLog.Err(err))

		admission.Release(ctx)

		return nil, err
	}

	return admission, nil
}

// confirmSeedAdmissionCoverage refuses a seed whose owning account the admission
// does not cover. The re-read runs under the ownership taken over the accounts the
// first read named; an account that appears only in the second read was never
// checked against a closing, and serving its row would be exactly the unprotected
// admission the ownership exists to prevent.
func (uc *UseCase) confirmSeedAdmissionCoverage(ctx context.Context, span trace.Span, admission *accountprotection.Admission, balances []*mmodel.Balance) error {
	if admission == nil || len(balances) == 0 {
		return nil
	}

	logger := libObservability.NewLoggerFromContext(ctx)

	owned := make(map[uuid.UUID]struct{}, len(admission.Accounts()))
	for _, accountID := range admission.Accounts() {
		owned[accountID] = struct{}{}
	}

	accountIDs, err := seedAccountIDs(balances)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Invalid account ID on balance", err)
		logger.Log(ctx, libLog.LevelError, "Invalid account ID on balance", libLog.Err(err))

		return err
	}

	for _, accountID := range accountIDs {
		if _, covered := owned[accountID]; covered {
			continue
		}

		indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Refused a balance seed outside the protected account set", indeterminate)
		logger.Log(ctx, libLog.LevelWarn, "Refused a balance seed outside the protected account set")

		return indeterminate
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
