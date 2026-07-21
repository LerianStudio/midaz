// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"strconv"
	"strings"

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libStreaming "github.com/LerianStudio/lib-streaming"
	"github.com/redis/go-redis/v9"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/http/in"
)

// These generic sentinels are recorded on the probe spans (never on the wire —
// the /readyz handler surfaces canonical numeric codes). They carry no client
// detail so nothing sensitive leaks even into telemetry.
var (
	errTenantManagerNotWired = errors.New("tenant manager client not wired")
	errTenantManagerProbe    = errors.New("tenant manager probe failed")
	errStreamingNotWired     = errors.New("streaming emitter not wired")
	errStreamingProbe        = errors.New("streaming probe failed")
)

// redisPingerAdapter adapts the raw go-redis UniversalClient to in.RedisPinger.
// Kept in the bootstrap package so the http/in package stays free of the
// go-redis import — the interface is the only coupling point.
type redisPingerAdapter struct {
	client redis.UniversalClient
}

// newRedisPinger wraps the multi-tenant Pub/Sub client for the redis /readyz
// probe. Returns a nil in.RedisPinger when the client is nil so the probe's
// nil-guard reports "connection not established" rather than dereferencing.
func newRedisPinger(client redis.UniversalClient) in.RedisPinger {
	if client == nil {
		return nil
	}

	return &redisPingerAdapter{client: client}
}

