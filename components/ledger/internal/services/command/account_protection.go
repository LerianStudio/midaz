// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/accountprotection"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
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

// acquireAccountAdmission takes the administrative ownership of the accounts an
// operation is about to touch. The caller MUST release it once the operation's
// result is known, and MUST mark it indeterminate instead when it is not.
func (uc *UseCase) acquireAccountAdmission(ctx context.Context, organizationID, ledgerID uuid.UUID, accountIDs ...uuid.UUID) (*accountprotection.Admission, error) {
	return uc.accountProtectionGuard().AcquireAdmission(ctx, organizationID, ledgerID, accountIDs)
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
