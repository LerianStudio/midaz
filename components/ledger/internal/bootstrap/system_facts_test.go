// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libSystemFacts "github.com/LerianStudio/lib-system-facts"
	"github.com/LerianStudio/lib-system-facts/noop"
	"github.com/LerianStudio/lib-system-facts/transport/httpsync/accessmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// enabledSystemFactsConfig is a single-tenant configuration the validator
// accepts; each case below breaks exactly one of its invariants.
func enabledSystemFactsConfig() Config {
	return Config{
		AuthEnabled:                true,
		DeploymentMode:             DeploymentModeBYOC,
		SystemFactsEnabled:         true,
		SystemFactsURL:             "https://system-facts.lerian.internal",
		SystemFactsM2MClientID:     "ledger",
		SystemFactsM2MClientSecret: "s3cret",
	}
}

func TestValidateSystemFactsConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(cfg *Config)
		wantErr string
	}{
		{
			name: "disabled reads nothing else",
			mutate: func(cfg *Config) {
				*cfg = Config{
					SystemFactsEnabled:                      false,
					SystemFactsReconcileAttemptTimeoutMs:    -1,
					SystemFactsRetentionInvalidHours:        -1,
					SystemFactsRetentionPublishedHours:      -1,
					SystemFactsForwarderBatchSize:           -1,
					SystemFactsForwarderDispatchIntervalMs:  -1,
					SystemFactsForwarderProcessingTimeoutMs: -1,
					SystemFactsSourceServiceCheck:           "off",
					SystemFactsInsecureHTTP:                 true,
				}
			},
		},
		{
			name:   "enabled single tenant with id and secret passes",
			mutate: func(*Config) {},
		},
		{
			name:    "enabled without url",
			mutate:  func(cfg *Config) { cfg.SystemFactsURL = "  " },
			wantErr: "SYSTEM_FACTS_URL",
		},
		{
			name:    "enabled without auth",
			mutate:  func(cfg *Config) { cfg.AuthEnabled = false },
			wantErr: "PLUGIN_AUTH_ENABLED=true",
		},
		{
			name:    "single tenant without client id",
			mutate:  func(cfg *Config) { cfg.SystemFactsM2MClientID = "" },
			wantErr: "SYSTEM_FACTS_M2M_CLIENT_ID/_SECRET: the producer's own credential single-tenant",
		},
		{
			name:    "single tenant without client secret",
			mutate:  func(cfg *Config) { cfg.SystemFactsM2MClientSecret = " " },
			wantErr: "SYSTEM_FACTS_M2M_CLIENT_ID/_SECRET",
		},
		{
			name:   "multi tenant with the pair passes",
			mutate: func(cfg *Config) { cfg.MultiTenantEnabled = true },
		},
		{
			name: "multi tenant without the pair is refused: the tenantless service credential registers the contract",
			mutate: func(cfg *Config) {
				cfg.MultiTenantEnabled = true
				cfg.SystemFactsM2MClientID = ""
				cfg.SystemFactsM2MClientSecret = ""
			},
			wantErr: "the tenantless service credential that registers the contract multi-tenant",
		},
		{
			name: "insecure http outside local",
			mutate: func(cfg *Config) {
				cfg.DeploymentMode = DeploymentModeSaaS
				cfg.SystemFactsInsecureHTTP = true
			},
			wantErr: "SYSTEM_FACTS_INSECURE_HTTP=true requires DEPLOYMENT_MODE=local",
		},
		{
			name: "insecure http in local passes",
			mutate: func(cfg *Config) {
				cfg.DeploymentMode = DeploymentModeLocal
				cfg.SystemFactsInsecureHTTP = true
			},
		},
		{
			name:    "reconcile attempt timeout below the floor",
			mutate:  func(cfg *Config) { cfg.SystemFactsReconcileAttemptTimeoutMs = 4999 },
			wantErr: "SYSTEM_FACTS_RECONCILE_ATTEMPT_TIMEOUT_MS",
		},
		{
			name:    "reconcile attempt timeout above the ceiling",
			mutate:  func(cfg *Config) { cfg.SystemFactsReconcileAttemptTimeoutMs = 600001 },
			wantErr: "SYSTEM_FACTS_RECONCILE_ATTEMPT_TIMEOUT_MS",
		},
		{
			name:   "reconcile attempt timeout at the floor passes",
			mutate: func(cfg *Config) { cfg.SystemFactsReconcileAttemptTimeoutMs = 5000 },
		},
		{
			name:   "reconcile attempt timeout at the ceiling passes",
			mutate: func(cfg *Config) { cfg.SystemFactsReconcileAttemptTimeoutMs = 600000 },
		},
		{
			name:    "negative retention",
			mutate:  func(cfg *Config) { cfg.SystemFactsRetentionInvalidHours = -1 },
			wantErr: "SYSTEM_FACTS_RETENTION_INVALID_HOURS",
		},
		{
			name: "insecure http with the deployment mode left empty is refused",
			mutate: func(cfg *Config) {
				cfg.DeploymentMode = ""
				cfg.SystemFactsInsecureHTTP = true
			},
			wantErr: "SYSTEM_FACTS_INSECURE_HTTP=true requires DEPLOYMENT_MODE=local",
		},
		{
			name:    "negative published retention",
			mutate:  func(cfg *Config) { cfg.SystemFactsRetentionPublishedHours = -1 },
			wantErr: "SYSTEM_FACTS_RETENTION_PUBLISHED_HOURS",
		},
		{
			name:    "negative forwarder batch size",
			mutate:  func(cfg *Config) { cfg.SystemFactsForwarderBatchSize = -1 },
			wantErr: "SYSTEM_FACTS_FORWARDER_BATCH_SIZE",
		},
		{
			name:    "negative forwarder dispatch interval",
			mutate:  func(cfg *Config) { cfg.SystemFactsForwarderDispatchIntervalMs = -1 },
			wantErr: "SYSTEM_FACTS_FORWARDER_DISPATCH_INTERVAL_MS",
		},
		{
			name:    "negative forwarder processing timeout",
			mutate:  func(cfg *Config) { cfg.SystemFactsForwarderProcessingTimeoutMs = -1 },
			wantErr: "SYSTEM_FACTS_FORWARDER_PROCESSING_TIMEOUT_MS",
		},
		{
			name: "forwarder tuning at explicit values passes",
			mutate: func(cfg *Config) {
				cfg.SystemFactsForwarderBatchSize = 25
				cfg.SystemFactsForwarderDispatchIntervalMs = 250
				cfg.SystemFactsForwarderProcessingTimeoutMs = 120000
				cfg.SystemFactsRetentionPublishedHours = 48
			},
		},
		{
			name:   "source service check require passes",
			mutate: func(cfg *Config) { cfg.SystemFactsSourceServiceCheck = "require" },
		},
		{
			name:   "source service check warn with padding and case passes",
			mutate: func(cfg *Config) { cfg.SystemFactsSourceServiceCheck = "  Warn " },
		},
		{
			name:    "source service check off is refused",
			mutate:  func(cfg *Config) { cfg.SystemFactsSourceServiceCheck = "off" },
			wantErr: "SYSTEM_FACTS_SOURCE_SERVICE_CHECK",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := enabledSystemFactsConfig()
			tt.mutate(&cfg)

			err := validateSystemFactsConfig(&cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestSystemFactsReconcileAttemptTimeout_DefaultsWhenUnset(t *testing.T) {
	t.Parallel()

	timeout, err := systemFactsReconcileAttemptTimeout(&Config{})
	require.NoError(t, err)
	assert.Equal(t, defaultSystemFactsReconcileAttemptTimeout, timeout)
	assert.Equal(t, time.Minute, timeout)
}

// systemFactsLoopbackURL is a loopback address the kit accepts over plain http
// without the insecure option; nothing listens on it, and nothing dials it.
const systemFactsLoopbackURL = "http://127.0.0.1:1"

func systemFactsAuthClient() *middleware.AuthClient {
	return &middleware.AuthClient{Enabled: true, Address: systemFactsLoopbackURL}
}

// offlineTransactionClient is a lib-commons client that connects lazily, so a
// producer build that runs no SQL needs no database.
func offlineTransactionClient(t *testing.T) *libPostgres.Client {
	t.Helper()

	client, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN: "postgres://user:pass@localhost:5432/transaction?sslmode=require",
	})
	require.NoError(t, err)

	return client
}

func TestBuildSystemFacts_Disabled_WiresNoopRecorderAndNothingElse(t *testing.T) {
	t.Parallel()

	components, err := buildSystemFacts(context.Background(), &Config{}, libLog.NewNop(), nil, nil, nil, nil, systemFactsDeps{})
	require.NoError(t, err)

	require.NotNil(t, components.recorder)
	assert.IsType(t, noop.Emitter{}, components.emitter, "disabled wires the kit's no-op emitter")
	assert.Nil(t, components.producer)
	assert.Nil(t, components.gate, "disabled builds no registry client and no reconciler")
}

func TestBuildSystemFacts_Enabled_RecordsFromTheFirstCommitAndWiresTheGate(t *testing.T) {
	t.Parallel()

	cfg := enabledSystemFactsConfig()
	cfg.SystemFactsURL = systemFactsLoopbackURL

	components, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
		systemFactsDeps{authClient: systemFactsAuthClient()})
	require.NoError(t, err)

	require.NotNil(t, components.recorder)
	require.NotNil(t, components.producer)
	require.NotNil(t, components.emitter)

	_, isNoop := components.emitter.(noop.Emitter)
	assert.False(t, isNoop, "enabled records from the first commit, whatever the registry answers later")

	require.NotNil(t, components.gate, "the forwarder is gated on the boot reconcile")
	assert.Equal(t, systemFactsContractPending, components.gate.contractState(), "nothing is reconciled at build time")
	assert.Equal(t, defaultSystemFactsReconcileAttemptTimeout, components.gate.attemptTimeout)

	kit, ok := components.gate.reconciler.(*kitSystemFactsReconciler)
	require.True(t, ok, "the production reconciler is the kit over the embedded manifest, got %T", components.gate.reconciler)
	assert.NotNil(t, kit.manifest)
	assert.NotNil(t, kit.registry)
}

