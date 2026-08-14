// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

//go:generate mockgen -source=reservation_reaper_repository.go -destination=mocks/reservation_reaper_repository_mock.go -package=mocks

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ReservationReaperRepository is the narrow surface the TTL reaper consumes. It
// is a subset of the reservation lifecycle: the reaper only needs to find the
// outstanding RESERVED rows that have passed their TTL and release each one as
// EXPIRED. It is deliberately separate from the full ReservationRepository (the
// confirm/release-by-id contract used by the two-phase API) so the worker
// package does not depend on the service-layer transaction handle.
//
// Implementations release each reservation in its OWN transaction (counter
// bucket move + row flip atomic per row); the reaper batches the AUDIT side
// separately via a single summary event, so ReleaseExpired does NOT write a
// per-row audit row.
type ReservationReaperRepository interface {
	// FindExpiredReservations returns the ids of reservations still in the
	// RESERVED state whose reservation_expires_at is strictly before now. It
	// scans the idx_usage_reservations_reaper partial index. An empty slice
	// (not an error) means there is nothing to reap this cycle.
	FindExpiredReservations(ctx context.Context, now time.Time) ([]uuid.UUID, error)

	// ReleaseExpired flips a RESERVED reservation to EXPIRED and returns its held
	// amount from the counter's reserved_usage, atomically in one transaction.
	// A reservation that has already reached a terminal state is an idempotent
	// no-op (mirrors the confirm/release WHERE status='RESERVED' guard) and MUST
	// NOT be reported as an error — a concurrent confirm/release between the find
	// and the release is expected, not a fault.
	ReleaseExpired(ctx context.Context, reservationID uuid.UUID) error
}
