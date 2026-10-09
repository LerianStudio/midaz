// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-system-facts/manifest"
	"github.com/LerianStudio/lib-system-facts/reconciler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
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
				*cfg = Config{SystemFactsEnabled: false, SystemFactsPreflightTimeoutMs: -1, SystemFactsRetentionInvalidHours: -1, SystemFactsInsecureHTTP: true}
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
			wantErr: "SYSTEM_FACTS_M2M_CLIENT_ID",
		},
		{
			name:    "single tenant without client secret",
			mutate:  func(cfg *Config) { cfg.SystemFactsM2MClientSecret = "" },
			wantErr: "SYSTEM_FACTS_M2M_CLIENT_SECRET",
		},
		{
			name: "multi tenant without client id passes",
			mutate: func(cfg *Config) {
				cfg.MultiTenantEnabled = true
				cfg.SystemFactsM2MClientID = ""
				cfg.SystemFactsM2MClientSecret = ""
			},
		},
		{
			name: "insecure http outside local",
			mutate: func(cfg *Config) {
				cfg.DeploymentMode = DeploymentModeSaaS
				cfg.SystemFactsInsecureHTTP = true
			},
			wantErr: "SYSTEM_FACTS_INSECURE_HTTP=true is only allowed when DEPLOYMENT_MODE=local",
		},
		{
			name: "insecure http in local passes",
			mutate: func(cfg *Config) {
				cfg.DeploymentMode = DeploymentModeLocal
				cfg.SystemFactsInsecureHTTP = true
			},
		},
		{
			name:    "preflight timeout below the floor",
			mutate:  func(cfg *Config) { cfg.SystemFactsPreflightTimeoutMs = 999 },
			wantErr: "SYSTEM_FACTS_PREFLIGHT_TIMEOUT_MS",
		},
		{
			name:    "preflight timeout above the ceiling",
			mutate:  func(cfg *Config) { cfg.SystemFactsPreflightTimeoutMs = 60001 },
			wantErr: "SYSTEM_FACTS_PREFLIGHT_TIMEOUT_MS",
		},
		{
			name:   "preflight timeout at the bounds passes",
			mutate: func(cfg *Config) { cfg.SystemFactsPreflightTimeoutMs = 60000 },
		},
		{
			name:    "negative retention",
			mutate:  func(cfg *Config) { cfg.SystemFactsRetentionInvalidHours = -1 },
			wantErr: "SYSTEM_FACTS_RETENTION_INVALID_HOURS",
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

func TestSystemFactsPreflightTimeout_DefaultsWhenUnset(t *testing.T) {
	t.Parallel()

	timeout, err := systemFactsPreflightTimeout(&Config{})
	require.NoError(t, err)
	assert.Equal(t, defaultSystemFactsPreflightTimeout, timeout)
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

// preflightRecorder is the reconciler seam: it answers a fixed report and
// records the deadline of the context it was called with.
type preflightRecorder struct {
	report reconciler.Report
	err    error

	mu          sync.Mutex
	calls       int
	deadline    time.Time
	hasDeadline bool
}

func (p *preflightRecorder) preflight(ctx context.Context, _ *manifest.Manifest, _ reconciler.Registry, _ reconciler.Options) (reconciler.Report, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls++
	p.deadline, p.hasDeadline = ctx.Deadline()

	return p.report, p.err
}

func TestBuildSystemFacts_Disabled_WiresNoopRecorderAndNoProducer(t *testing.T) {
	t.Parallel()

	components, err := buildSystemFacts(context.Background(), &Config{}, libLog.NewNop(), nil, nil, nil, nil, systemFactsDeps{})
	require.NoError(t, err)

	require.NotNil(t, components.recorder)
	assert.Nil(t, components.producer)
	assert.False(t, components.emitting)
}

func TestBuildSystemFacts_SingleTenant_GatesEmissionOnPreflightOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		report       reconciler.Report
		err          error
		wantEmitting bool
	}{
		{name: "outcome ok enables emission", report: reconciler.Report{Outcome: reconciler.OutcomeOK}, wantEmitting: true},
		{name: "outcome rejected keeps emission off", report: reconciler.Report{Outcome: reconciler.OutcomeRejected}, err: reconciler.ErrRejected},
		{name: "outcome unavailable keeps emission off", report: reconciler.Report{Outcome: reconciler.OutcomeUnavailable}, err: reconciler.ErrUnavailable},
		{name: "error without outcome keeps emission off", err: errors.New("registry exploded")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := enabledSystemFactsConfig()
			cfg.SystemFactsURL = systemFactsLoopbackURL
			seam := &preflightRecorder{report: tt.report, err: tt.err}

			components, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
				systemFactsDeps{authClient: systemFactsAuthClient(), preflight: seam.preflight})
			require.NoError(t, err)

			require.NotNil(t, components.recorder)
			require.NotNil(t, components.producer, "the producer is kept so a later unit can drain the backlog")
			assert.Equal(t, tt.wantEmitting, components.emitting)
			assert.Equal(t, 1, seam.calls)
		})
	}
}

