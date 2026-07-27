// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build !libsd

package servicediscovery

import (
	"context"
	"errors"

	libLog "github.com/LerianStudio/lib-observability/v2/log"
)

// errServiceDiscoveryDisabled is returned by the no-op Manager's resolve so
// ResolveAuthHost degrades to the configured static host — matching the
// SD-disabled behavior the real library exhibits when discovery is off.
var errServiceDiscoveryDisabled = errors.New("service discovery disabled")

// Manager is the no-op service-discovery handle used by the DEFAULT build. The
// default build carries zero dependency on lib-service-discovery: register,
// resolve, and close are safe no-ops, and resolvers fall back to the configured
// static addresses exactly as SD-disabled mode does today.
//
// See the package doc and TODO(3482): the real Manager (which wraps
// lib-service-discovery) lives in manager_libsd.go under //go:build libsd.
type Manager struct{}

// BuildManager returns a disabled no-op Manager. The returned bool is always
// false (discovery disabled) so callers wire no register/deregister runnable,
// preserving the SD-disabled semantics. It never errors.
func BuildManager(_ libLog.Logger) (*Manager, bool, error) {
	return &Manager{}, false, nil
}

// ResolvePreferredURL always returns an error so ResolveAuthHost degrades to the
// configured static host, matching SD-disabled semantics. It satisfies the
// AuthHostResolver contract so the composition roots can call ResolveAuthHost
// unconditionally.
func (m *Manager) ResolvePreferredURL(_ context.Context, _, _ string) (string, error) {
	return "", errServiceDiscoveryDisabled
}
