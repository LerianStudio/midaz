// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	mongoDB "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/readyz"
	libRedis "github.com/LerianStudio/midaz/v4/pkg/reporter/redis"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/storage"

	libRabbitmq "github.com/LerianStudio/lib-commons/v5/commons/rabbitmq"
	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	"github.com/gofiber/fiber/v2"
)

// ManagerReadyzDeps bundles the connections, configuration flags, and shared
// state required to assemble the Manager's /readyz handler. It is built in
// bootstrap and passed through routes.go to NewManagerReadyzHandler.
//
// All fields are optional in the sense that nil/zero values are tolerated —
// each pkg/readyz Checker handles its own missing-dependency case (e.g.
// nil MongoConnection reports StatusDown). This keeps the handler factory
// permissive so the same code path works in single-tenant and multi-tenant
// configurations.
type ManagerReadyzDeps struct {
	// MongoConnection is the static (non-tenant-scoped) MongoDB connection.
	// In multi-tenant mode this is nil and the MongoChecker reports n/a.
	MongoConnection *mongoDB.MongoConnection

	// RabbitMQConnection is the static RabbitMQ connection used by the
	// Manager to publish messages. In multi-tenant mode the RabbitMQChecker
	// reports n/a.
	RabbitMQConnection *libRabbitmq.RabbitMQConnection

	// RedisConnection is the Manager's Redis/Valkey connection (used by
	// rate limiter and caching).
	RedisConnection *libRedis.RedisConnection

	// StorageClient is the S3-compatible object storage adapter.
	StorageClient storage.ObjectStorage

	// StorageEndpoint is the configured storage endpoint URL, used only by
	// the StorageChecker for TLS posture detection (not for the probe).
	StorageEndpoint string

	// TenantManagerClient is the Tenant Manager client. nil when
	// MultiTenantEnabled=false. The TenantManagerChecker performs only a
	// nil-check — it does NOT issue an HTTP probe, per the existing decision
	// at init_tenant.go:117-132.
	TenantManagerClient *tmclient.Client

	// MultiTenantEnabled mirrors the MULTI_TENANT_ENABLED env var.
	MultiTenantEnabled bool

	// MongoURI is the configured MongoDB URI, used only for TLS posture
	// detection (the MongoChecker does not re-dial from this string).
	MongoURI string

	// RabbitURI is the configured RabbitMQ URI, used only for TLS posture
	// detection.
	RabbitURI string

	// DrainState is the shared graceful-shutdown flag. When IsDraining()
	// returns true the handler short-circuits to a 503 "draining" response.
	DrainState *readyz.DrainState

	// Version is emitted in every /readyz response (top-level field).
	Version string

	// DeploymentMode echoes the DEPLOYMENT_MODE env var (saas | byoc | local).
	DeploymentMode string

	// Metrics is the OTel emitter for the canonical readyz metric set
	// (readyz_check_duration_ms, readyz_check_status, selfprobe_result).
	// nil is tolerated — the handler no-ops emission rather than panicking.
	// Production bootstrap MUST construct one via readyz.NewMetrics(meter).
	Metrics *readyz.Metrics

	// SelfProbeState gates the /health endpoint. Starts unhealthy; bootstrap
	// flips it to healthy via MarkHealthy() once readyz.RunSelfProbe
	// succeeds. nil is tolerated (treated as healthy) so tests can mount
	// the routes without explicit wiring; production bootstrap MUST set it.
	SelfProbeState *readyz.SelfProbeState
}

// BuildManagerCheckers assembles the canonical dependency checker list for the
// Manager. Exported so bootstrap can reuse the exact same list for both
// startup self-probe (Gate 7) and the runtime /readyz handler — keeping the
// two surfaces in lock-step. nil deps yields the same defensive behavior as
// the handler factory: every probe reports its own missing-dependency state.
//
// The remote Fetcher reachability checker has been removed: schema discovery
// runs in-process, so there is no external Fetcher dependency to probe.
// Per-tenant datasource connectivity is exercised lazily on the schema-read
// path itself and surfaces there.
func BuildManagerCheckers(deps *ManagerReadyzDeps) []readyz.Checker {
	if deps == nil {
		deps = &ManagerReadyzDeps{}
	}

	return []readyz.Checker{
		readyz.NewMongoChecker(deps.MongoConnection, deps.MultiTenantEnabled),
		readyz.NewRabbitMQChecker(deps.RabbitMQConnection, deps.MultiTenantEnabled),
		// Redis is always required on the Manager (rate-limiter, idempotency,
		// multi-tenant cache). A nil connection is a hard failure.
		readyz.NewRedisChecker(deps.RedisConnection, true),
		readyz.NewStorageChecker(deps.StorageClient, deps.StorageEndpoint),
		readyz.NewTenantManagerChecker(deps.MultiTenantEnabled, func() bool {
			return deps.TenantManagerClient != nil
		}),
	}
}

// NewManagerReadyzHandler assembles the Manager's /readyz Fiber handler from
// the bundled dependencies. The returned handler is mounted at /readyz in
// routes.go.
func NewManagerReadyzHandler(deps *ManagerReadyzDeps) fiber.Handler {
	if deps == nil {
		// Defensive default: build a handler that reports the service
		// itself is misconfigured by reporting every dependency down.
		deps = &ManagerReadyzDeps{}
	}

	checkers := BuildManagerCheckers(deps)

	return readyz.NewHandler(checkers, deps.DrainState, deps.Version, deps.DeploymentMode, deps.Metrics)
}

// NewManagerHealthHandler returns the Manager's /health (liveness) handler,
// gated by the startup self-probe state.
//
// Behavior:
//   - state==nil: returns 200 (treated as healthy). This keeps tests and
//     partial-bootstrap code paths working without explicit wiring.
//   - state.IsHealthy()=false: returns 503 with status="unhealthy" and a
//     reason pointing at the self-probe. K8s livenessProbe interprets the
//     503 as "restart this pod"; we deliberately do not exit the process
//     ourselves so logs flush and the operator sees the failed probe in
//     CloudWatch / Loki.
//   - state.IsHealthy()=true: returns 200 with status="alive".
//
// Gate 7 of ring:dev-readyz couples this to readyz.RunSelfProbe at bootstrap.
func NewManagerHealthHandler(state *readyz.SelfProbeState) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if state != nil && !state.IsHealthy() {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "unhealthy",
				"reason": "startup self-probe has not succeeded; pod will be restarted by K8s livenessProbe",
			})
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "alive"})
	}
}