func TestBuildSystemFacts_GateCarriesTheConfiguredAttemptTimeout(t *testing.T) {
	t.Parallel()

	cfg := enabledSystemFactsConfig()
	cfg.SystemFactsURL = systemFactsLoopbackURL
	cfg.SystemFactsReconcileAttemptTimeoutMs = 7500

	components, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
		systemFactsDeps{authClient: systemFactsAuthClient()})
	require.NoError(t, err)

	require.NotNil(t, components.gate)
	assert.Equal(t, 7500*time.Millisecond, components.gate.attemptTimeout)
}

func TestBuildSystemFacts_Enabled_RefusesAnAuthClientThatCannotMint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		client *middleware.AuthClient
	}{
		{name: "nil client"},
		{name: "disabled client", client: &middleware.AuthClient{Enabled: false, Address: systemFactsLoopbackURL}},
		{name: "client without address", client: &middleware.AuthClient{Enabled: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := enabledSystemFactsConfig()
			cfg.SystemFactsURL = systemFactsLoopbackURL

			_, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
				systemFactsDeps{authClient: tt.client})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "PLUGIN_AUTH_HOST")
		})
	}
}

func TestBuildSystemFacts_MultiTenant_RequiresTheTenantPoolManager(t *testing.T) {
	t.Parallel()

	cfg := enabledSystemFactsConfig()
	cfg.SystemFactsURL = systemFactsLoopbackURL
	cfg.MultiTenantEnabled = true

	_, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
		systemFactsDeps{authClient: systemFactsAuthClient(), tenantServiceName: "ledger"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tenant")
}

// systemFactsTenantSecretPath is where the tenant-manager writes the ledger's
// System Facts credential for seamTenantID in the staging environment: the same
// layout as the tracer seam, under the system-facts target segment.
const systemFactsTenantSecretPath = "tenants/staging/0198a1b2c3d47e5f8a9b0c1d2e3f4a5b/ledger/m2m/system-facts/credentials"

func TestSystemFactsTenantCredentials_ReadsTheTenantSecretOnce(t *testing.T) {
	t.Parallel()

	secrets := &secretIDRecorder{}

	creds, err := systemFactsTenantCredentials(secrets, "staging", "ledger")
	require.NoError(t, err)

	for range 2 {
		clientID, clientSecret, err := creds(context.Background(), seamTenantID)
		require.NoError(t, err)
		assert.Equal(t, "ledger-tenant", clientID)
		assert.Equal(t, "s3cret", clientSecret)
	}

	assert.Equal(t, []string{systemFactsTenantSecretPath}, secrets.read(), "the second lookup is answered from the cache")
}

func TestSystemFactsTenantCredentials_RefusesAnEmptyTenant(t *testing.T) {
	t.Parallel()

	creds, err := systemFactsTenantCredentials(&secretIDRecorder{}, "staging", "ledger")
	require.NoError(t, err)

	_, _, err = creds(context.Background(), "")
	require.Error(t, err)
}

// systemFactsTenantPools builds the transaction tenant pool manager over a
// tenant-manager client that is never dialled: the producer build opens no
// connection.
func systemFactsTenantPools(t *testing.T) (*tmclient.Client, *tmpostgres.Manager) {
	t.Helper()

	tenantClient, err := tmclient.NewClient("http://localhost:0", libLog.NewNop(),
		tmclient.WithAllowInsecureHTTP(), tmclient.WithServiceAPIKey("test-api-key"))
	require.NoError(t, err)

	return tenantClient, tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(libLog.NewNop()))
}