// Ping issues a single go-redis PING and returns its error verbatim; the probe
// maps a non-nil error to the canonical sentinel, so no raw client text reaches
// the wire.
func (a *redisPingerAdapter) Ping(ctx context.Context) error {
	if a == nil || a.client == nil {
		return in.ErrRedisConnectionNotEstablished
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	return a.client.Ping(ctx).Err()
}

// activeTenantsGetter is the minimal slice of *tmclient.Client the readiness
// probe depends on. Narrowing to this interface (rather than the concrete
// client) lets the classification logic be unit-tested with a fake — the real
// *tmclient.Client satisfies it.
type activeTenantsGetter interface {
	GetActiveTenantsByService(ctx context.Context, service string) ([]*tmclient.TenantSummary, error)
}

// tenantManagerHealthProber adapts the tenant-manager HTTP client to
// in.TenantManagerProber. The client has no dedicated health signal, so the
// probe uses GetActiveTenantsByService as a functional check and classifies its
// error against the tenant-manager circuit-breaker sentinel here — keeping the
// tenant-manager import out of the http/in package.
type tenantManagerHealthProber struct {
	client  activeTenantsGetter
	service string
}

// newTenantManagerHealthProber wires the tenant-manager readiness prober.
// Returns a nil in.TenantManagerProber when the client is nil so the probe
// reports "down" via its nil-guard.
func newTenantManagerHealthProber(client *tmclient.Client, service string) in.TenantManagerProber {
	if client == nil {
		return nil
	}

	return &tenantManagerHealthProber{client: client, service: service}
}

// tmStatusMessagePrefix is the fixed leader of the tenant-manager client's
// non-2xx error message. lib-commons v5.10.0
// GetActiveTenantsByService returns a plain
// fmt.Errorf("tenant manager returned status %d for service %s", ...) for every
// non-2xx round-trip — there is no typed error, exported sentinel, or status
// accessor to key off (the exported core.Err* sentinels are only wrapped by
// GetTenantConfig, not this call). Distinguishing a reachable 4xx from a 5xx
// therefore requires reading the status code out of that message. This couples
// the probe to the client's message shape; if lib-commons ever exposes a typed
// status error, switch to it and delete this.
const tmStatusMessagePrefix = "tenant manager returned status "

// Probe classifies the tenant-manager readiness into the closed /readyz
// vocabulary:
//
//   - nil error                        => up
//   - core.ErrCircuitBreakerOpen       => degraded (client is serving fail-fast)
//   - reachable 4xx (client answered)  => up   (the service IS up; only its
//     answer is a client error — e.g. a rotated service key or unknown service)
//   - 5xx / transport / read failure   => down
//
// Tenant Manager stays a HARD /readyz gate, so only a genuinely unreachable or
// server-erroring dependency flips it down.
//
// ACCEPTED COUPLING: this functional probe drives the SAME production circuit
// breaker as live tenant resolution (GetActiveTenantsByService internally calls
// recordSuccess/recordFailure), because the tenant-manager client exposes no
// read-only health surface. A reachable 4xx resets the failure counter
// (recordSuccess) client-side, consistent with classifying it as "up" here.
//
// The returned error is generic (never the raw client error) so span
// attribution stays free of internal detail.
func (p *tenantManagerHealthProber) Probe(ctx context.Context) (string, error) {
	if p == nil || p.client == nil {
		return in.StatusDown, errTenantManagerNotWired
	}

	if ctx.Err() != nil {
		return in.StatusDown, errTenantManagerProbe
	}

	_, err := p.client.GetActiveTenantsByService(ctx, p.service)

	switch {
	case err == nil:
		return in.StatusUp, nil
	case errors.Is(err, tmcore.ErrCircuitBreakerOpen):
		return in.StatusDegraded, tmcore.ErrCircuitBreakerOpen
	default:
		if code, ok := tmHTTPStatusFromError(err); ok && code >= 400 && code < 500 {
			return in.StatusUp, nil
		}

		return in.StatusDown, errTenantManagerProbe
	}
}

// tmHTTPStatusFromError extracts the HTTP status code the tenant-manager client
// reported on a non-2xx round-trip. It matches only on the exact message prefix
// (see tmStatusMessagePrefix) so transport / read / parse failures — which use
// different message shapes — never match and correctly fall through to "down".
// Returns (code, true) only when a numeric code follows the prefix.
func tmHTTPStatusFromError(err error) (int, bool) {
	if err == nil {
		return 0, false
	}

	idx := strings.Index(err.Error(), tmStatusMessagePrefix)
	if idx < 0 {
		return 0, false
	}

	fields := strings.Fields(err.Error()[idx+len(tmStatusMessagePrefix):])
	if len(fields) == 0 {
		return 0, false
	}

	code, convErr := strconv.Atoi(fields[0])
	if convErr != nil {
		return 0, false
	}

	return code, true
}

// streamingHealthProber adapts a lib-streaming Emitter to in.StreamingHealthProber.
// Emitter.Healthy returns a tri-state HealthError; decoding its .State() needs
// the concrete lib-streaming type, so the classification lives here and the
// http/in package sees only the resolved status.
type streamingHealthProber struct {
	emitter libStreaming.Emitter
}

// newStreamingHealthProber wires the streaming readiness prober. The emitter is
// always non-nil in bootstrap (a NoopEmitter when streaming is disabled), but
// the nil-guard keeps the adapter safe if that ever changes.
func newStreamingHealthProber(emitter libStreaming.Emitter) in.StreamingHealthProber {
	if emitter == nil {
		return nil
	}

	return &streamingHealthProber{emitter: emitter}
}

// Probe classifies the streaming producer readiness into the closed /readyz
// vocabulary. lib-streaming's HealthState maps directly: Healthy=>up,
// Degraded=>degraded, Down=>down; a non-HealthError failure fails closed to
// down. The returned error is lib-streaming's HealthError, whose message is
// already broker-URL-sanitized, so it is safe for span attribution.
func (p *streamingHealthProber) Probe(ctx context.Context) (string, error) {
	if p == nil || p.emitter == nil {
		return in.StatusDown, errStreamingNotWired
	}

	if ctx.Err() != nil {
		return in.StatusDown, errStreamingProbe
	}

	err := p.emitter.Healthy(ctx)
	if err == nil {
		return in.StatusUp, nil
	}

	var he *libStreaming.HealthError
	if errors.As(err, &he) {
		switch he.State() {
		case libStreaming.Healthy:
			return in.StatusUp, nil
		case libStreaming.Degraded:
			return in.StatusDegraded, err
		case libStreaming.Down:
			return in.StatusDown, err
		}
	}

	// Unclassifiable failure (not a *HealthError) — fail closed to down and
	// return the generic sentinel so no raw broker/topology detail reaches the
	// probe span. Mirrors the tenant_manager prober; the confirmed HealthError
	// branches above are already broker-URL-sanitized and safe to surface.
	return in.StatusDown, errStreamingProbe
}
