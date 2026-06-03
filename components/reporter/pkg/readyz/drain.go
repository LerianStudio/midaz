// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package readyz

import "sync/atomic"

// DrainState is a goroutine-safe flag indicating whether the service has
// begun graceful shutdown. /readyz handlers consult IsDraining() and
// short-circuit to a 503 response when true so K8s and load balancers
// stop sending new traffic before the process exits.
//
// The flag is one-way: once StartDraining() is called, it stays set until
// the process exits. There is no Stop or Reset by design — a draining
// service should never re-enter the "ready" state.
//
// Wraps atomic.Bool with named accessors so the intent is obvious at call
// sites and the type is composable in larger structs.
type DrainState struct {
	flag atomic.Bool
}

// StartDraining marks the service as draining. Subsequent IsDraining()
// calls will return true. Idempotent — repeated calls are no-ops.
func (d *DrainState) StartDraining() {
	d.flag.Store(true)
}

// IsDraining reports whether StartDraining has been called.
// Safe to call from any goroutine.
func (d *DrainState) IsDraining() bool {
	return d.flag.Load()
}
