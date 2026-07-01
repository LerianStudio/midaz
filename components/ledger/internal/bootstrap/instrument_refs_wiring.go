// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/google/uuid"
)

// ledgerAccountReaderAdapter satisfies crmservices.LedgerAccountReader over the
// ledger query use case, letting the CRM instrument-create path verify ledger
// and account references in-process without importing the query package
// (dependency-inward). The underlying queries return 404-typed not-found
// sentinels; this adapter discriminates those into a sentinel-free boolean so
// the CRM use case maps absence to its own 422 referential errors.
type ledgerAccountReaderAdapter struct {
	query *query.UseCase
}

// LedgerExists reports whether a ledger exists within the organization. A
// ledger-not-found business error is mapped to (false, nil); every other error
// propagates so transient/infrastructure failures do not masquerade as absence.
func (a ledgerAccountReaderAdapter) LedgerExists(ctx context.Context, organizationID, ledgerID uuid.UUID) (bool, error) {
	if _, err := a.query.GetLedgerByID(ctx, organizationID, ledgerID); err != nil {
		var notFound pkg.EntityNotFoundError
		if errors.As(err, &notFound) && notFound.Code == constant.ErrLedgerIDNotFound.Error() {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// AccountExists reports whether an account exists within the ledger. The
// portfolio filter is nil because instrument references address an account
// directly. An account-not-found business error is mapped to (false, nil);
// every other error propagates.
func (a ledgerAccountReaderAdapter) AccountExists(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (bool, error) {
	if _, err := a.query.GetAccountByID(ctx, organizationID, ledgerID, nil, accountID); err != nil {
		var notFound pkg.EntityNotFoundError
		if errors.As(err, &notFound) && notFound.Code == constant.ErrAccountIDNotFound.Error() {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// CountAccountsByHolder reports how many active accounts the holder owns within
// the organization, across all ledgers. It backs the CRM holder-delete
// ownership guard; errors propagate unchanged.
func (a ledgerAccountReaderAdapter) CountAccountsByHolder(ctx context.Context, organizationID, holderID uuid.UUID) (int64, error) {
	return a.query.CountAccountsByHolderID(ctx, organizationID, holderID)
}
