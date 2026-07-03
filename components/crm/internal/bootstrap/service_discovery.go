// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"

	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
)

// serviceDiscoveryWiring holds the service-discovery outputs consumed by the
// composition root: the Manager, whether discovery is enabled, the descriptor to
// register, and the plugin-auth host resolved (or degraded) via discovery.
type serviceDiscoveryWiring struct {
	manager    *libsd.Manager
	enabled    bool
	descriptor libsd.Service
	authHost   string
}

// wireServiceDiscovery performs the full service-discovery composition: it builds
// the Manager (fail-fast on misconfiguration), parses the advertised port from
// the listen address (fail-fast on a malformed address), builds this instance's
// registry descriptor, and resolves the plugin-auth host — degrading to the
// static PLUGIN_AUTH_ADDRESS when auth is disabled or resolution fails so a
// discovery outage never fails boot. Extracted from InitServersWithOptions to
// keep the composition root's branch count within the gocyclo budget.
func wireServiceDiscovery(cfg *Config, logger libLog.Logger) (serviceDiscoveryWiring, error) {
	manager, enabled, err := buildServiceDiscovery(logger)
	if err != nil {
		return serviceDiscoveryWiring{}, err
	}

	serverPort, err := parseServerPort(cfg.ServerAddress)
	if err != nil {
		return serviceDiscoveryWiring{}, err
	}

	resolveCtx, cancel := context.WithTimeout(context.Background(), serviceDiscoveryResolveTimeout)
	defer cancel()

	return serviceDiscoveryWiring{
		manager:    manager,
		enabled:    enabled,
		descriptor: buildCRMServiceDescriptor(serverPort),
		authHost:   resolveAuthHost(resolveCtx, manager, cfg.AuthEnabled, cfg.AuthAddress),
	}, nil
}

// buildServiceDiscovery constructs the service-discovery Manager from SD_* env
// vars. When discovery is disabled the returned Manager is a working no-op, so
// callers can invoke Register/Resolve unconditionally. The returned bool mirrors
// SD_ENABLED so the caller can decide whether to wire a register/deregister
// runnable. Returns an error (fail-fast) when discovery is enabled but
// misconfigured, e.g. an empty advertise address.
func buildServiceDiscovery(logger libLog.Logger) (*libsd.Manager, bool, error) {
	sdCfg := libsd.ConfigFromEnv()

	m, err := libsd.New(sdCfg, libsd.WithLogger(logger))
	if err != nil {
		return nil, sdCfg.Enabled, fmt.Errorf("initializing service discovery: %w", err)
	}

	return m, sdCfg.Enabled, nil
}
