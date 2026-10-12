// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libOutboxPostgres "github.com/LerianStudio/lib-commons/v7/commons/outbox/postgres"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/LerianStudio/lib-commons/v7/commons/secretsmanager"
	tmclient "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/client"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libSystemFacts "github.com/LerianStudio/lib-system-facts"
	"github.com/LerianStudio/lib-system-facts/emitter"
	"github.com/LerianStudio/lib-system-facts/manifest"
	"github.com/LerianStudio/lib-system-facts/registry"
	"github.com/LerianStudio/lib-system-facts/transport/httpsync"
	"github.com/LerianStudio/lib-system-facts/transport/httpsync/accessmanager"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	ledgerembed "github.com/LerianStudio/midaz/v4/components/ledger"
	systemfactsadapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/systemfacts"
	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
)

const (
	// systemFactsContractsDir holds the embedded manifest and schemas under
	// ledgerembed.SystemFactsContracts.
	systemFactsContractsDir = "contracts/system-facts"
	// systemFactsM2MTargetService is the target-service segment of the ledger's
	// M2M secret path for the producer credential.
	systemFactsM2MTargetService = "system-facts"
	// systemFactsPlatformTenant is the platform tenant lib-commons appends to the
	// dispatch scopes; the ledger records nothing under it.
	systemFactsPlatformTenant = "default"
	// systemFactsCredentialTTL bounds how long a tenant credential is served from
	// memory, so a rotated secret reaches the token grant without a restart.
	systemFactsCredentialTTL = 5 * time.Minute
	// systemFactsSourceServiceCheckWarn and systemFactsSourceServiceCheckRequire
	// are the SYSTEM_FACTS_SOURCE_SERVICE_CHECK values.
	systemFactsSourceServiceCheckWarn    = "warn"
	systemFactsSourceServiceCheckRequire = "require"

	// An explicit SYSTEM_FACTS_RECONCILE_ATTEMPT_TIMEOUT_MS must sit within
	// these bounds: below, a healthy registry reads as unavailable on every
	// attempt; above, a hung call pins the reconcile loop for too long.
	minSystemFactsReconcileAttemptTimeoutMs = 5000
	maxSystemFactsReconcileAttemptTimeoutMs = 600000
)

// errSystemFactsAuthClientNotAuthorizing refuses the producer on an Access
// Manager client that cannot mint: every token grant would fail.
var errSystemFactsAuthClientNotAuthorizing = errors.New("system facts producer: PLUGIN_AUTH_ENABLED=true requires an enabled Access Manager client with an address (PLUGIN_AUTH_HOST)")

// systemFactsComponents is what the ledger wires from the System Facts
// producer: the recorder the completion Store records through, the producer
// whose forwarder the Launcher runs, and the gate that holds that forwarder
// until the contract is ACTIVE.
type systemFactsComponents struct {
	// recorder is always non-nil: around the producer's recording emitter when
	// enabled, around the kit's no-op emitter otherwise.
	recorder *systemfactsadapter.Recorder
	// emitter is what recorder wraps: with the flag on it records from the
	// first commit, whatever the registry answers later.
	emitter emitter.Emitter
	// producer is nil with SYSTEM_FACTS_ENABLED=false.
	producer *libSystemFacts.Producer
	// gate reconciles the contract at boot and releases the forwarder on
	// OutcomeOK; nil with SYSTEM_FACTS_ENABLED=false, when no registry client
	// exists and nothing is called.
	gate *systemFactsContractGate
}

// reconcileContract is the forwarder app's reconcile seam: nil when no gate
// was built, which the app fails closed on.
func (c *systemFactsComponents) reconcileContract() func(context.Context) bool {
	if c == nil || c.gate == nil {
		return nil
	}

	return c.gate.reconcile
}

// systemFactsDeps carries the collaborators the producer is built from. A nil
// newAWSSecretsClient means the default AWS credential chain.
type systemFactsDeps struct {
	authClient          *middleware.AuthClient
	tenantServiceName   string
	tenantClient        *tmclient.Client
	metricsFactory      *metrics.MetricsFactory
	newAWSSecretsClient awsSecretsClientFactory
}