func TestBuildSystemFacts_PreflightContextCarriesTheConfiguredTimeout(t *testing.T) {
	t.Parallel()

	cfg := enabledSystemFactsConfig()
	cfg.SystemFactsURL = systemFactsLoopbackURL
	cfg.SystemFactsPreflightTimeoutMs = 2500
	seam := &preflightRecorder{report: reconciler.Report{Outcome: reconciler.OutcomeOK}}

	parent, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	_, err := buildSystemFacts(parent, &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
		systemFactsDeps{authClient: systemFactsAuthClient(), preflight: seam.preflight})
	require.NoError(t, err)

	require.True(t, seam.hasDeadline)

	parentDeadline, _ := parent.Deadline()
	assert.True(t, seam.deadline.Before(parentDeadline), "the preflight deadline must be tighter than the caller's")
	assert.LessOrEqual(t, time.Until(seam.deadline), 2500*time.Millisecond)
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
			seam := &preflightRecorder{report: reconciler.Report{Outcome: reconciler.OutcomeOK}}

			_, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
				systemFactsDeps{authClient: tt.client, preflight: seam.preflight})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "PLUGIN_AUTH_HOST")
			assert.Equal(t, 0, seam.calls, "no preflight runs without a credential")
		})
	}
}

func TestBuildSystemFacts_MultiTenant_RequiresTheTenantPoolManager(t *testing.T) {
	t.Parallel()

	cfg := enabledSystemFactsConfig()
	cfg.SystemFactsURL = systemFactsLoopbackURL
	cfg.MultiTenantEnabled = true
	seam := &preflightRecorder{report: reconciler.Report{Outcome: reconciler.OutcomeOK}}

	_, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), nil,
		systemFactsDeps{authClient: systemFactsAuthClient(), tenantServiceName: "ledger", preflight: seam.preflight})
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

func TestBuildSystemFacts_MultiTenant_BuildsThePerTenantProducer(t *testing.T) {
	t.Parallel()

	cfg := enabledSystemFactsConfig()
	cfg.SystemFactsURL = systemFactsLoopbackURL
	cfg.MultiTenantEnabled = true
	cfg.EnvName = "staging"
	cfg.SystemFactsM2MClientID = ""
	cfg.SystemFactsM2MClientSecret = ""
	seam := &preflightRecorder{report: reconciler.Report{Outcome: reconciler.OutcomeOK}}
	secrets := &secretIDRecorder{}
	tenantClient, pgManager := systemFactsTenantPools(t)

	components, err := buildSystemFacts(context.Background(), &cfg, libLog.NewNop(), nil, nil, offlineTransactionClient(t), pgManager,
		systemFactsDeps{
			authClient:        systemFactsAuthClient(),
			tenantServiceName: "ledger",
			tenantClient:      tenantClient,
			newAWSSecretsClient: func(context.Context) (secretsmanager.SecretsManagerClient, error) {
				return secrets, nil
			},
			preflight: seam.preflight,
		})
	require.NoError(t, err)

	require.NotNil(t, components.producer)
	assert.True(t, components.emitting)
	assert.Equal(t, 1, seam.calls)
	assert.Empty(t, secrets.read(), "no tenant secret is read until a tenant's facts are delivered")
}

// countingCredentialProvider answers a fixed pair and counts reads and
// invalidations, standing in for the tracer seam's cached provider.
type countingCredentialProvider struct {
	mu           sync.Mutex
	reads        int
	invalidated  []string
	clientSecret string
}

func (p *countingCredentialProvider) Credentials(context.Context, string) (tracerclient.Credentials, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.reads++

	return tracerclient.Credentials{ClientID: "ledger-tenant", ClientSecret: p.clientSecret}, nil
}

func (p *countingCredentialProvider) Invalidate(tenantID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.invalidated = append(p.invalidated, tenantID)
}

func TestExpiringTenantCredentials_InvalidatesOnlyAfterTheTTL(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.October, 9, 12, 0, 0, 0, time.UTC)
	now := start
	provider := &countingCredentialProvider{clientSecret: "s3cret"}
	creds := &expiringTenantCredentials{provider: provider, ttl: 5 * time.Minute, now: func() time.Time { return now }, readAt: make(map[string]time.Time)}

	for range 2 {
		_, _, err := creds.lookup(context.Background(), seamTenantID)
		require.NoError(t, err)
	}

	assert.Empty(t, provider.invalidated, "inside the TTL the cached pair is served")

	now = start.Add(5 * time.Minute)

	_, _, err := creds.lookup(context.Background(), seamTenantID)
	require.NoError(t, err)
	assert.Equal(t, []string{seamTenantID}, provider.invalidated, "at the TTL the entry is dropped so the next read reaches the secret store")

	_, _, err = creds.lookup(context.Background(), seamTenantID)
	require.NoError(t, err)
	assert.Len(t, provider.invalidated, 1, "the refilled entry starts a new TTL window")
}
