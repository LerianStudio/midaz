// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmmongo "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/mongo"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
)

var (
	cascadeOrganizationID = uuid.MustParse("6f1d2c3b-4a5e-4f60-8a71-0b2c3d4e5f60")
	cascadeLedgerID       = uuid.MustParse("7a2e3d4c-5b6f-4071-9b82-1c3d4e5f6071")
	cascadeAccountID      = uuid.MustParse("8b3f4e5d-6c70-4182-ac93-2d4e5f607182")
)

const cascadeAlias = "@cascade-account"

// fakeInstrumentCascader records the call DeleteInstrumentsByAccount receives.
type fakeInstrumentCascader struct {
	count int
	err   error

	called         bool
	ctx            context.Context
	organizationID string
	ledgerID       uuid.UUID
	accountID      uuid.UUID
}

func (f *fakeInstrumentCascader) DeleteInstrumentsByAccount(ctx context.Context, organizationID string, ledgerID, accountID uuid.UUID) (int, error) {
	f.called = true
	f.ctx = ctx
	f.organizationID = organizationID
	f.ledgerID = ledgerID
	f.accountID = accountID

	return f.count, f.err
}

// fakeFeeAliasDetacher records the call DetachAccountAlias receives.
type fakeFeeAliasDetacher struct {
	result model.FeeAliasDetachResult
	err    error

	called         bool
	ctx            context.Context
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	alias          string
}

func (f *fakeFeeAliasDetacher) DetachAccountAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) (model.FeeAliasDetachResult, error) {
	f.called = true
	f.ctx = ctx
	f.organizationID = organizationID
	f.ledgerID = ledgerID
	f.alias = alias

	return f.result, f.err
}

type cascadeCtxKey struct{}

func TestInstrumentCascadeAdapter_SingleTenant(t *testing.T) {
	t.Run("passes ctx unchanged and returns the cascaded count", func(t *testing.T) {
		service := &fakeInstrumentCascader{count: 2}
		adapter := instrumentCascadeAdapter{service: service}

		ctx := context.WithValue(context.Background(), cascadeCtxKey{}, "caller")

		count, err := adapter.SoftDeleteInstrumentsByAccount(ctx, cascadeOrganizationID, cascadeLedgerID, cascadeAccountID)
		require.NoError(t, err)
		assert.Equal(t, 2, count)

		require.True(t, service.called)
		assert.Equal(t, ctx, service.ctx, "single-tenant must hand the caller's ctx to the CRM service")
		assert.Equal(t, cascadeOrganizationID.String(), service.organizationID)
		assert.Equal(t, cascadeLedgerID, service.ledgerID)
		assert.Equal(t, cascadeAccountID, service.accountID)
	})

	t.Run("service error propagates unchanged", func(t *testing.T) {
		serviceErr := errors.New("mongo timeout")
		adapter := instrumentCascadeAdapter{service: &fakeInstrumentCascader{err: serviceErr}}

		count, err := adapter.SoftDeleteInstrumentsByAccount(context.Background(), cascadeOrganizationID, cascadeLedgerID, cascadeAccountID)
		require.ErrorIs(t, err, serviceErr)
		assert.Zero(t, count)
	})
}