// multiTenantSystemFactsDeps are the collaborators of a multi-tenant build
// whose tenant secrets are answered by secrets.
func multiTenantSystemFactsDeps(secrets *secretIDRecorder, tenantClient *tmclient.Client) systemFactsDeps {
	return systemFactsDeps{
		authClient:        systemFactsAuthClient(),
		tenantServiceName: "ledger",
		tenantClient:      tenantClient,
		newAWSSecretsClient: func(context.Context) (secretsmanager.SecretsManagerClient, error) {
			return secrets, nil
		},
	}
}

func TestBuildSystemFacts_MultiTenant_BuildsThePerTenantProducer(t *testing.T) {
	t.Parallel()

	cfg := enabledSystemFactsConfig()
	cfg.SystemFactsURL = systemFactsLoopbackURL
	cfg.MultiTenantEnabled = true
	cfg.EnvName = "staging"
	secrets := &secretIDRecorder{}
	tenantClient, pgManager := systemFactsTenantPools(t)

	components, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), pgManager,
		multiTenantSystemFactsDeps(secrets, tenantClient))
	require.NoError(t, err)

	require.NotNil(t, components.producer)
	require.NotNil(t, components.gate)
	assert.Equal(t, systemFactsContractPending, components.gate.contractState())
	assert.Empty(t, secrets.read(), "no tenant secret is read until a tenant's facts are delivered")
}

