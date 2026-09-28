// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import "time"

// ReservationTTL is the lifetime of capacity held for a DIRECT transaction
// before expiry may return it. It bounds how long an abandoned reservation can
// hold capacity when the producer neither confirms nor releases (crash between
// reserve and commit). Direct transactions resolve in seconds, so a short TTL
// converges quickly.
const ReservationTTL = 5 * time.Minute

// DefaultLongLivedReservationTTL is the lifetime granted to capacity held for a
// PENDING transaction (the longLived reserve hint). PENDING transactions persist
// with no sweep of their own, so the direct TTL would expire capacity backing a
// still-valid pending. 30 days is long enough that real pending lifetimes never
// hit it and short enough that a genuinely abandoned pending still converges.
// Operators tune it via RESERVATION_LONG_LIVED_TTL_HOURS.
const DefaultLongLivedReservationTTL = 720 * time.Hour // 30 days
