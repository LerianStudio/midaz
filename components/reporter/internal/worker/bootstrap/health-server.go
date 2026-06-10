// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"net"
	"net/http"
	"time"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	mongoDB "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/readyz"
	libRedis "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/storage"

	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	"github.com/LerianStudio/lib-observability/log"
)

const (
	// healthServerReadTimeout is the maximum duration for reading the entire request.
	healthServerReadTimeout = 5 * time.Second

	// healthServerWriteTimeout is the maximum duration before timing out writes of the response.
	healthServerWriteTimeout = 5 * time.Second

	// healthServerIdleTimeout is the maximum duration an idle connection will remain open.
	healthServerIdleTimeout = 30 * time.Second

	// healthServerShutdownTimeout is the maximum duration to wait for the server to shutdown gracefully.
	healthServerShutdownTimeout = 5 * time.Second
)

// goNamedFunc is the function signature for launching a named goroutine with panic recovery.
type goNamedFunc func(logger log.Logger, name string, fn func())

// HealthServer is the Worker's bare-stdlib HTTP server hosting /health and
// /readyz. It runs alongside the RabbitMQ consumer goroutine.
//
// Endpoints:
//   - GET /health  → 200 with {"status":"alive"} (Gate 7 will add self-probe gating)
//   - GET /readyz → canonical readyz contract (see pkg/readyz)
//
// The legacy /ready alias is intentionally NOT registered. The contract
// path is exactly /readyz.
type HealthServer struct {
	server    *http.Server
	logger    log.Logger
	goNamedFn goNamedFunc
}

// HealthServerConfig bundles the dependencies required to assemble the
// Worker's /readyz Checker set.
//
// nil/zero values are tolerated — each pkg/readyz Checker reports its own
// missing-dependency state (e.g. nil MongoConnection → StatusDown). This
// keeps the constructor permissive across single-tenant and multi-tenant
// configurations.
type HealthServerConfig struct {
	// Port is the TCP port to bind, e.g. "4006".
	Port string

	// MongoConnection is the static MongoDB connection (nil in multi-tenant
	// mode where per-tenant probing is deferred to a future gate).
	MongoConnection *mongoDB.MongoConnection

	// RabbitMQConnection is the static RabbitMQ connection (nil in
	// multi-tenant mode).
	RabbitMQConnection *libRabbitmq.RabbitMQConnection

	// RedisConnection is the worker's Redis/Valkey connection (multi-tenant
	// per-tenant Redis client + tenant event-listener). nil if Redis is not
	// required (single-tenant mode no longer dials Redis).
	RedisConnection *libRedis.RedisConnection

	// StorageClient is the S3-compatible object storage adapter.
	StorageClient storage.ObjectStorage

	// StorageEndpoint is the configured storage endpoint URL — used only
	// for TLS posture detection.
	StorageEndpoint string

	// TenantManagerClient is the Tenant Manager client. nil when
	// MultiTenantEnabled=false. The TenantManagerChecker performs only a
	// nil-check.
	TenantManagerClient *tmclient.Client

	// MultiTenantEnabled mirrors MULTI_TENANT_ENABLED. It drives whether Redis
	// is treated as a required dependency: the Worker only constructs a Redis
	// connection when MULTI_TENANT_ENABLED=true (per-tenant Redis client +
	// tenant event-listener). In single-tenant mode a nil RedisConnection is the
	// expected steady state and the RedisChecker reports StatusSkipped instead
	// of StatusDown.
	MultiTenantEnabled bool

	// MongoURI is used only for TLS posture detection.
	MongoURI string

	// RabbitURI is used only for TLS posture detection.
	RabbitURI string

	// DrainState is the shared graceful-shutdown flag. When IsDraining()
	// returns true the /readyz handler short-circuits to a 503 "draining"
	// response.
	DrainState *readyz.DrainState

	// Version is emitted in every /readyz response.
	Version string

	// DeploymentMode echoes DEPLOYMENT_MODE (saas | byoc | local).
	DeploymentMode string

	// Logger is the structured logger used for server lifecycle events.
	Logger log.Logger

	// Metrics is the OTel emitter for the canonical readyz metric set.
	// nil is tolerated — the underlying handler no-ops emission rather
	// than panicking. Production bootstrap MUST construct one via
	// readyz.NewMetrics(meter).
	Metrics *readyz.Metrics

	// SelfProbeState gates the /health endpoint. Starts unhealthy; bootstrap
	// flips it to healthy via MarkHealthy() once readyz.RunSelfProbe
	// succeeds. nil is tolerated (treated as healthy) so existing tests can
	// mount the server without explicit wiring; production bootstrap MUST
	// set it.
	SelfProbeState *readyz.SelfProbeState
}