func TestBuildSystemFactsTokenSource_RegistrationCredential(t *testing.T) {
	t.Parallel()

	t.Run("single tenant registers with the producer's own credential", func(t *testing.T) {
		t.Parallel()

		cfg := enabledSystemFactsConfig()
		cfg.SystemFactsURL = systemFactsLoopbackURL

		delivery, registration, err := buildSystemFactsTokenSource(context.Background(), &cfg,
			systemFactsDeps{authClient: systemFactsAuthClient()})
		require.NoError(t, err)

		assert.IsType(t, &accessmanager.TokenSource{}, delivery)
		assert.True(t, delivery == registration, "the registration source IS the delivery source")
	})

	t.Run("multi tenant registers with a tenantless service credential and delivers per tenant", func(t *testing.T) {
		t.Parallel()

		cfg := enabledSystemFactsConfig()
		cfg.SystemFactsURL = systemFactsLoopbackURL
		cfg.MultiTenantEnabled = true
		cfg.EnvName = "staging"
		secrets := &secretIDRecorder{}
		tenantClient, _ := systemFactsTenantPools(t)

		delivery, registration, err := buildSystemFactsTokenSource(context.Background(), &cfg,
			multiTenantSystemFactsDeps(secrets, tenantClient))
		require.NoError(t, err)

		assert.IsType(t, &accessmanager.TenantTokenSource{}, delivery, "delivery authenticates with each tenant's own credential")
		assert.IsType(t, &accessmanager.TokenSource{}, registration, "registration authenticates with the static service pair")
		assert.Empty(t, secrets.read(), "building the sources reads no tenant secret")
	})
}

