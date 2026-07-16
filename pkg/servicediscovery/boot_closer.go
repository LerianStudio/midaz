// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package servicediscovery

import (
	"context"

	libLog "github.com/LerianStudio/lib-observability/log"
	libsd "github.com/LerianStudio/lib-service-discovery"
)

// BootCloser tears down the boot-time service-discovery Resolve watcher when a
// composition root aborts after wiring discovery but before the launcher
// Runnable takes over. When discovery is enabled, the boot-time plugin-auth
// resolve lazy-spawns a background watcher goroutine; if the Service is never
// built, that Runnable never runs and the watcher would leak. A BootCloser is
// armed at construction and closes the Manager on a deferred CloseOnBootFailure
// unless it has been disarmed on the success path, where the Runnable then owns
// the graceful close. libsd.Manager.Close is idempotent, so CloseOnBootFailure
// is a safe backstop even if the manager is closed elsewhere.
type BootCloser struct {
	logger  libLog.Logger
	manager *libsd.Manager
	armed   bool
}

// NewBootCloser returns an armed BootCloser bound to the given manager. Defer
// CloseOnBootFailure immediately after construction and call Disarm on the
// success path once the launcher Runnable owns the manager.
func NewBootCloser(logger libLog.Logger, manager *libsd.Manager) *BootCloser {
	return &BootCloser{logger: logger, manager: manager, armed: true}
}

// Disarm marks the closer inactive so a subsequent CloseOnBootFailure is a
// no-op. Called on the success path once the launcher Runnable takes ownership
// of the graceful close. Nil-safe on the receiver.
func (b *BootCloser) Disarm() {
	if b == nil {
		return
	}

	b.armed = false
}

// CloseOnBootFailure closes the manager only when the closer is still armed and
// a manager is present, tearing down the boot-time Resolve watcher on a
// partial-boot failure. A close error is logged at Warn and never propagated.
// Nil-safe on the receiver, the manager, and the logger.
func (b *BootCloser) CloseOnBootFailure() {
	if b == nil || !b.armed || b.manager == nil {
		return
	}

	if err := b.manager.Close(); err != nil && b.logger != nil {
		b.logger.Log(
			context.Background(), libLog.LevelWarn,
			"Failed to close service discovery manager during bootstrap cleanup",
			libLog.Err(err),
		)
	}
}
