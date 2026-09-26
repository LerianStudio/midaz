// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
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

// acquireAccountOwnership takes the exclusive administrative ownership of the
// accounts an operation is about to touch. The caller MUST release it once the
// operation's result is known, and MUST mark it indeterminate instead when it is
// not.
//
// Exclusive, because closing, balance creation and balance deletion each change
// or validate the balance list the others and every cache-miss load rely on. A
// refusal answers 0522 while a closing holds the account and 0526 for any other
// holder, live seed admissions included.
func (uc *UseCase) acquireAccountOwnership(ctx context.Context, organizationID, ledgerID uuid.UUID, accountIDs ...uuid.UUID) (*accountprotection.Admission, error) {
	return uc.accountProtectionGuard().AcquireExclusive(ctx, organizationID, ledgerID, accountIDs)
}

// ensureAccountsNotClosed refuses the operation when any of the accounts carries a
// closing instant, translating the guard's report into the business error of this
// surface. closedErr is the sentinel the caller's contract uses for a closed
// account; every other refusal already carries its own code.
func (uc *UseCase) ensureAccountsNotClosed(ctx context.Context, organizationID, ledgerID uuid.UUID, closedErr error, accountIDs ...uuid.UUID) error {
	err := uc.accountProtectionGuard().EnsureOpen(ctx, organizationID, ledgerID, accountIDs)
	if err == nil {
		return nil
	}

	if _, closed := accountprotection.AsClosedAccountError(err); closed {
		return pkg.ValidateBusinessError(closedErr, constant.EntityAccount)
	}

	return err
}

// resolveAccountAdmission ends the ownership according to what is known about the
// write the operation issued.
//
// writeIssued tells the two situations apart: a failure raised before any write
// left nothing behind, so the ownership goes back immediately. Once a write is on
// the wire, only a server answer proves it did not land — PostgreSQL rejecting the
// statement with a SQLSTATE. A cancelled context, an expired deadline or a lost
// connection proves nothing: the commit may have happened with the answer lost on
// the way back, and releasing then would let a closing validate a balance list
// that is still changing. Such an outcome keeps the ownership for reconciliation.
func resolveAccountAdmission(ctx context.Context, admission *accountprotection.Admission, writeIssued bool, err error) {
	if err != nil && writeIssued && !sqlWriteOutcomeIsKnown(err) {
		admission.MarkIndeterminate()
	}

	admission.Release(ctx)
}

// resolveEngineAdmissions ends the ownerships an execution was held for according
// to what its answer proves.
//
// A success, a request never submitted and a refusal the engine decided before its
// commit phase all prove the accounting state did not change, so the ownership goes
// back. Anything else — a timeout, a lost connection, a malformed response, a
// failure after writes may have started — leaves an execution whose outcome is
// unknown: the ownership stays for reconciliation, because releasing it would let a
// closing validate a balance list a live execution can still move.
func resolveEngineAdmissions(admissions *accountprotection.Sink, request accounting.Execution, outcome EngineExecutionOutcome, err error) {
	if err == nil || !outcome.Executed || confirmedPrecommitEngineFailure(request, err) {
		return
	}

	admissions.MarkIndeterminate()
}

// ensureBalanceAccountsAvailable refuses a compatibility movement whose balances
// belong to an account the cache marks as closing or closed.
//
// It is the availability check of the paths that do not reach the engine, where
// the same controls are read inside the atomic execution. Markers only: no
// ownership is taken and no account row is read, so a warm load keeps costing no
// query. The cache-miss admission of those same balances is protected by the load
// itself.
func (uc *UseCase) ensureBalanceAccountsAvailable(ctx context.Context, organizationID, ledgerID uuid.UUID, balances []*mmodel.Balance) error {
	if len(balances) == 0 {
		return nil
	}

	accountIDs := make([]uuid.UUID, 0, len(balances))

	for _, balance := range balances {
		accountID, err := uuid.Parse(balance.AccountID)
		if err != nil {
			return err
		}

		accountIDs = append(accountIDs, accountID)
	}

	err := uc.accountProtectionGuard().EnsureAvailable(ctx, organizationID, ledgerID, accountIDs)
	if err == nil {
		return nil
	}

	if _, closed := accountprotection.AsClosedAccountError(err); closed {
		return pkg.ValidateBusinessError(constant.ErrAccountClosed, constant.EntityAccount)
	}

	return err
}

// sqlWriteOutcomeIsKnown reports whether err proves the write did not land: a
// SQLSTATE the server answered with, or a refusal this service decided on its own
// — a uniqueness conflict is reported as the second after being recognized as the
// first, and both mean nothing was persisted.
func sqlWriteOutcomeIsKnown(err error) bool {
	if err == nil {
		return true
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr != nil {
		return true
	}

	return pkg.IsBusinessError(err)
}
