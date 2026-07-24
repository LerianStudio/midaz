// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package servicediscovery holds the shared service-discovery wiring adopted
// identically by the ledger and tracer composition roots: Manager construction,
// server-port parsing, descriptor building, plugin-auth host resolution, and the
// register/deregister Launcher runnable.
//
// TODO(3482): restore Service Discovery when lib-service-discovery publishes a
// release built against lib-observability/v2 (+ lib-commons/v6). Until then the
// DEFAULT build no-ops SD; the real implementation lives under //go:build libsd
// in the *_libsd.go files. lib-service-discovery v1.1.0 still requires
// lib-observability v1.1.0, so its libsd.WithLogger rejects Midaz's now-v2
// log.Logger (interface-identity mismatch across module majors). Because there
// is no upstream fix to adopt, SD is gated behind the build tag so the default
// build carries zero dependency on lib-service-discovery. The seam below keeps
// this package's public API stable across both builds: Manager, BuildManager,
// NewRunnable, and NewBootCloser exist in both, so the composition roots depend
// only on this package and never import lib-service-discovery directly.
package servicediscovery

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"time"
)

// hostnameFn resolves the host identity folded into the instance ID. It is a
// package var so tests can override it deterministically; production uses
// os.Hostname (= pod name in K8s, machine name on bare metal).
var hostnameFn = os.Hostname

// ResolveTimeout bounds the boot-time plugin-auth resolve so a TCP-reachable but
// slow/hung registry (brownout) cannot stall boot. On deadline the resolve
// degrades to the static auth host, keeping "a discovery outage never stalls
// boot" true for the slow-registry case, not just connection refused.
const ResolveTimeout = 5 * time.Second

// DeregisterTimeout bounds the shutdown deregister call so a slow or unreachable
// registry cannot hold the process open at exit. TTL expiry is the backstop when
// deregister does not complete in time.
const DeregisterTimeout = 5 * time.Second

// Descriptor is the registry descriptor advertised by a service instance,
// modeled independently of lib-service-discovery so the composition roots depend
// only on this package. Under //go:build libsd it is converted to the library's
// Service type at register time; in the default (no-op) build it is never
// registered. Address and Scheme are intentionally not modeled here: the library
// fills them from SD_EXTERNAL_ADDRESS at Register.
type Descriptor struct {
	// ID is the registry instance ID. It folds in the host identity so every
	// replica registers a distinct ID against the same Name.
	ID string
	// Name is the registry service name (e.g. "midaz-ledger", "midaz-tracer")
	// and stays stable — consumers resolve by it.
	Name string
	// Port is the advertised service port.
	Port int
	// HealthCheckTTL is the TTL advertised for the registry heartbeat health
	// check. The registry heartbeats from inside the process, so no reachable
	// HTTP endpoint is needed.
	HealthCheckTTL string
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
// instance. name is the registry service name (e.g. "midaz-ledger",
// "midaz-tracer") and stays stable — consumers resolve by it.
//
// The instance ID folds in the host identity ("<name>-<host>-<port>") so every
// replica registers a distinct ID against the same name; without it, N pods
// sharing a name collide on one central registry and their TTL health flaps. If
// the host is unresolvable it falls back to the legacy "<name>-<port>" scheme:
// a descriptor must always be buildable, so this never errors.
func BuildServiceDescriptor(name string, port int) Descriptor {
	id := name + "-" + strconv.Itoa(port)

	if host, err := hostnameFn(); err == nil && host != "" {
		id = name + "-" + host + "-" + strconv.Itoa(port)
	}

	return Descriptor{
		ID:             id,
		Name:           name,
		Port:           port,
		HealthCheckTTL: "30s",
	}
}
