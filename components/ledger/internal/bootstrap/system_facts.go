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
	"sync"
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
	"github.com/LerianStudio/lib-system-facts/reconciler"
	"github.com/LerianStudio/lib-system-facts/registry"
	"github.com/LerianStudio/lib-system-facts/transport/httpsync"
	"github.com/LerianStudio/lib-system-facts/transport/httpsync/accessmanager"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	ledgerembed "github.com/LerianStudio/midaz/v4/components/ledger"
	systemfactsadapter "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/systemfacts"
	tracerclient "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

const (
	// systemFactsContractsDir is where the embedded System Facts manifest and
	// schemas live under ledgerembed.SystemFactsContracts.
	systemFactsContractsDir = "contracts/system-facts"
	// systemFactsM2MTargetService is the target-service segment of the ledger's
	// M2M secret path for the System Facts producer credential.
	systemFactsM2MTargetService = "system-facts"
	// systemFactsCredentialTTL bounds how long a tenant's credential is served
	// from memory: the kit asks for it on every token call, so it must be cheap,
	// and a rotated secret must be picked up without a restart.
	systemFactsCredentialTTL = 5 * time.Minute

	// defaultSystemFactsPreflightTimeout bounds the boot-time contract
	// preflight when SYSTEM_FACTS_PREFLIGHT_TIMEOUT_MS is unset.
	defaultSystemFactsPreflightTimeout = 10 * time.Second
	// minSystemFactsPreflightTimeoutMs and maxSystemFactsPreflightTimeoutMs
	// bound an explicit SYSTEM_FACTS_PREFLIGHT_TIMEOUT_MS: below the floor a
	// healthy registry reads as unavailable, above the ceiling a dead one
	// holds boot.
	minSystemFactsPreflightTimeoutMs = 1000
	maxSystemFactsPreflightTimeoutMs = 60000
)

// errSystemFactsAuthClientNotAuthorizing refuses the producer on an Access
// Manager client that is nil, disabled or has no address: every token grant
// would fail, so no fact would ever be delivered.
var errSystemFactsAuthClientNotAuthorizing = errors.New("system facts producer: PLUGIN_AUTH_ENABLED=true requires an enabled Access Manager client with an address (PLUGIN_AUTH_HOST)")

// systemFactsPreflightTotal counts boot-time contract preflights by outcome. A
// non-OK outcome means the pod runs with emission off, so any rate of it needs
// an operator: the reconciler Job did not run, or the registry moved.
var systemFactsPreflightTotal = metrics.Metric{
	Name:        "system_facts_preflight_total",
	Unit:        "1",
	Description: "System Facts contract preflights at boot, by outcome (OK, Rejected, Unavailable, Invalid, error).",
}

// systemFactsPreflightFunc is the read-only contract preflight the producer
// gates emission on; reconciler.Preflight in production.
type systemFactsPreflightFunc func(ctx context.Context, m *manifest.Manifest, c reconciler.Registry, opts reconciler.Options) (reconciler.Report, error)

// systemFactsComponents is what the ledger wires from the System Facts
// producer: the recorder the completion Store records through, and the
// producer whose forwarder the Launcher runs and the readiness probe reads.
type systemFactsComponents struct {
	// recorder is always non-nil: it wraps the validating emitter when the
	// preflight answered OK and the kit's no-op emitter otherwise.
	recorder *systemfactsadapter.Recorder
	// producer is nil when SYSTEM_FACTS_ENABLED=false. It is kept even when
	// emission is off so the forwarder can still drain a backlog.
	producer *libSystemFacts.Producer
	// emitting is true only when the producer is enabled and the preflight
	// outcome was OK.
	emitting bool
}

// systemFactsDeps carries the collaborators the producer is built from.
// authClient is the ledger's Access Manager client, which mints the producer
// token; tenantServiceName is the application segment the tenant-manager writes
// the ledger's M2M secrets under; tenantClient enumerates the tenants the
// forwarder drains; metricsFactory records the preflight counter;
// newAWSSecretsClient builds the AWS Secrets Manager client when that is the
// custody backend (from the default AWS credential chain when nil); preflight
// is the contract preflight (reconciler.Preflight when nil).
type systemFactsDeps struct {
	authClient          *middleware.AuthClient
	tenantServiceName   string
	tenantClient        *tmclient.Client
	metricsFactory      *metrics.MetricsFactory
	newAWSSecretsClient awsSecretsClientFactory
	preflight           systemFactsPreflightFunc
}

