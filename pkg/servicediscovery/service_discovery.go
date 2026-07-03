// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package servicediscovery holds the shared service-discovery wiring adopted
// identically by the ledger and CRM composition roots: Manager construction,
// server-port parsing, descriptor building, plugin-auth host resolution, and the
// register/deregister Launcher runnable.
package servicediscovery

import (
	"fmt"
	"net"
	"strconv"
	"time"

	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
)

// ResolveTimeout bounds the boot-time plugin-auth resolve so a TCP-reachable but
// slow/hung registry (brownout) cannot stall boot. On deadline the resolve
// degrades to the static auth host, keeping "a discovery outage never stalls
// boot" true for the slow-registry case, not just connection refused.
const ResolveTimeout = 5 * time.Second

// DeregisterTimeout bounds the shutdown deregister call so a slow or unreachable
// registry cannot hold the process open at exit. TTL expiry is the backstop when
// deregister does not complete in time.
const DeregisterTimeout = 5 * time.Second

// BuildManager constructs the service-discovery Manager from SD_* env vars. When
// discovery is disabled the returned Manager is a working no-op, so callers can
// invoke Register/Resolve unconditionally. The returned bool mirrors SD_ENABLED
// so the caller can decide whether to wire a register/deregister runnable.
// Returns an error (fail-fast) when discovery is enabled but misconfigured, e.g.
// an empty advertise address.
func BuildManager(logger libLog.Logger) (*libsd.Manager, bool, error) {
	sdCfg := libsd.ConfigFromEnv()

	m, err := libsd.New(sdCfg, libsd.WithLogger(logger))
	if err != nil {
		return nil, sdCfg.Enabled, fmt.Errorf("initializing service discovery: %w", err)
	}

	return m, sdCfg.Enabled, nil
}

// ParseServerPort extracts the numeric port from a listen address. It accepts
// both the leading-colon form (":3002") and the host:port form ("0.0.0.0:8080");
// net.SplitHostPort handles both. A malformed address is a config bug and
// surfaces as an error for fail-fast handling at wiring time.
func ParseServerPort(serverAddress string) (int, error) {
	_, portStr, err := net.SplitHostPort(serverAddress)
	if err != nil {
		return 0, fmt.Errorf("parsing server address %q: %w", serverAddress, err)
	}

	return strconv.Atoi(portStr)
}

// BuildServiceDescriptor builds the registry descriptor advertised by a service
// instance. Address and Scheme are intentionally left unset: Manager.Register
// fills them from SD_ADVERTISE_ADDRESS. The TTL health check needs no reachable
// HTTP endpoint — the registry heartbeats from inside the process. name is the
// registry service name (e.g. "midaz-ledger", "midaz-crm").
func BuildServiceDescriptor(name string, port int) libsd.Service {
	return libsd.Service{
		ID:          name + "-" + strconv.Itoa(port),
		Name:        name,
		Port:        port,
		HealthCheck: &libsd.HealthCheck{TTL: "30s"},
	}
}