func TestSystemFactsForwarderTuning(t *testing.T) {
	t.Parallel()

	assert.Equal(t, libSystemFacts.ForwarderTuning{}, systemFactsForwarderTuning(&Config{}),
		"zero values are left for the kit to default")

	cfg := &Config{
		SystemFactsForwarderBatchSize:           25,
		SystemFactsForwarderDispatchIntervalMs:  250,
		SystemFactsForwarderProcessingTimeoutMs: 120000,
		SystemFactsRetentionPublishedHours:      48,
		SystemFactsRetentionInvalidHours:        72,
	}

	assert.Equal(t, libSystemFacts.ForwarderTuning{
		BatchSize:          25,
		DispatchInterval:   250 * time.Millisecond,
		ProcessingTimeout:  2 * time.Minute,
		RetentionPublished: 48 * time.Hour,
		RetentionInvalid:   72 * time.Hour,
	}, systemFactsForwarderTuning(cfg))
}

func TestSystemFactsSourceServiceCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		want    accessmanager.SourceServiceCheck
		wantErr bool
	}{
		{name: "unset is warn", value: "", want: accessmanager.SourceServiceCheckWarn},
		{name: "warn", value: "warn", want: accessmanager.SourceServiceCheckWarn},
		{name: "require", value: "require", want: accessmanager.SourceServiceCheckRequire},
		{name: "padding and case are ignored", value: " Require ", want: accessmanager.SourceServiceCheckRequire},
		{name: "off is refused", value: "off", wantErr: true},
		{name: "anything else is refused", value: "strict", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := systemFactsSourceServiceCheck(&Config{SystemFactsSourceServiceCheck: tt.value})
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "SYSTEM_FACTS_SOURCE_SERVICE_CHECK")

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBuildSystemFacts_RefusesAForwarderBatchThatOutlivesItsClaim pins the
// kit's claim invariant at boot: a batch of 100 under the default attempts and
// delivery timeout cannot finish inside the default processing timeout.
func TestBuildSystemFacts_RefusesAForwarderBatchThatOutlivesItsClaim(t *testing.T) {
	t.Parallel()

	cfg := enabledSystemFactsConfig()
	cfg.SystemFactsURL = systemFactsLoopbackURL
	cfg.SystemFactsForwarderBatchSize = 100

	_, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
		systemFactsDeps{authClient: systemFactsAuthClient()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "build the System Facts producer")
	assert.Contains(t, err.Error(), "ProcessingTimeout")
	assert.Contains(t, err.Error(), "BatchSize 100")
}