// buildSystemFacts wires the System Facts producer. With SYSTEM_FACTS_ENABLED
// false it wires the kit's no-op emitter and builds nothing else. Otherwise it
// compiles the embedded contracts, builds the token source (the static
// SYSTEM_FACTS_M2M_* pair single-tenant, each tenant's own secret-store
// credential multi-tenant), the transport, the producer and the registry
// client, then runs the read-only contract preflight: only an OK outcome turns
// emission on. Any other outcome, or an error, keeps the no-op emitter and the
// producer, logs a Warn and counts it; it never refuses boot, because the
// reconciler Job already gates the rollout.
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

		return &systemFactsComponents{recorder: recorder}, nil
	}

	preflightTimeout, err := systemFactsPreflightTimeout(cfg)
	if err != nil {
		return nil, err
	}

	m, catalog, err := loadSystemFactsCatalog()
	if err != nil {
		return nil, err
	}

	identity, tokens, err := buildSystemFactsTokenSource(ctx, cfg, deps)
	if err != nil {
		return nil, err
	}

	var httpsyncOpts []httpsync.Option
	if cfg.SystemFactsInsecureHTTP {
		httpsyncOpts = append(httpsyncOpts, httpsync.WithInsecureHTTP())
	}

	transport, err := httpsync.New(cfg.SystemFactsURL, tokens, httpsyncOpts...)
	if err != nil {
		return nil, fmt.Errorf("build the System Facts transport: %w", err)
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
		Forwarder: libSystemFacts.ForwarderTuning{
			RetentionInvalid: time.Duration(cfg.SystemFactsRetentionInvalidHours) * time.Hour,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("build the System Facts producer: %w", err)
	}

	var registryOpts []registry.Option
	if cfg.SystemFactsInsecureHTTP {
		registryOpts = append(registryOpts, registry.WithInsecureHTTP())
	}

	client, err := registry.New(cfg.SystemFactsURL, tokens, registryOpts...)
	if err != nil {
		return nil, fmt.Errorf("build the System Facts registry client: %w", err)
	}

	facts, emitting := gateSystemFactsEmission(ctx, logger, deps, producer, m, catalog, identity, client, preflightTimeout)

	recorder, err := systemfactsadapter.NewRecorder(facts, logger)
	if err != nil {
		return nil, err
	}

	return &systemFactsComponents{recorder: recorder, producer: producer, emitting: emitting}, nil
}

// systemFactsTelemetry is the tracer and meter provider the kit instruments
// the forwarder and the emitter with. Each is handed over only when telemetry
// built it: a nil SDK provider boxed in the interface would be a non-nil value
// the kit then dereferences.
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

// systemFactsTokenSource is what the transport and the registry client
// authenticate with; the Access Manager sources also invalidate on a 401,
// which the transport discovers by type.
type systemFactsTokenSource interface {
	Token(ctx context.Context) (string, error)
}

