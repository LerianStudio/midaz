// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build libsd

package servicediscovery

import (
	"context"
	"fmt"

	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
)

// Manager wraps the lib-service-discovery Manager so the composition roots
// depend only on this package and never import lib-service-discovery directly.
//
// TODO(3482): this build (-tags libsd) currently will NOT resolve until
// lib-service-discovery ships a release built against lib-observability/v2
// (+ lib-commons/v6) — libsd.WithLogger below rejects the now-v2 log.Logger.
// The real implementation is preserved here syntactically for restoration once
// that release lands; the default build no-ops SD (see manager_noop.go).
type Manager struct {
	inner *libsd.Manager
}

// BuildManager constructs the service-discovery Manager from SD_* env vars. When
// discovery is disabled the returned Manager is a working no-op, so callers can
// invoke Register/Resolve unconditionally. The returned bool mirrors SD_ENABLED
// so the caller can decide whether to wire a register/deregister runnable.
// Returns an error (fail-fast) when discovery is enabled but misconfigured, e.g.
// no advertise address is set. The advertise address is read from the canonical
// SD_EXTERNAL_ADDRESS (legacy SD_ADVERTISE_ADDRESS / SERVICE_ADVERTISE_ADDR still
// honored). lib-service-discovery moved advertise validation out of New into
// Register, so the guard below re-asserts fail-fast at boot rather than deferring
// the failure to the first register attempt.
func BuildManager(logger libLog.Logger) (*Manager, bool, error) {
	sdCfg := libsd.ConfigFromEnv()

	if sdCfg.Enabled && sdCfg.AdvertiseAddr == "" && sdCfg.AdvertiseInternalAddr == "" {
		return nil, sdCfg.Enabled, fmt.Errorf("initializing service discovery: %w", libsd.ErrNoEndpoint)
	}

	m, err := libsd.New(sdCfg, libsd.WithLogger(logger))
	if err != nil {
		return nil, sdCfg.Enabled, fmt.Errorf("initializing service discovery: %w", err)
	}

	return &Manager{inner: m}, sdCfg.Enabled, nil
}

// ResolvePreferredURL resolves a service to a preferred, scheme-complete URL via
// the wrapped library Manager, returning the fallback verbatim when discovery is
// disabled or fails. It satisfies the AuthHostResolver contract.
func (m *Manager) ResolvePreferredURL(ctx context.Context, name, fallback string) (string, error) {
	return m.inner.ResolvePreferredURL(ctx, name, fallback)
}

// toService converts the library-neutral Descriptor to the lib-service-discovery
// Service registered against the registry. Address and Scheme are intentionally
// left unset: Manager.Register fills them from SD_EXTERNAL_ADDRESS. The TTL
// health check needs no reachable HTTP endpoint — the registry heartbeats from
// inside the process.
func (d Descriptor) toService() libsd.Service {
	return libsd.Service{
		ID:          d.ID,
		Name:        d.Name,
		Port:        d.Port,
		HealthCheck: &libsd.HealthCheck{TTL: d.HealthCheckTTL},
	}
}
