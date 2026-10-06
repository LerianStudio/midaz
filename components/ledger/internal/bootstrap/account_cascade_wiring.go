// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"

	tmmongo "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/mongo"
	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

// instrumentCascader is the narrow seam over the CRM service's
// DeleteInstrumentsByAccount. It is satisfied by *crmservices.UseCase and lets
// the tenant resolution be tested without a Mongo-backed service.
type instrumentCascader interface {
	DeleteInstrumentsByAccount(ctx context.Context, organizationID string, ledgerID, accountID uuid.UUID) (int, error)
}

// feeAliasDetacher is the narrow seam over the fee service's DetachAccountAlias.
// It is satisfied by *feesservices.UseCase.
type feeAliasDetacher interface {
	DetachAccountAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (model.FeeAliasDetachResult, error)
}

var (
	_ command.InstrumentCascader = instrumentCascadeAdapter{}
	_ command.FeeAliasDetacher   = feeAliasDetachAdapter{}
)

// instrumentCascadeAdapter satisfies command.InstrumentCascader over the CRM
// service, so the account delete can soft-delete the instruments linked to the
// account without the command package importing CRM.
type instrumentCascadeAdapter struct {
	service instrumentCascader

	// crmTenantDB resolves the tenant CRM database in multi-tenant mode. It is nil
	// in single-tenant mode, where the CRM repos use their static connection.
	crmTenantDB command.TenantMongoResolver
}

// newInstrumentCascadeAdapter builds the instrument cascade over the CRM service.
// crmManager is the CRM Mongo manager, nil in single-tenant mode; it is stored only
// when non-nil, because a nil *tmmongo.Manager inside the interface would be a
// non-nil resolver.
func newInstrumentCascadeAdapter(service instrumentCascader, crmManager *tmmongo.Manager) instrumentCascadeAdapter {
	adapter := instrumentCascadeAdapter{service: service}
	if crmManager != nil {
		adapter.crmTenantDB = crmManager
	}

	return adapter
}

// SoftDeleteInstrumentsByAccount soft-deletes the account's live instruments in
// the request tenant's CRM database. The account-delete route binds only the
// module-keyed ledger stores, so the CRM database is resolved here; the derived
// context is scoped to the cascade and never returned to the caller.
func (a instrumentCascadeAdapter) SoftDeleteInstrumentsByAccount(ctx context.Context, organizationID, ledgerID, accountID uuid.UUID) (int, error) {
	if a.crmTenantDB != nil {
		tenantCtx, err := command.ResolveTenantMongoContext(ctx, a.crmTenantDB, "account delete instrument cascade")
		if err != nil {
			return 0, err
		}

		ctx = tenantCtx
	}

	return a.service.DeleteInstrumentsByAccount(ctx, organizationID.String(), ledgerID, accountID)
}

// feeAliasDetachAdapter satisfies command.FeeAliasDetacher over the fee service,
// so the account delete can remove the account alias from fee and billing
// packages without the command package importing fees.
type feeAliasDetachAdapter struct {
	service feeAliasDetacher

	// feesTenantDB resolves the tenant fees database in multi-tenant mode. It is
	// nil in single-tenant mode, where the fee repos use their static connection.
	feesTenantDB command.TenantMongoResolver
}

// newFeeAliasDetachAdapter builds the alias detach over the fee service.
// feesManager is the fees Mongo manager, nil in single-tenant mode; it is stored
// only when non-nil, because a nil *tmmongo.Manager inside the interface would be
// a non-nil resolver.
func newFeeAliasDetachAdapter(service feeAliasDetacher, feesManager *tmmongo.Manager) feeAliasDetachAdapter {
	adapter := feeAliasDetachAdapter{service: service}
	if feesManager != nil {
		adapter.feesTenantDB = feesManager
	}

	return adapter
}

// DetachAccountAlias removes alias from the packages in the request tenant's fees
// database. The account-delete route binds only the module-keyed ledger stores,
// so the fees database is resolved here; the derived context is scoped to the
// detach and never returned to the caller.
func (a feeAliasDetachAdapter) DetachAccountAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (model.FeeAliasDetachResult, error) {
	if a.feesTenantDB != nil {
		tenantCtx, err := command.ResolveTenantMongoContext(ctx, a.feesTenantDB, "account delete fee detach")
		if err != nil {
			return model.FeeAliasDetachResult{}, err
		}

		ctx = tenantCtx
	}

	return a.service.DetachAccountAlias(ctx, organizationID, ledgerID, alias)
}
