// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/tracer"
)

// TracerReserver is the narrow port the transaction create seam depends on to
// drive the tracer's two-phase reservation lifecycle. It is declared here, at
// the consuming seam, so the concrete HTTP client can be injected at bootstrap
// and faked in tests — mirroring the FeeApplier precedent.
//
// Reserve holds limit capacity for the fee-inclusive transaction (phase one)
// and returns a handle carrying the reservation ids and the limit-exceeded
// decision. Confirm commits the held capacity on a successful transaction;
// Release returns it on an aborted one. A nil reserver means the tracer
// integration is disabled (tracer.mode=off / unconfigured) and the create path
// stays unchanged — call sites guard with a nil check, mirroring the streaming
// nil-emitter pattern.
//
// Availability failures (timeout, transport error, open breaker) surface as
// tracer.ErrTracerUnavailable so the anchor can branch on tracer.failPosture;
// a DENIED decision is a successful Reserve return (handle.Denied=true), not an
// error.
type TracerReserver interface {
	Reserve(ctx context.Context, req tracer.ReserveRequest) (*tracer.ReserveResult, error)
	Confirm(ctx context.Context, reservationID uuid.UUID) error
	Release(ctx context.Context, reservationID uuid.UUID) error
}