// buildSystemFactsTokenSource selects the credential every call carries: the
// static SYSTEM_FACTS_M2M_CLIENT_ID / SYSTEM_FACTS_M2M_CLIENT_SECRET pair in
// single-tenant mode, and each tenant's own secret-store credential in
// multi-tenant mode (the static pair is never a fallback there, because its
// token would ingest every tenant's facts into one tenant). The source-service
// check stays at the kit's Warn default until every credential is rotated.
func buildSystemFactsTokenSource(ctx context.Context, cfg *Config, deps systemFactsDeps) (tracerSeamIdentity, systemFactsTokenSource, error) {
	if deps.authClient == nil || !deps.authClient.Enabled || strings.TrimSpace(deps.authClient.Address) == "" {
		return "", nil, errSystemFactsAuthClientNotAuthorizing
	}

	var opts []accessmanager.Option
	if cfg.SystemFactsInsecureHTTP {
		opts = append(opts, accessmanager.WithInsecureHTTP())
	}

	if !cfg.MultiTenantEnabled {
		tokens, err := accessmanager.NewTokenSource(deps.authClient,
			strings.TrimSpace(cfg.SystemFactsM2MClientID), strings.TrimSpace(cfg.SystemFactsM2MClientSecret), opts...)
		if err != nil {
			return "", nil, fmt.Errorf("build the System Facts token source: %w", err)
		}

		return seamIdentityStaticToken, tokens, nil
	}

	applicationName := strings.TrimSpace(deps.tenantServiceName)
	if applicationName == "" {
		return "", nil, errors.New("system facts tenant credentials require APPLICATION_NAME, the ledger's tenant-manager service name")
	}

	reader, err := buildM2MSecretsReader(ctx, cfg, deps.newAWSSecretsClient)
	if err != nil {
		return "", nil, err
	}

	creds, err := systemFactsTenantCredentials(reader, strings.TrimSpace(cfg.EnvName), applicationName)
	if err != nil {
		return "", nil, err
	}

	tokens, err := accessmanager.NewTenantTokenSource(deps.authClient, creds, opts...)
	if err != nil {
		return "", nil, fmt.Errorf("build the System Facts tenant token source: %w", err)
	}

	return seamIdentityTenantToken, tokens, nil
}

// buildSystemFactsTenancy is the zero (single-tenant) tenancy, or the
// database-per-tenant tenancy over the transaction tenant pools. lib-commons
// requires a platform default tenant it resolves from the root pool and appends
// to the tenant list; the ledger serves no such tenant, so the streaming
// default name is passed: no request ever carries it, so nothing is recorded
// under it, and the dispatcher skips a root database without the kit table.
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
		strings.TrimSpace(deps.tenantServiceName), pkgStreaming.DefaultTenantID)
	if err != nil {
		return libSystemFacts.TenancyConfig{}, fmt.Errorf("build the System Facts tenant pool resolver: %w", err)
	}

	return libSystemFacts.TenancyConfig{MultiTenantEnabled: true, Pools: pools}, nil
}

// gateSystemFactsEmission runs the read-only contract preflight under its
// timeout and returns the emitter business code records through: the
// producer's validating emitter on OK, the kit's no-op emitter otherwise. The
// outcome is logged and counted either way.
func gateSystemFactsEmission(
	ctx context.Context,
	logger libLog.Logger,
	deps systemFactsDeps,
	producer *libSystemFacts.Producer,
	m *manifest.Manifest,
	catalog *manifest.Catalog,
	identity tracerSeamIdentity,
	client reconciler.Registry,
	timeout time.Duration,
) (emitter.Emitter, bool) {
	preflight := deps.preflight
	if preflight == nil {
		preflight = reconciler.Preflight
	}

	preflightCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	report, err := preflight(preflightCtx, m, client, reconciler.Options{})

	outcome := string(report.Outcome)
	if outcome == "" {
		outcome = "error"
	}

	recordSystemFactsPreflight(ctx, deps.metricsFactory, logger, outcome)

	if err == nil && report.Outcome == reconciler.OutcomeOK {
		logger.Log(ctx, libLog.LevelInfo, "System Facts emission enabled: contract preflight OK",
			libLog.String("producer", catalog.Producer()),
			libLog.String("namespace", catalog.Namespace()),
			libLog.String("identity", string(identity)))

		return producer.Emitter(), true
	}

	logger.Log(ctx, libLog.LevelWarn, "System Facts emission kept off: contract preflight did not answer OK; the forwarder still drains the backlog",
		libLog.String("outcome", outcome),
		libLog.String("identity", string(identity)),
		libLog.Err(err))

	facts, err := libSystemFacts.New(libSystemFacts.Config{Enabled: false})
	if err != nil {
		// Config{Enabled: false} reads no field, so this cannot fail; the
		// no-op emitter is still the only safe answer.
		logger.Log(ctx, libLog.LevelError, "System Facts no-op emitter could not be built", libLog.Err(err))
	}

	return facts, false
}