// TestInstrumentCascadeAdapter_MultiTenant pins the CRM database resolution the cascade
// performs in multi-tenant mode: the account-delete route binds only the ledger stores, so
// without it the CRM repos would find no tenant database on the generic key.
func TestInstrumentCascadeAdapter_MultiTenant(t *testing.T) {
	crmDB := (&mongo.Client{}).Database("crm_tenant_a")

	t.Run("resolves the tenant CRM database for the cascade only", func(t *testing.T) {
		service := &fakeInstrumentCascader{count: 1}
		resolver := &fakeCRMTenantDatabase{db: crmDB}
		adapter := instrumentCascadeAdapter{service: service, crmTenantDB: resolver}

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

		count, err := adapter.SoftDeleteInstrumentsByAccount(ctx, cascadeOrganizationID, cascadeLedgerID, cascadeAccountID)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		assert.Equal(t, "tenant-a", resolver.tenantID, "the CRM database must be resolved for the caller's tenant")
		require.True(t, service.called)
		assert.Same(t, crmDB, tmcore.GetMBContext(service.ctx),
			"the cascade must see the tenant CRM database on the generic key the CRM repos read")
		assert.Nil(t, tmcore.GetMBContext(ctx), "the resolution must not leak onto the caller's context")
	})

	t.Run("missing tenant id fails without resolving or deleting", func(t *testing.T) {
		service := &fakeInstrumentCascader{}
		resolver := &fakeCRMTenantDatabase{db: crmDB}
		adapter := instrumentCascadeAdapter{service: service, crmTenantDB: resolver}

		count, err := adapter.SoftDeleteInstrumentsByAccount(context.Background(), cascadeOrganizationID, cascadeLedgerID, cascadeAccountID)
		require.ErrorIs(t, err, tmcore.ErrTenantNotFound)
		assert.Zero(t, count)
		assert.False(t, resolver.called, "without a tenant no CRM database may be resolved")
		assert.False(t, service.called, "without a tenant the cascade must not fall through to any store")
	})

	t.Run("service error propagates unchanged", func(t *testing.T) {
		serviceErr := errors.New("mongo timeout")
		adapter := instrumentCascadeAdapter{service: &fakeInstrumentCascader{err: serviceErr}, crmTenantDB: &fakeCRMTenantDatabase{db: crmDB}}

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

		_, err := adapter.SoftDeleteInstrumentsByAccount(ctx, cascadeOrganizationID, cascadeLedgerID, cascadeAccountID)
		require.ErrorIs(t, err, serviceErr)
	})
}

func TestFeeAliasDetachAdapter_SingleTenant(t *testing.T) {
	t.Run("passes ctx unchanged and returns the detach result", func(t *testing.T) {
		want := model.FeeAliasDetachResult{PackagesUpdated: 3, PackagesDisabled: 1}
		service := &fakeFeeAliasDetacher{result: want}
		adapter := feeAliasDetachAdapter{service: service}

		ctx := context.WithValue(context.Background(), cascadeCtxKey{}, "caller")

		got, err := adapter.DetachAccountAlias(ctx, cascadeOrganizationID, cascadeLedgerID, cascadeAlias)
		require.NoError(t, err)
		assert.Equal(t, want, got)

		require.True(t, service.called)
		assert.Equal(t, ctx, service.ctx, "single-tenant must hand the caller's ctx to the fee service")
		assert.Equal(t, cascadeOrganizationID, service.organizationID)
		assert.Equal(t, cascadeLedgerID, service.ledgerID)
		assert.Equal(t, cascadeAlias, service.alias)
	})

	t.Run("service error propagates unchanged", func(t *testing.T) {
		serviceErr := errors.New("mongo timeout")
		adapter := feeAliasDetachAdapter{service: &fakeFeeAliasDetacher{err: serviceErr}}

		got, err := adapter.DetachAccountAlias(context.Background(), cascadeOrganizationID, cascadeLedgerID, cascadeAlias)
		require.ErrorIs(t, err, serviceErr)
		assert.Equal(t, model.FeeAliasDetachResult{}, got)
	})
}

