// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"testing"

	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestInitContextLimitDefinitionPolicy(t *testing.T) {
	t.Parallel()

	for _, producers := range []string{"", "   "} {
		cfg := validContextPolicyConfig()
		cfg.TracerPlatformProducers = producers
		cfg.ContextLimitMaxScopes = 0

		disabled, err := initContextLimitDefinitionPolicy(cfg, nil)
		require.NoError(t, err, "a validations-only Tracer reads no limit bounds")
		require.Nil(t, disabled, "a validations-only Tracer accepts any limit scope")
	}

	for _, tc := range []struct {
		name   string
		mutate func(*Config)
		valid  bool
	}{
		{name: "valid", mutate: func(*Config) {}, valid: true},
		{name: "invalid bounds", mutate: func(c *Config) { c.ContextMaxAccounts = 0 }},
		{name: "invalid fraction digits", mutate: func(c *Config) { c.ContextMaxFractionDigits = "1" }},
		{name: "no scopes", mutate: func(c *Config) { c.ContextLimitMaxScopes = 0 }},
		{name: "no scope bytes", mutate: func(c *Config) { c.ContextLimitMaxScopeBytes = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validContextPolicyConfig()
			cfg.TracerPlatformProducers = testPlatformProducers
			cfg.ContextLimitMaxScopes = 100
			cfg.ContextLimitMaxScopeBytes = 32768
			tc.mutate(cfg)

			policy, err := initContextLimitDefinitionPolicy(cfg, nil)
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, policy)

				return
			}

			require.Error(t, err)
			require.Nil(t, policy)
		})
	}
}

// unusedAuditWriter satisfies the constructor; neither path under test
// reaches the audit write.
type unusedAuditWriter struct{ command.AuditWriter }

// TestLimitCreateScopeFollowsTheReservationSurface drives a merchant-scoped
// limit through the create command wired with the bootstrap policy. With the
// reservation surface enabled the policy refuses it before any write; without
// it the command proceeds to persistence, observed as the transaction begin.
func TestLimitCreateScopeFollowsTheReservationSurface(t *testing.T) {
	t.Parallel()

	errPersistReached := errors.New("persistence reached")

	for _, tc := range []struct {
		name      string
		producers string
		begins    int
		want      error
	}{
		{name: "validations-only accepts a merchant scope", producers: "", begins: 1, want: errPersistReached},
		{name: "reservation surface rejects a merchant scope", producers: testPlatformProducers, begins: 0, want: constant.ErrContextLimitsUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validContextPolicyConfig()
			cfg.TracerPlatformProducers = tc.producers
			cfg.ContextLimitMaxScopes = 10
			cfg.ContextLimitMaxScopeBytes = 4096

			policy, err := initContextLimitDefinitionPolicy(cfg, nil)
			require.NoError(t, err)

			ctrl := gomock.NewController(t)
			txBeginner := dbmocks.NewMockTxBeginner(ctrl)
			txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(nil, errPersistReached).Times(tc.begins)

			cmd, err := command.NewCreateLimitCommand(command.NewMockLimitRepository(ctrl), testutil.NewDefaultMockClock(), unusedAuditWriter{}, txBeginner)
			require.NoError(t, err)

			cmd.ContextLimits = policy

			merchant := testutil.MustDeterministicUUID(921)
			result, err := cmd.Execute(t.Context(), &command.CreateLimitInput{
				Name: "Merchant limit", LimitType: model.LimitTypeDaily, Asset: "BRL",
				MaxAmount: decimal.NewFromInt(100), Scopes: []model.Scope{{MerchantID: &merchant}},
			})
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, result)
		})
	}
}

func TestInitContextLimitDefinitionPolicyMultiTenantRequiresTenancy(t *testing.T) {
	t.Parallel()

	cfg := validContextPolicyConfig()
	cfg.MultiTenantEnabled = true
	cfg.TracerPlatformProducers = ""
	cfg.ContextLimitMaxScopes = 10
	cfg.ContextLimitMaxScopeBytes = 4096

	policy, err := initContextLimitDefinitionPolicy(cfg, nil)
	require.ErrorContains(t, err, "multi-tenant limit administration requires the tenant reservation tenancy")
	require.Nil(t, policy)

	policy, err = initContextLimitDefinitionPolicy(cfg, producerauth.NewServiceTenancy(func(context.Context, string, string) error { return nil }, producerauth.ServiceLedger))
	require.NoError(t, err, "multi-tenancy installs the policy without TRACER_PLATFORM_PRODUCERS")
	require.NotNil(t, policy)
}