// recordSystemFactsPreflight increments the preflight counter for outcome.
func recordSystemFactsPreflight(ctx context.Context, factory *metrics.MetricsFactory, logger libLog.Logger, outcome string) {
	incrementSystemFactsCounter(ctx, factory, logger, systemFactsPreflightTotal, map[string]string{"outcome": outcome})
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

// systemFactsPreflightTimeout resolves SYSTEM_FACTS_PREFLIGHT_TIMEOUT_MS: unset
// is defaultSystemFactsPreflightTimeout, and a value must sit within
// minSystemFactsPreflightTimeoutMs..maxSystemFactsPreflightTimeoutMs.
func systemFactsPreflightTimeout(cfg *Config) (time.Duration, error) {
	ms := cfg.SystemFactsPreflightTimeoutMs
	if ms == 0 {
		return defaultSystemFactsPreflightTimeout, nil
	}

	if ms < minSystemFactsPreflightTimeoutMs || ms > maxSystemFactsPreflightTimeoutMs {
		return 0, fmt.Errorf("invalid SYSTEM_FACTS_PREFLIGHT_TIMEOUT_MS %d: expected %d..%d",
			ms, minSystemFactsPreflightTimeoutMs, maxSystemFactsPreflightTimeoutMs)
	}

	return time.Duration(ms) * time.Millisecond, nil
}

// systemFactsTenantCredentials is the kit's per-tenant credential lookup over
// the M2M secret the tenant-manager writes at
// tenants/{env}/{tenant}/{applicationName}/m2m/system-facts/credentials. The
// tracer seam's credential provider caches each tenant's pair, shares concurrent
// reads and remembers a missing or malformed secret briefly; the kit asks for
// the pair on every token call, so that cache is what keeps the secret store
// off the delivery path. An entry older than systemFactsCredentialTTL is
// dropped before the next read, which is how a rotated secret reaches the
// Access Manager grant without a restart.
func systemFactsTenantCredentials(reader secretsmanager.SecretsManagerClient, env, applicationName string) (accessmanager.TenantCredentials, error) {
	fetch := func(ctx context.Context, tenantOrgID string) (*secretsmanager.M2MCredentials, error) {
		return secretsmanager.GetM2MCredentials(ctx, reader, env, tenantOrgID, applicationName, systemFactsM2MTargetService)
	}

	provider, err := tracerclient.NewTenantCredentials(fetch)
	if err != nil {
		return nil, fmt.Errorf("build the System Facts tenant credential provider: %w", err)
	}

	expiring := &expiringTenantCredentials{provider: provider, ttl: systemFactsCredentialTTL, now: time.Now, readAt: make(map[string]time.Time)}

	return expiring.lookup, nil
}

// expiringTenantCredentials bounds how long a cached credential is served:
// it drops the provider's entry for a tenant once ttl has passed since the
// read that filled it, so the next lookup reads the secret store again.
type expiringTenantCredentials struct {
	provider tracerclient.CredentialProvider
	ttl      time.Duration
	now      func() time.Time

	mu     sync.Mutex
	readAt map[string]time.Time
}

func (e *expiringTenantCredentials) lookup(ctx context.Context, tenantID string) (string, string, error) {
	if strings.TrimSpace(tenantID) == "" {
		return "", "", errors.New("system facts tenant credential requires a tenant")
	}

	e.expire(tenantID)

	creds, err := e.provider.Credentials(ctx, tenantID)
	if err != nil {
		return "", "", err
	}

	e.mu.Lock()
	if _, ok := e.readAt[tenantID]; !ok {
		e.readAt[tenantID] = e.now()
	}
	e.mu.Unlock()

	return creds.ClientID, creds.ClientSecret, nil
}

// expire drops the tenant's cached credential once it is older than ttl.
func (e *expiringTenantCredentials) expire(tenantID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	at, ok := e.readAt[tenantID]
	if !ok || e.now().Sub(at) < e.ttl {
		return
	}

	e.provider.Invalidate(tenantID)
	delete(e.readAt, tenantID)
}
