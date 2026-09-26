// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// accountProtectionGuard builds the seed admission guard over the repositories
// this use case already holds. A deployment without one of them carries no
// protection surface and the guard stays inert.
func (uc *UseCase) accountProtectionGuard() *accountprotection.Guard {
	if uc.AccountRepo == nil || uc.TransactionRedisRepo == nil {
		return nil
	}

	return accountprotection.NewSeedAdmissionGuard(uc.AccountRepo, seedAdmissionStore{uc.TransactionRedisRepo})
}

// seedAdmissionStore presents the transaction cache as the guard's seed admission
// surface, translating the cache's holder classification into the guard's
// contract.
type seedAdmissionStore struct {
	redis.RedisRepository
}

// AdmitAccountSeed reports a refusal only for the one holder that can refuse a
// seed admission, an exclusive owner. Any other classification on a refusal is a
// state the guard cannot interpret, so it becomes an unreadable protection
// instead of a business answer.
func (s seedAdmissionStore) AdmitAccountSeed(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	admitted, holder, err := s.AcquireAccountSeedAdmission(ctx, organizationID, ledgerID, accountID, token)
	if err != nil || admitted {
		return admitted, err
	}

	if holder != redis.AccountAdminHolderExclusive {
		return false, fmt.Errorf("%w: seed admission refused by holder %q", redis.ErrAccountProtectionMarkerUnreadable, holder)
	}

	return false, nil
}

// ReleaseAccountSeed removes only this admission's member.
func (s seedAdmissionStore) ReleaseAccountSeed(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID, token string) (bool, error) {
	return s.ReleaseAccountSeedAdmission(ctx, organizationID, ledgerID, accountID, token)
}

// protectBalanceSeedAdmission takes a shared seed admission over the accounts a
// cache-miss load is about to seed, and proves none of them is closed.
//
// The admission is shared so concurrent loads of the same account never refuse
// each other; it still excludes a closing, a balance creation and a balance
// deletion, in both directions.
//
// A cache hit never reaches here: it takes no admission and reads no account row,
// which is what keeps the cost of the protection on the miss alone.
//
// The balances handed in only name the accounts. They are the resolution step —
// aliases become account identifiers only by reading them — and they are NOT the
// seeds: rows read before this admission existed may already describe a closed
// account. The caller re-reads them under the returned admission and uses those
// rows instead.
//
// The returned admission is nil when the deployment carries no protection surface
// or there is nothing to protect; Release handles that, and so does
// readSeedsUnderAdmission.
//
// The admission is held through the seed load and its rebuild. Where the caller
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

	admission, err := guard.AcquireSeedAdmission(ctx, organizationID, ledgerID, accountIDs)
	if err != nil {
		recordSeedAdmissionRefusal(ctx, span, logger, "Failed to protect the balance seed admission", err)

		return nil, err
	}

	if err := guard.EnsureOpen(ctx, organizationID, ledgerID, accountIDs); err != nil {
		if _, closed := accountprotection.AsClosedAccountError(err); closed {
			err = pkg.ValidateBusinessError(constant.ErrAccountClosed, constant.EntityAccount)
		}

		recordSeedAdmissionRefusal(ctx, span, logger, "Refused to admit a balance of a closed account", err)

		admission.Release(ctx)

		return nil, err
	}

	return admission, nil
}

// maxSeedAdmissionExtensions bounds how many times one load extends its seed
// admission to accounts that appeared between two of its reads.
const maxSeedAdmissionExtensions = 2

// readSeedsUnderAdmission reads the seeds again under the admission and returns
// only rows whose owning accounts the load's admissions all cover.
//
// A row can appear between the resolution read and this one — an overdraft
// companion a concurrent settings update just created, for instance — and its
// account was never checked against a closing. Serving it would be exactly the
// unprotected seed the admission exists to prevent, so the admission is extended
// to that account, which proves it open, and the rows are read once more: rows
// read before an admission covered their account are never served. The extension
// is bounded; a load whose every read keeps naming a new account refuses as
// indeterminate instead of chasing the drift.
//
// hold receives every extension so the caller can hand it to the execution or
// release it when the load ends, exactly like the first admission.
func (uc *UseCase) readSeedsUnderAdmission(
	ctx context.Context,
	span trace.Span,
	organizationID, ledgerID uuid.UUID,
	aliases []string,
	admission *accountprotection.Admission,
	hold func(*accountprotection.Admission),
) ([]*mmodel.Balance, error) {
	logger := libObservability.NewLoggerFromContext(ctx)

	covered := make(map[uuid.UUID]struct{})
	for _, accountID := range admission.Accounts() {
		covered[accountID] = struct{}{}
	}

	for extensions := 0; ; extensions++ {
		balances, err := uc.BalanceRepo.ListByAliasesWithKeys(ctx, organizationID, ledgerID, aliases)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to get balances from database", err)
			logger.Log(ctx, libLog.LevelError, "Failed to get balances from database", libLog.Err(err))

			return nil, err
		}

		if uc.accountProtectionGuard() == nil {
			return balances, nil
		}

		uncovered, err := uncoveredSeeds(balances, covered)
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Invalid account ID on balance", err)
			logger.Log(ctx, libLog.LevelError, "Invalid account ID on balance", libLog.Err(err))

			return nil, err
		}

		if len(uncovered) == 0 {
			return balances, nil
		}

		if extensions == maxSeedAdmissionExtensions {
			indeterminate := pkg.ValidateBusinessError(constant.ErrAccountClosingProtectionIndeterminate, constant.EntityAccount)

			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Refused a balance seed outside the protected account set", indeterminate)
			logger.Log(ctx, libLog.LevelWarn, "Refused a balance seed outside the protected account set")

			return nil, indeterminate
		}

		extension, err := uc.protectBalanceSeedAdmission(ctx, span, organizationID, ledgerID, uncovered)
		if err != nil {
			return nil, err
		}

		hold(extension)

		for _, accountID := range extension.Accounts() {
			covered[accountID] = struct{}{}
		}
	}
}

// uncoveredSeeds returns the rows whose owning account is not covered yet. An
// unparsable account identifier fails the read: a seed whose owner cannot be
// named cannot be checked against a closing either.
func uncoveredSeeds(balances []*mmodel.Balance, covered map[uuid.UUID]struct{}) ([]*mmodel.Balance, error) {
	var uncovered []*mmodel.Balance

	for _, balance := range balances {
		accountID, err := uuid.Parse(balance.AccountID)
		if err != nil {
			return nil, err
		}

		if _, ok := covered[accountID]; !ok {
			uncovered = append(uncovered, balance)
		}
	}

	return uncovered, nil
}

// recordSeedAdmissionRefusal records a refused seed admission with the class of
// its cause: another operation holding the account, or a closed account, is the
// protection answering as designed; a protection state that could not be read is a
// dependency failing and must reach this span as such.
func recordSeedAdmissionRefusal(ctx context.Context, span trace.Span, logger libLog.Logger, message string, err error) {
	if pkg.IsBusinessError(err) {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, message, err)
		logger.Log(ctx, libLog.LevelWarn, message, libLog.Err(err))

		return
	}

	libOpentelemetry.HandleSpanError(span, message, err)
	logger.Log(ctx, libLog.LevelError, message, libLog.Err(err))
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
