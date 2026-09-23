// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	tmmongo "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/mongo"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
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

	// crmTenantDB resolves the tenant CRM database in multi-tenant mode. It is nil
	// in single-tenant mode, where the CRM repos use their static connection.
	crmTenantDB command.TenantMongoResolver
}

// newHolderReaderAdapter builds the holder reader over the CRM holder service.
// crmManager is the CRM Mongo manager, nil in single-tenant mode; it is stored only
// when non-nil, because a nil *tmmongo.Manager inside the interface would be a
// non-nil resolver.
func newHolderReaderAdapter(service holderByIDReader, crmManager *tmmongo.Manager) holderReaderAdapter {
	adapter := holderReaderAdapter{service: service}
	if crmManager != nil {
		adapter.crmTenantDB = crmManager
	}

	return adapter
}

// Exists reports whether a holder with id exists within the organization. A
// holder-not-found business error is mapped to (false, nil); every other error
// propagates so transient/infrastructure failures do not masquerade as absence.
func (a holderReaderAdapter) Exists(ctx context.Context, organizationID string, id uuid.UUID) (bool, error) {
	ctx, err := a.crmTenantContext(ctx)
	if err != nil {
		return false, err
	}

	if _, err := a.service.GetHolderByID(ctx, organizationID, id, false); err != nil {
		var notFound pkg.EntityNotFoundError
		if errors.As(err, &notFound) && notFound.Code == constant.ErrHolderNotFound.Error() {
			return false, nil
		}

		return false, err
	}

	return true, nil
}

// crmTenantContext returns a context carrying the tenant CRM database on the
// generic Mongo key the CRM holder repo reads. The account-create route that runs
// this check binds only the module-keyed ledger stores. The derived context is
// scoped to the holder read and never returned to the caller. In single-tenant mode
// there is no resolver and ctx is returned unchanged.
func (a holderReaderAdapter) crmTenantContext(ctx context.Context) (context.Context, error) {
	if a.crmTenantDB == nil {
		return ctx, nil
	}

	return command.ResolveTenantMongoContext(ctx, a.crmTenantDB, "holder seam")
}

// holderAccountsReaderAdapter satisfies httpin.HolderAccountsReader over the
// ledger account query use case. Ownership is org-global — the holder collection
// is per-organization — so the listing spans every ledger of the organization and
// ledger_id is an optional narrowing filter rather than a scoping key.
type holderAccountsReaderAdapter struct {
	query *query.UseCase
}

// ListAccountsByHolder lists the accounts a holder owns across the organization.
// A ledger_id query parameter narrows the result to one ledger; a malformed one
// is a query-parameter validation error.
func (a holderAccountsReaderAdapter) ListAccountsByHolder(ctx context.Context, organizationID string, holderID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Account, error) {
	orgID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, constant.EntityOrganization)
	}

	var ledgerID *uuid.UUID

	if !libCommons.IsNilOrEmpty(filter.LedgerID) {
		parsed, err := uuid.Parse(*filter.LedgerID)
		if err != nil {
			return nil, pkg.ValidateBusinessError(constant.ErrInvalidQueryParameter, constant.EntityAccount, "ledger_id")
		}

		ledgerID = &parsed
	}

	// HolderOnV2: the holder-accounts route is served on /v2 only, so the
	// accounts it answers with carry the holder keys.
	return a.query.GetAllAccountsByHolder(ctx, orgID, holderID, ledgerID, filter, mmodel.HolderOnV2)
}