// buildSystemFacts wires the producer. SYSTEM_FACTS_ENABLED=false wires only
// the kit's no-op emitter; otherwise the embedded contracts, token sources,
// transport, producer, registry client and the contract gate. The recorder
// wraps the recording emitter unconditionally: the gate is the forwarder,
// never the emitter, so no fact is lost while the reconcile is pending.
func buildSystemFacts(
	ctx context.Context,
	cfg *Config,
	logger libLog.Logger,
	tracer trace.Tracer,
	meterProvider metric.MeterProvider,
	txnPG *libPostgres.Client,
	txnPGManager *tmpostgres.Manager,
	deps systemFactsDeps,
) (*systemFactsComponents, error) {
	if !cfg.SystemFactsEnabled {
		facts, err := libSystemFacts.New(libSystemFacts.Config{Enabled: false})
		if err != nil {
			return nil, fmt.Errorf("build the no-op emitter: %w", err)
		}

		recorder, err := systemfactsadapter.NewRecorder(facts, logger)
		if err != nil {
			return nil, err
		}

		logger.Log(ctx, libLog.LevelInfo, "System Facts emission disabled (SYSTEM_FACTS_ENABLED=false)")

		return &systemFactsComponents{recorder: recorder, emitter: facts}, nil
	}

	attemptTimeout, err := systemFactsReconcileAttemptTimeout(cfg)
	if err != nil {
		return nil, err
	}

	m, catalog, err := loadSystemFactsCatalog()
	if err != nil {
		return nil, err
	}

	delivery, registration, err := buildSystemFactsTokenSource(ctx, cfg, deps)
	if err != nil {
		return nil, err
	}

	transport, client, err := buildSystemFactsClients(cfg, delivery, registration)
	if err != nil {
		return nil, err
	}

	tenancy, err := buildSystemFactsTenancy(cfg, txnPG, txnPGManager, deps)
	if err != nil {
		return nil, err
	}

	producer, err := libSystemFacts.NewProducer(libSystemFacts.ProducerConfig{
		Catalog:       catalog,
		Postgres:      txnPG,
		Tenancy:       tenancy,
		Transport:     transport,
		Logger:        logger,
		Tracer:        tracer,
		MeterProvider: meterProvider,
		Forwarder:     systemFactsForwarderTuning(cfg),
	})
	if err != nil {
		return nil, fmt.Errorf("build the System Facts producer: %w", err)
	}

	facts := producer.Emitter()

	recorder, err := systemfactsadapter.NewRecorder(facts, logger)
	if err != nil {
		return nil, err
	}

	gate := newSystemFactsContractGate(&kitSystemFactsReconciler{manifest: m, registry: client}, attemptTimeout, logger, deps.metricsFactory)

	logger.Log(ctx, libLog.LevelInfo, "System Facts emission enabled: facts are recorded from the first commit; delivery waits for the boot reconcile",
		libLog.String("producer", catalog.Producer()),
		libLog.String("namespace", catalog.Namespace()))

	return &systemFactsComponents{recorder: recorder, emitter: facts, producer: producer, gate: gate}, nil
}

// buildSystemFactsClients builds the delivery transport over delivery and the
// registry client over registration, admitting plain http only with
// SYSTEM_FACTS_INSECURE_HTTP.
func buildSystemFactsClients(cfg *Config, delivery, registration httpsync.TokenSource) (*httpsync.Client, *registry.Client, error) {
	var (
		transportOpts []httpsync.Option
		registryOpts  []registry.Option
	)

	if cfg.SystemFactsInsecureHTTP {
		transportOpts = append(transportOpts, httpsync.WithInsecureHTTP())
		registryOpts = append(registryOpts, registry.WithInsecureHTTP())
	}

	transport, err := httpsync.New(cfg.SystemFactsURL, delivery, transportOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("build the System Facts transport: %w", err)
	}

	client, err := registry.New(cfg.SystemFactsURL, registration, registryOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("build the System Facts registry client: %w", err)
	}

	return transport, client, nil
}

// systemFactsTelemetry is the tracer and meter provider the kit instruments
// with, each handed over only when telemetry built it: a nil SDK provider
// boxed in the interface would be dereferenced by the kit.
func systemFactsTelemetry(telemetry *libOpentelemetry.Telemetry) (trace.Tracer, metric.MeterProvider) {
	if telemetry == nil {
		return nil, nil
	}

	var tracer trace.Tracer
	if telemetry.TracerProvider != nil {
		tracer = telemetry.TracerProvider.Tracer("midaz.ledger.system_facts")
	}

	var meterProvider metric.MeterProvider
	if telemetry.MeterProvider != nil {
		meterProvider = telemetry.MeterProvider
	}

	return tracer, meterProvider
}

