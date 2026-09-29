// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/producerauth"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// startLedgerTenantManager serves the active-tenant list: bindingLedgerTenant
// is the only tenant active for the ledger, and no tenant is active for any
// other service, so the worker supervisor stays idle.
func startLedgerTenantManager(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v1/tenants/active") {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.Header().Set("Content-Type", "application/json")

		body := `[]`
		if r.URL.Query().Get("service") == producerauth.ServiceLedger {
			body = `[{"id":"` + bindingLedgerTenant + `","name":"ledger tenant","status":"active"}]`
		}

		_, _ = w.Write([]byte(body))
	}))

	t.Cleanup(srv.Close)

	return srv
}

const (
	bindingLedgerTenant = "tenant-ledger-bound"
	bindingOtherTenant  = "tenant-validations-only"
)

// TestMultiTenantStackBindsTheLimitTenancy boots the multi-tenant limit stack
// the way InitServers does: the limit commands are built first, the
// multi-tenant components after them. Until the stack is built every lookup
// is an outage; once it is built, the create command's definition policy asks
// the tenant-manager and refuses a merchant scope for the listed ledger tenant
// while accepting it for any other tenant.
func TestMultiTenantStackBindsTheLimitTenancy(t *testing.T) {
	t.Parallel()

	mr := miniredis.RunT(t)
	redisHost, redisPort := splitHostPort(t, mr.Addr())
	tm := startLedgerTenantManager(t)

	cfg := validContextPolicyConfig()
	cfg.ApplicationName = "tracer"
	cfg.MultiTenantEnabled = true
	cfg.MultiTenantURL = tm.URL
	cfg.MultiTenantServiceAPIKey = "svc-api-key"
	cfg.MultiTenantRedisHost = redisHost
	cfg.MultiTenantRedisPort = redisPort
	cfg.MultiTenantTimeout = 30
	cfg.MultiTenantCacheTTLSec = 60
	cfg.MultiTenantCircuitBreakerThreshold = 5
	cfg.MultiTenantCircuitBreakerTimeoutSec = 30
	cfg.MultiTenantMaxTenantPools = 100
	cfg.MultiTenantIdleTimeoutSec = 300
	cfg.MultiTenantConnectionsCheckIntervalSec = 30
	cfg.MultiTenantAllowInsecureHTTP = true
	cfg.ContextLimitMaxScopes = 10
	cfg.ContextLimitMaxScopeBytes = 4096

	errPersistReached := errors.New("persistence reached")

	ctrl := gomock.NewController(t)
	txBeginner := dbmocks.NewMockTxBeginner(ctrl)
	txBeginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(nil, errPersistReached).Times(1)

	limitDeps, err := initLimitService(cfg, dbmocks.NewMockConnection(ctrl), unusedAuditWriter{}, testutil.NewDefaultMockClock(), txBeginner, nil)
	require.NoError(t, err)
	require.NotNil(t, limitDeps.reservationTenancy, "multi-tenant limit commands carry a deferred tenant lookup")

	createMerchantLimit := func(tenantID string) error {
		merchant := testutil.MustDeterministicUUID(931)
		_, err := limitDeps.service.CreateLimit(tmcore.ContextWithTenantID(t.Context(), tenantID), &command.CreateLimitInput{
			Name: "Merchant limit", LimitType: model.LimitTypeDaily, Asset: "BRL",
			MaxAmount: decimal.NewFromInt(100), Scopes: []model.Scope{{MerchantID: &merchant}},
		})

		return err
	}

	require.ErrorIs(t, createMerchantLimit(bindingLedgerTenant), constant.ErrTenantServiceUnavailable,
		"before the multi-tenant stack exists the lookup is an outage, never a denial")

	logger := testutil.NewMockLogger()
	celAdapter, err := cel.NewAdapter(cel.AdapterConfig{CostLimit: 10000}, logger)
	require.NoError(t, err)

	mtComponents, _, err := buildMultiTenantStack(t.Context(), cfg, logger, nil, cache.NewRuleCache(clock.RealClock{}), nil, limitDeps, celAdapter, clock.RealClock{})
	require.NoError(t, err)
	require.NotNil(t, mtComponents)
	t.Cleanup(func() {
		mtComponents.supervisor.Shutdown()
		_ = mtComponents.tmClient.Close()
	})

	require.ErrorIs(t, createMerchantLimit(bindingLedgerTenant), constant.ErrContextLimitsUnavailable,
		"the bound lookup lists the tenant as ledger-associated, so a merchant scope is refused")
	require.ErrorIs(t, createMerchantLimit(bindingOtherTenant), errPersistReached,
		"a tenant absent from the ledger list keeps any scope and reaches persistence")
}
