// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ReservationSpec is one counter-backed limit's resolved reservation parameters.
// It carries everything the reservation row and the reserve CTE need so confirm
// and release never re-query limits (R38): the row stores LimitID/ScopeKey/
// PeriodKey/Amount, and the reserve guard uses MaxAmount. Amounts are decimal
// currency values (e.g. 10.50), matching the DECIMAL counter columns.
type ReservationSpec struct {
	LimitID   uuid.UUID
	ScopeKey  string
	PeriodKey string
	Amount    decimal.Decimal
	MaxAmount decimal.Decimal
}
