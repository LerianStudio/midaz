// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const wantTenantReadyzCarveOutReason = "multi-tenant: per-tenant databases are probed in the background; see tenant_dependency_up"

// tenantPostgresCheckerNames are the readiness checks that a multi-tenant
// deployment reports as not applicable.
var tenantPostgresCheckerNames = []string{"postgres_onboarding", "postgres_transaction"}

func checkersByName(checkers []DependencyChecker) map[string]DependencyChecker {
	byName := make(map[string]DependencyChecker, len(checkers))
	for _, checker := range checkers {
		byName[checker.Name()] = checker
	}

	return byName
}

func TestBuildReadyzHandler_MultiTenantCarvesOutTenantPostgres(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		connection *libPostgres.Client
	}{
		{name: "without_fallback_connection", connection: nil},
		{name: "with_fallback_connection", connection: &libPostgres.Client{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &Config{
				DeploymentMode:      DeploymentModeLocal,
				MultiTenantEnabled:  true,
				CrmPrefixedMongoURI: "mongodb://localhost:27017",
			}

			handler, err := buildReadyzHandler(cfg, libLog.NewNop(), nil,
				&onboardingPostgresComponents{connection: tt.connection, pgManager: &tmpostgres.Manager{}},
				&transactionPostgresComponents{connection: tt.connection, pgManager: &tmpostgres.Manager{}},
				&onboardingMongoComponents{}, &transactionMongoComponents{},
				&crmComponents{}, nil, nil, nil)
			require.NoError(t, err)
			require.NotNil(t, handler)

			byName := checkersByName(handler.checkers)

			for _, name := range tenantPostgresCheckerNames {
				checker, ok := byName[name]
				require.True(t, ok, "checker %s must be registered in multi-tenant mode", name)

				_, isNA := checker.(*NotApplicableChecker)
				assert.True(t, isNA, "checker %s must not probe the fallback connection", name)
				assert.False(t, checker.TLSEnabled(), "checker %s holds no connection", name)

				result := checker.Check(context.Background())
				assert.Equal(t, StatusNA, result.Status)
				assert.Equal(t, wantTenantReadyzCarveOutReason, result.Reason)
				assert.Nil(t, result.LatencyMs)
				assert.Empty(t, result.Error)
			}

			for _, checker := range handler.checkers {
				assert.NotContains(t, strings.ToLower(checker.Name()), "tenant",
					"readyz must not register a per-tenant checker")
			}

			// The not-applicable checks never mark the pod unready.
			ready := newReadyHandler(ReadyzHandlerConfig{
				Logger:         libLog.NewNop(),
				Checkers:       []DependencyChecker{byName["postgres_onboarding"], byName["postgres_transaction"]},
				DeploymentMode: cfg.DeploymentMode,
			})

			app := fiber.New()
			app.Get("/readyz", ready.HandleReadyz)

			resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/readyz", nil), fiber.TestConfig{Timeout: 0})
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			var response ReadyzResponse
			require.NoError(t, json.Unmarshal(body, &response))
			assert.Equal(t, "healthy", response.Status)

			for _, name := range tenantPostgresCheckerNames {
				check, ok := response.Checks[name]
				require.True(t, ok, "response must carry check %s", name)
				assert.Equal(t, StatusNA, check.Status)
				assert.Equal(t, wantTenantReadyzCarveOutReason, check.Reason)
				assert.Nil(t, check.LatencyMs)
			}
		})
	}
}

func TestBuildReadyzHandler_MultiTenantSaaSBootsWithCarveOut(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		DeploymentMode:              DeploymentModeSaaS,
		MultiTenantEnabled:          true,
		CrmPrefixedMongoURI:         "mongodb://localhost:27017",
		OnbPrefixedPrimaryDBSSLMode: "require",
		TxnPrefixedPrimaryDBSSLMode: "require",
		RabbitURI:                   "amqps",
		RabbitMQHealthCheckURL:      "https://rabbitmq:15671",
	}

	handler, err := buildReadyzHandler(cfg, libLog.NewNop(), nil,
		&onboardingPostgresComponents{connection: &libPostgres.Client{}, pgManager: &tmpostgres.Manager{}},
		&transactionPostgresComponents{connection: &libPostgres.Client{}, pgManager: &tmpostgres.Manager{}},
		&onboardingMongoComponents{}, &transactionMongoComponents{},
		&crmComponents{}, nil, nil, nil)
	require.NoError(t, err, "a not-applicable check holds no connection and must not fail the SaaS TLS gate")
	require.NotNil(t, handler)
}

func TestBuildReadyzHandler_SingleTenantKeepsPostgresProbes(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		DeploymentMode:      DeploymentModeLocal,
		CrmPrefixedMongoURI: "mongodb://localhost:27017",
	}

	handler, err := buildReadyzHandler(cfg, libLog.NewNop(), nil,
		&onboardingPostgresComponents{connection: &libPostgres.Client{}},
		&transactionPostgresComponents{connection: &libPostgres.Client{}},
		&onboardingMongoComponents{}, &transactionMongoComponents{},
		&crmComponents{}, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, handler)

	byName := checkersByName(handler.checkers)

	for _, name := range tenantPostgresCheckerNames {
		checker, ok := byName[name]
		require.True(t, ok, "checker %s must be registered in single-tenant mode", name)

		_, isPostgres := checker.(*PostgresChecker)
		assert.True(t, isPostgres, "checker %s must probe the static connection", name)
	}
}

func TestBuildReadyzHandler_MultiTenantSaaSStillEnforcesFallbackPostgresTLS(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		DeploymentMode:              DeploymentModeSaaS,
		MultiTenantEnabled:          true,
		CrmPrefixedMongoURI:         "mongodb://localhost:27017",
		OnbPrefixedPrimaryDBSSLMode: "disable",
		TxnPrefixedPrimaryDBSSLMode: "require",
		RabbitURI:                   "amqps",
		RabbitMQHealthCheckURL:      "https://rabbitmq:15671",
	}

	_, err := buildReadyzHandler(cfg, libLog.NewNop(), nil,
		&onboardingPostgresComponents{connection: &libPostgres.Client{}, pgManager: &tmpostgres.Manager{}},
		&transactionPostgresComponents{connection: &libPostgres.Client{}, pgManager: &tmpostgres.Manager{}},
		&onboardingMongoComponents{}, &transactionMongoComponents{},
		&crmComponents{}, nil, nil, nil)
	require.Error(t, err, "the fallback connection is still opened in multi-tenant mode, so SaaS must keep enforcing its TLS")
	assert.Contains(t, err.Error(), "postgres_onboarding")
	assert.NotContains(t, err.Error(), "postgres_transaction")
}
