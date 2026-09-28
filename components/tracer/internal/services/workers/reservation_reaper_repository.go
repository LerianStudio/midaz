// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

//go:generate mockgen -source=reservation_reaper_repository.go -destination=mocks/reservation_reaper_repository_mock.go -package=mocks

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// ReservationReaperRepository is the narrow read surface the TTL reaper
// consumes: it locates the outstanding RESERVED rows that have passed their TTL.
// The rows expire through their owning operation (ReserveOperationExpirer), so
// the repository writes nothing.
type ReservationReaperRepository interface {
	// FindExpiredReservations returns at most limit reservations still in the
	// RESERVED state whose reservation_expires_at is strictly before now,
	// ordered by (expiry, id), each carrying its expiry and its owning
	// operation. A non-nil after resumes strictly past that position; nil
	// starts from the oldest expiry. It scans the idx_usage_reservations_reaper
	// partial index. The cap may split an operation's reservations across
	// sweeps. An empty slice (not an error) means there is nothing to reap from
	// that position.
	FindExpiredReservations(ctx context.Context, now time.Time, after *model.ReservationExpiryPosition, limit int) ([]model.ExpiredReservation, error)
}

// ReserveOperationExpirer closes a decision-owned operation as EXPIRED, returns
// all of its capacity and audits it in one transaction. released is the number
// of reservations it moved; zero means the operation was already terminal.
type ReserveOperationExpirer interface {
	Execute(ctx context.Context, key model.ReserveOperationIdentity, at time.Time) (released int, err error)
}