// loadSystemFactsCatalog compiles the embedded contracts the way the offline
// contract check and the adapter tests read them.
func loadSystemFactsCatalog() (*manifest.Manifest, *manifest.Catalog, error) {
	dir, err := fs.Sub(ledgerembed.SystemFactsContracts, systemFactsContractsDir)
	if err != nil {
		return nil, nil, fmt.Errorf("open the embedded System Facts contracts: %w", err)
	}

	m, err := manifest.Load(dir, ".")
	if err != nil {
		return nil, nil, fmt.Errorf("load the embedded System Facts manifest: %w", err)
	}

	catalog, err := m.Compile()
	if err != nil {
		return nil, nil, fmt.Errorf("compile the embedded System Facts manifest: %w", err)
	}

	return m, catalog, nil
}

// buildSystemFactsTokenSource selects the credentials of the two calls the
// producer makes and answers (delivery, registration). The static
// SYSTEM_FACTS_M2M_* pair is the service credential: single-tenant it
// authenticates both delivery and the contract registration; multi-tenant it
// is the tenantless credential that registers the contract (issued with the
// same source as every tenant credential, so the owner is the same
// svc:<sourceService>), while each tenant's own secret-store credential
// authenticates that tenant's delivery. The static pair never authenticates
// a tenant's delivery: a shared token would merge every tenant's facts.
func buildSystemFactsTokenSource(ctx context.Context, cfg *Config, deps systemFactsDeps) (httpsync.TokenSource, httpsync.TokenSource, error) {
	if deps.authClient == nil || !deps.authClient.Enabled || strings.TrimSpace(deps.authClient.Address) == "" {
		return nil, nil, errSystemFactsAuthClientNotAuthorizing
	}

	check, err := systemFactsSourceServiceCheck(cfg)
	if err != nil {
		return nil, nil, err
	}

	opts := []accessmanager.Option{accessmanager.WithSourceServiceCheck(check)}
	if cfg.SystemFactsInsecureHTTP {
		opts = append(opts, accessmanager.WithInsecureHTTP())
	}

	service, err := accessmanager.NewTokenSource(deps.authClient,
		strings.TrimSpace(cfg.SystemFactsM2MClientID), strings.TrimSpace(cfg.SystemFactsM2MClientSecret), opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("build the System Facts service token source: %w", err)
	}

	if !cfg.MultiTenantEnabled {
		return service, service, nil
	}

	applicationName := strings.TrimSpace(deps.tenantServiceName)
	if applicationName == "" {
		return nil, nil, errors.New("system facts tenant credentials require APPLICATION_NAME, the ledger's tenant-manager service name")
	}

	reader, err := buildM2MSecretsReader(ctx, cfg, deps.newAWSSecretsClient)
	if err != nil {
		return nil, nil, err
	}

	creds, err := systemFactsTenantCredentials(reader, strings.TrimSpace(cfg.EnvName), applicationName)
	if err != nil {
		return nil, nil, err
	}

	tenants, err := accessmanager.NewTenantTokenSource(deps.authClient, creds, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("build the System Facts tenant token source: %w", err)
	}

	return tenants, service, nil
}

// buildSystemFactsTenancy is the zero (single-tenant) tenancy, or the
// database-per-tenant tenancy over the transaction tenant pools.
func buildSystemFactsTenancy(cfg *Config, txnPG *libPostgres.Client, txnPGManager *tmpostgres.Manager, deps systemFactsDeps) (libSystemFacts.TenancyConfig, error) {
	if !cfg.MultiTenantEnabled {
		return libSystemFacts.TenancyConfig{}, nil
	}

	if txnPGManager == nil {
		return libSystemFacts.TenancyConfig{}, errors.New("system facts producer: multi-tenant mode requires the transaction tenant pool manager")
	}

	if deps.tenantClient == nil {
		return libSystemFacts.TenancyConfig{}, errors.New("system facts producer: multi-tenant mode requires the tenant-manager client")
	}

	pools, err := libOutboxPostgres.NewManagerPoolResolver(txnPGManager, deps.tenantClient, txnPG,
		strings.TrimSpace(deps.tenantServiceName), systemFactsPlatformTenant)
	if err != nil {
		return libSystemFacts.TenancyConfig{}, fmt.Errorf("build the System Facts tenant pool resolver: %w", err)
	}

	return libSystemFacts.TenancyConfig{MultiTenantEnabled: true, Pools: pools}, nil
}