// TestFeeAliasDetachAdapter_MultiTenant pins the fees database resolution the detach
// performs in multi-tenant mode, for the same reason as the instrument cascade.
func TestFeeAliasDetachAdapter_MultiTenant(t *testing.T) {
	feesDB := (&mongo.Client{}).Database("fees_tenant_a")

	t.Run("resolves the tenant fees database for the detach only", func(t *testing.T) {
		want := model.FeeAliasDetachResult{PackagesUpdated: 1}
		service := &fakeFeeAliasDetacher{result: want}
		resolver := &fakeCRMTenantDatabase{db: feesDB}
		adapter := feeAliasDetachAdapter{service: service, feesTenantDB: resolver}

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

		got, err := adapter.DetachAccountAlias(ctx, cascadeOrganizationID, cascadeLedgerID, cascadeAlias)
		require.NoError(t, err)
		assert.Equal(t, want, got)

		assert.Equal(t, "tenant-a", resolver.tenantID, "the fees database must be resolved for the caller's tenant")
		require.True(t, service.called)
		assert.Same(t, feesDB, tmcore.GetMBContext(service.ctx),
			"the detach must see the tenant fees database on the generic key the fee repos read")
		assert.Nil(t, tmcore.GetMBContext(ctx), "the resolution must not leak onto the caller's context")
	})

	t.Run("missing tenant id fails without resolving or detaching", func(t *testing.T) {
		service := &fakeFeeAliasDetacher{}
		resolver := &fakeCRMTenantDatabase{db: feesDB}
		adapter := feeAliasDetachAdapter{service: service, feesTenantDB: resolver}

		got, err := adapter.DetachAccountAlias(context.Background(), cascadeOrganizationID, cascadeLedgerID, cascadeAlias)
		require.ErrorIs(t, err, tmcore.ErrTenantNotFound)
		assert.Equal(t, model.FeeAliasDetachResult{}, got)
		assert.False(t, resolver.called, "without a tenant no fees database may be resolved")
		assert.False(t, service.called, "without a tenant the detach must not fall through to any store")
	})

	t.Run("service error propagates unchanged", func(t *testing.T) {
		serviceErr := errors.New("mongo timeout")
		adapter := feeAliasDetachAdapter{service: &fakeFeeAliasDetacher{err: serviceErr}, feesTenantDB: &fakeCRMTenantDatabase{db: feesDB}}

		ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")

		_, err := adapter.DetachAccountAlias(ctx, cascadeOrganizationID, cascadeLedgerID, cascadeAlias)
		require.ErrorIs(t, err, serviceErr)
	})
}

// TestNewInstrumentCascadeAdapter pins the resolver wiring: a nil *tmmongo.Manager must not be
// stored, because inside the interface it is non-nil and the adapter would resolve through it
// instead of using the static CRM connection.
func TestNewInstrumentCascadeAdapter(t *testing.T) {
	t.Parallel()

	service := &fakeInstrumentCascader{}

	t.Run("single-tenant nil manager leaves no resolver", func(t *testing.T) {
		t.Parallel()

		adapter := newInstrumentCascadeAdapter(service, nil)
		assert.Same(t, service, adapter.service)
		assert.Nil(t, adapter.crmTenantDB, "a nil manager must leave the resolver interface nil")
	})

	t.Run("multi-tenant manager is kept as the resolver", func(t *testing.T) {
		t.Parallel()

		manager := &tmmongo.Manager{}
		adapter := newInstrumentCascadeAdapter(service, manager)
		assert.Same(t, manager, adapter.crmTenantDB, "the multi-tenant manager must be the resolver")
	})
}

// TestNewFeeAliasDetachAdapter pins the same nil-manager rule for the fees resolver.
func TestNewFeeAliasDetachAdapter(t *testing.T) {
	t.Parallel()

	service := &fakeFeeAliasDetacher{}

	t.Run("single-tenant nil manager leaves no resolver", func(t *testing.T) {
		t.Parallel()

		adapter := newFeeAliasDetachAdapter(service, nil)
		assert.Same(t, service, adapter.service)
		assert.Nil(t, adapter.feesTenantDB, "a nil manager must leave the resolver interface nil")
	})

	t.Run("multi-tenant manager is kept as the resolver", func(t *testing.T) {
		t.Parallel()

		manager := &tmmongo.Manager{}
		adapter := newFeeAliasDetachAdapter(service, manager)
		assert.Same(t, manager, adapter.feesTenantDB, "the multi-tenant manager must be the resolver")
	})
}
