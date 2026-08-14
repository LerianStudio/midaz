// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
)

// postgresSelfProbe satisfies SelfProbeChecker for the PostgreSQL dep at
// startup. Reuses the *in.HealthChecker's PostgresDBProvider so the boot-time
// probe exercises the same code path as the per-request /readyz probe — no
// drift between "boot passes but readyz fails".
type postgresSelfProbe struct {
	hc *in.HealthChecker
}

func newPostgresSelfProbe(hc *in.HealthChecker) *postgresSelfProbe {
	return &postgresSelfProbe{hc: hc}
}

// Check probes the primary PostgreSQL connection by acquiring a *sql.DB and
// pinging it. Honours ctx cancellation through PingContext.
func (p *postgresSelfProbe) Check(ctx context.Context) error {
	if p == nil || p.hc == nil {
		return errors.New("postgres self-probe: health checker not wired")
	}

	provider := p.hc.PostgresProvider()
	if provider == nil || !provider.IsConnected() {
		return errors.New("postgres self-probe: connection not established")
	}

	db, err := provider.GetDB(ctx)
	if err != nil {
		return fmt.Errorf("postgres self-probe: get db: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("postgres self-probe: ping: %w", err)
	}

	return nil
}

// ruleCacheSelfProbe satisfies SelfProbeChecker for the in-process rule
// cache. In single-tenant mode the bootstrap warmup populates the empty-
// tenant ("") bucket; the probe asserts IsReady on that bucket so a failed
// warmup keeps /health at 503.
type ruleCacheSelfProbe struct {
	hc *in.HealthChecker
}

func newRuleCacheSelfProbe(hc *in.HealthChecker) *ruleCacheSelfProbe {
	return &ruleCacheSelfProbe{hc: hc}
}

// Check returns nil when the cache reports ready for the empty-tenant
// bucket (single-tenant mode). The cache is in-process, so no transport
// timeout is needed; the underlying provider returns synchronously.
func (r *ruleCacheSelfProbe) Check(ctx context.Context) error {
	if r == nil || r.hc == nil {
		return errors.New("rule cache self-probe: health checker not wired")
	}

	provider := r.hc.CacheHealthProvider()
	if provider == nil {
		return errors.New("rule cache self-probe: provider not configured")
	}

	if !provider.IsReady(ctx) {
		return errors.New("rule cache self-probe: cache not ready")
	}

	return nil
}