// incrementSystemFactsCounter adds one to m under labels. A nil factory records
// nothing; an emit failure is logged at Debug and never fails the caller.
func incrementSystemFactsCounter(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, m metrics.Metric, labels map[string]string) {
	if factory == nil {
		return
	}

	counter, err := factory.Counter(m)
	if err == nil {
		err = counter.WithLabels(labels).Add(ctx, 1)
	}

	if err != nil {
		logger.Log(ctx, libLog.LevelDebug, "Failed to emit a System Facts metric", libLog.String("metric", m.Name), libLog.Err(err))
	}
}

// systemFactsReconcileAttemptTimeout resolves
// SYSTEM_FACTS_RECONCILE_ATTEMPT_TIMEOUT_MS: unset is
// defaultSystemFactsReconcileAttemptTimeout, and a value must sit within
// minSystemFactsReconcileAttemptTimeoutMs..maxSystemFactsReconcileAttemptTimeoutMs.
func systemFactsReconcileAttemptTimeout(cfg *Config) (time.Duration, error) {
	ms := cfg.SystemFactsReconcileAttemptTimeoutMs
	if ms == 0 {
		return defaultSystemFactsReconcileAttemptTimeout, nil
	}

	if ms < minSystemFactsReconcileAttemptTimeoutMs || ms > maxSystemFactsReconcileAttemptTimeoutMs {
		return 0, fmt.Errorf("invalid SYSTEM_FACTS_RECONCILE_ATTEMPT_TIMEOUT_MS %d: expected %d..%d",
			ms, minSystemFactsReconcileAttemptTimeoutMs, maxSystemFactsReconcileAttemptTimeoutMs)
	}

	return time.Duration(ms) * time.Millisecond, nil
}

// systemFactsTenantCredentials is the kit's per-tenant credential lookup over
// the M2M secret at tenants/{env}/{tenant}/{applicationName}/m2m/system-facts/credentials,
// cached per tenant for systemFactsCredentialTTL with stale-while-revalidate.
func systemFactsTenantCredentials(reader secretsmanager.SecretsManagerClient, env, applicationName string) (accessmanager.TenantCredentials, error) {
	fetch := func(ctx context.Context, tenantOrgID string) (*secretsmanager.M2MCredentials, error) {
		return secretsmanager.GetM2MCredentials(ctx, reader, env, tenantOrgID, applicationName, systemFactsM2MTargetService)
	}

	provider, err := tracerclient.NewTenantCredentials(fetch, tracerclient.WithCredentialTTL(systemFactsCredentialTTL))
	if err != nil {
		return nil, fmt.Errorf("build the System Facts tenant credential provider: %w", err)
	}

	return func(ctx context.Context, tenantID string) (string, string, error) {
		creds, err := provider.Credentials(ctx, tenantID)
		if err != nil {
			return "", "", err
		}

		return creds.ClientID, creds.ClientSecret, nil
	}, nil
}

// systemFactsForwarderTuning maps the SYSTEM_FACTS_FORWARDER_* and retention
// knobs onto the kit's tuning; zero is left for the kit to default.
func systemFactsForwarderTuning(cfg *Config) libSystemFacts.ForwarderTuning {
	return libSystemFacts.ForwarderTuning{
		BatchSize:          cfg.SystemFactsForwarderBatchSize,
		DispatchInterval:   time.Duration(cfg.SystemFactsForwarderDispatchIntervalMs) * time.Millisecond,
		ProcessingTimeout:  time.Duration(cfg.SystemFactsForwarderProcessingTimeoutMs) * time.Millisecond,
		RetentionPublished: time.Duration(cfg.SystemFactsRetentionPublishedHours) * time.Hour,
		RetentionInvalid:   time.Duration(cfg.SystemFactsRetentionInvalidHours) * time.Hour,
	}
}

// systemFactsSourceServiceCheck resolves SYSTEM_FACTS_SOURCE_SERVICE_CHECK:
// unset or warn logs a failing sourceService claim once and still delivers;
// require fails the grant so facts wait in the outbox. Nothing else is accepted.
func systemFactsSourceServiceCheck(cfg *Config) (accessmanager.SourceServiceCheck, error) {
	switch mode := strings.ToLower(strings.TrimSpace(cfg.SystemFactsSourceServiceCheck)); mode {
	case "", systemFactsSourceServiceCheckWarn:
		return accessmanager.SourceServiceCheckWarn, nil
	case systemFactsSourceServiceCheckRequire:
		return accessmanager.SourceServiceCheckRequire, nil
	default:
		return accessmanager.SourceServiceCheckWarn, fmt.Errorf("invalid SYSTEM_FACTS_SOURCE_SERVICE_CHECK %q: expected warn or require", mode)
	}
}
