// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"

	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
)

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
