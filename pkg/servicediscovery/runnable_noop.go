// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build !libsd

package servicediscovery

import (
	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
)

// Runnable is the no-op service-discovery Launcher app for the DEFAULT build.
// Discovery is disabled, so Run registers and deregisters nothing. The real
// register/deregister lifecycle lives in runnable_libsd.go under //go:build
// libsd (see the package doc and TODO(3482)).
type Runnable struct{}

// NewRunnable returns a no-op Runnable. It accepts the same arguments as the
// real constructor so the composition roots wire it identically regardless of
// build.
func NewRunnable(_ *Manager, _ Descriptor, _ libLog.Logger, _ MetricsRecorder) *Runnable {
	return &Runnable{}
}

// Run is a no-op that returns immediately: with discovery disabled there is
// nothing to register or deregister.
func (r *Runnable) Run(_ *libCommons.Launcher) error {
	return nil
}
