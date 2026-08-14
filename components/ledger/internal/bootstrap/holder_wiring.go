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
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/google/uuid"
)

// holderByIDReader is the narrow seam over the CRM holder service's
// GetHolderByID. It is satisfied by *crmservices.UseCase and lets Exists's
// not-found discrimination be tested without a Mongo-backed service.
type holderByIDReader interface {
	GetHolderByID(ctx context.Context, organizationID string, id uuid.UUID, includeDeleted bool) (*mmodel.Holder, error)
}

// holderReaderAdapter satisfies command.HolderReader over the CRM holder
// service, hiding the repository's misleadingly-named collection parameter and
// passing the organization ID through correctly. It lets the command package
// assert holder existence without importing the CRM package (dependency-inward).
type holderReaderAdapter struct {
	service holderByIDReader
}

// Exists reports whether a holder with id exists within the organization. A
// holder-not-found business error is mapped to (false, nil); every other error
// propagates so transient/infrastructure failures do not masquerade as absence.
func (a holderReaderAdapter) Exists(ctx context.Context, organizationID string, id uuid.UUID) (bool, error) {
	if _, err := a.service.GetHolderByID(ctx, organizationID, id, false); err != nil {
		var notFound pkg.EntityNotFoundError
		if errors.As(err, &notFound) && notFound.Code == constant.ErrHolderNotFound.Error() {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// holderAccountsReaderAdapter satisfies httpin.HolderAccountsReader over the
// ledger account query use case. Ownership is org-global (the holder collection
// is per-organization), so the account-list query is scoped by holder_id; the
// existing account-list read path is ledger-partitioned, so the caller supplies
// the ledger via the ledger_id query parameter.
type holderAccountsReaderAdapter struct {
	query *query.UseCase
}

// ListAccountsByHolder lists the accounts owned by a holder. The holder_id
// filter (set by the handler) performs the ownership scoping; ledger_id narrows
// the read to one ledger. The underlying account-list read is ledger-scoped, so
// ledger_id is required and its absence is reported as a missing-parameter error.
func (a holderAccountsReaderAdapter) ListAccountsByHolder(ctx context.Context, organizationID string, holderID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Account, error) {
	orgID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, constant.EntityOrganization)
	}

	if filter.LedgerID == nil || *filter.LedgerID == "" {
		return nil, pkg.ValidateBusinessError(constant.ErrMissingRequiredQueryParameter, constant.EntityAccount, "ledger_id")
	}

	ledgerID, err := uuid.Parse(*filter.LedgerID)
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, constant.EntityLedger)
	}

	return a.query.GetAllAccount(ctx, orgID, ledgerID, nil, nil, filter)
}