// TestLimitCreateScopeFollowsTheTenantLedgerAssociation drives a
// merchant-scoped limit through the multi-tenant create command: the policy
// refuses it for a tenant the tenant-manager lists as active for the ledger,
// accepts it for any other tenant, and refuses the write when the
// tenant-manager cannot answer.
func TestLimitCreateScopeFollowsTheTenantLedgerAssociation(t *testing.T) {
	t.Parallel()

	errPersistReached := errors.New("persistence reached")

	const (
		ledgerTenant     = "tenant-ledger"
		validationTenant = "tenant-validations"
		unknownTenant    = "tenant-unknown"
	)

	lookup := func(_ context.Context, tenantID, service string) error {
		require.Equal(t, producerauth.ServiceLedger, service)

		switch tenantID {
		case ledgerTenant:
			return nil
		case validationTenant:
			return fmt.Errorf("tenant is not active for ledger: %w", tmcore.ErrTenantNotFound)
		default:
			return fmt.Errorf("%w for ledger", errActiveTenantsUnavailable)
		}
	}

	for _, tc := range []struct {
		name   string
		tenant string
		begins int
		want   error
	}{
		{name: "ledger tenant rejects a merchant scope", tenant: ledgerTenant, want: constant.ErrContextLimitsUnavailable},
		{name: "validations-only tenant accepts a merchant scope", tenant: validationTenant, begins: 1, want: errPersistReached},
		{name: "unanswerable tenant-manager refuses the write", tenant: unknownTenant, want: constant.ErrTenantServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := validContextPolicyConfig()
			cfg.MultiTenantEnabled = true
			cfg.TracerPlatformProducers = ""
			cfg.ContextLimitMaxScopes = 10
			cfg.ContextLimitMaxScopeBytes = 4096

			policy, err := initContextLimitDefinitionPolicy(cfg, producerauth.NewServiceTenancy(lookup, producerauth.ServiceLedger))
			require.NoError(t, err)

			ctrl := gomock.NewController(t)
			txBeginner := dbmocks.NewMockTxBeginner(ctrl)
			txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(nil, errPersistReached).Times(tc.begins)

			cmd, err := command.NewCreateLimitCommand(command.NewMockLimitRepository(ctrl), testutil.NewDefaultMockClock(), unusedAuditWriter{}, txBeginner)
			require.NoError(t, err)

			cmd.ContextLimits = policy

			merchant := testutil.MustDeterministicUUID(922)
			result, err := cmd.Execute(tmcore.ContextWithTenantID(t.Context(), tc.tenant), &command.CreateLimitInput{
				Name: "Merchant limit", LimitType: model.LimitTypeDaily, Asset: "BRL",
				MaxAmount: decimal.NewFromInt(100), Scopes: []model.Scope{{MerchantID: &merchant}},
			})
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, result)
		})
	}
}

func TestDeferredTenantLookupIsUnavailableUntilBound(t *testing.T) {
	t.Parallel()

	deferred := &deferredTenantLookup{}
	tenancy := producerauth.NewServiceTenancy(deferred.Lookup, producerauth.ServiceLedger)
	ctx := tmcore.ContextWithTenantID(t.Context(), "tenant-a")

	applies, err := tenancy.Applies(ctx)
	require.ErrorIs(t, err, constant.ErrTenantServiceUnavailable, "an unbound lookup is never a denial")
	require.False(t, applies)

	require.ErrorIs(t, deferred.bind(nil), errTenantAssociationsUnbound, "multi-tenant boot without components is refused")
	require.ErrorIs(t, deferred.bind(&componentsMT{}), errTenantAssociationsUnbound, "multi-tenant boot without the association set is refused")

	_, err = tenancy.Applies(ctx)
	require.ErrorIs(t, err, constant.ErrTenantServiceUnavailable, "a refused bind installs nothing")

	lister := &fakeTenantLister{}
	lister.set(nil, &tmclient.TenantSummary{ID: "tenant-a", Status: "active"})
	sets := newActiveTenantSets(lister, activeTenantSetConfig{}, producerauth.ServiceLedger)
	require.NoError(t, deferred.bind(&componentsMT{tenantAssociations: sets}))

	applies, err = tenancy.Applies(ctx)
	require.NoError(t, err)
	require.True(t, applies)
}

func TestDeferredTenantLookupSingleTenantBindsNothing(t *testing.T) {
	t.Parallel()

	var deferred *deferredTenantLookup

	require.NoError(t, deferred.bind(nil), "single-tenant mode has no lookup to bind")
}