// BuildWorkerCheckers assembles the canonical six-dep checker list for the
// Worker. Exported so bootstrap can reuse the exact same list for both the
// startup self-probe (Gate 7) and the runtime /readyz handler — keeping the
// two surfaces in lock-step.
//
// Redis is conditionally required: the Worker constructs a Redis connection
// only when MULTI_TENANT_ENABLED=true (per-tenant Redis client + tenant
// event-listener). Redis now backs only SchemaCache/locks, no longer the
// retired fetcher reconciler. In single-tenant mode a nil RedisConnection is
// expected and the RedisChecker reports StatusSkipped rather than failing the
// aggregation.
func BuildWorkerCheckers(cfg HealthServerConfig) []readyz.Checker {
	redisRequired := cfg.MultiTenantEnabled

	return []readyz.Checker{
		readyz.NewMongoChecker(cfg.MongoConnection, cfg.MultiTenantEnabled),
		readyz.NewRabbitMQChecker(cfg.RabbitMQConnection, cfg.MultiTenantEnabled),
		readyz.NewRedisChecker(cfg.RedisConnection, redisRequired),
		readyz.NewStorageChecker(cfg.StorageClient, cfg.StorageEndpoint),
		readyz.NewTenantManagerChecker(cfg.MultiTenantEnabled, func() bool {
			return cfg.TenantManagerClient != nil
		}),
	}
}

// NewHealthServer creates a HealthServer wired with the canonical /readyz
// handler from pkg/readyz. The returned server is ready to Start() — call
// Shutdown() during graceful shutdown.
func NewHealthServer(cfg HealthServerConfig) *HealthServer {
	checkers := BuildWorkerCheckers(cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", newWorkerHealthHandler(cfg.SelfProbeState))
	mux.HandleFunc("/readyz", readyz.NewNetHTTPHandler(checkers, cfg.DrainState, cfg.Version, cfg.DeploymentMode, cfg.Metrics))

	return &HealthServer{
		server: &http.Server{
			Addr:         net.JoinHostPort("", cfg.Port),
			Handler:      mux,
			ReadTimeout:  healthServerReadTimeout,
			WriteTimeout: healthServerWriteTimeout,
			IdleTimeout:  healthServerIdleTimeout,
		},
		logger:    cfg.Logger,
		goNamedFn: pkg.GoNamed,
	}
}

// Start begins listening for health check requests in a background goroutine.
// Uses pkg.GoNamed for panic recovery to prevent unrecovered panics from
// crashing the process.
func (hs *HealthServer) Start() {
	hs.goNamedFn(hs.logger, "health-server", func() {
		hs.logger.Log(context.Background(), log.LevelInfo, "Health server listening", log.String("addr", hs.server.Addr))

		if err := hs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			hs.logger.Log(context.Background(), log.LevelError, "Health server error", log.Err(err))
		}
	})
}

// Shutdown gracefully stops the health server.
func (hs *HealthServer) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), healthServerShutdownTimeout)
	defer cancel()

	if err := hs.server.Shutdown(ctx); err != nil {
		hs.logger.Log(ctx, log.LevelError, "Health server shutdown error", log.Err(err))
	}
}

// newWorkerHealthHandler returns the Worker's /health (liveness) handler,
// gated by the startup self-probe state.
//
// Behavior mirrors the Manager's /health (see NewManagerHealthHandler):
//   - state==nil: returns 200 (treated as healthy). Preserves the pre-Gate-7
//     contract for test wiring without explicit state.
//   - state.IsHealthy()=false: returns 503 with status="unhealthy". K8s
//     livenessProbe interprets the 503 as "restart this pod"; we
//     deliberately do not exit the process ourselves so logs flush and
//     the operator sees the failed probe in CloudWatch / Loki.
//   - state.IsHealthy()=true: returns 200 with status="alive".
//
// Bodies are hand-written JSON literals — these handlers run on the hot
// path of K8s liveness probes (typically every 1–5s) and a constant
// literal avoids per-request allocations.
func newWorkerHealthHandler(state *readyz.SelfProbeState) http.HandlerFunc {
	const aliveBody = `{"status":"alive"}`

	const unhealthyBody = `{"status":"unhealthy","reason":"startup self-probe has not succeeded; pod will be restarted by K8s livenessProbe"}`

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if state != nil && !state.IsHealthy() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(unhealthyBody))

			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(aliveBody))
	}
}
