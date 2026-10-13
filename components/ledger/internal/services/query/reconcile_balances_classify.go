// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// ClassifyBalanceTrailPair applies the reconciler rule in this order:
// grace → no trail → legacy trail → version order → values.
//
// Grace is measured against the newest of the balance update and the moment the trail
// head was recorded; a non-positive grace disables deferral. A trail head at version 0
// predates the operation version columns, so a balance that has moved since cannot be
// compared and is unverifiable. The values of a validated-cancel CREDIT head are a
// historical projection, so only its version is compared.
func ClassifyBalanceTrailPair(pair mmodel.BalanceTrailPair, now time.Time, grace time.Duration) mmodel.BalanceTrailDirection {
	newest := pair.UpdatedAt
	if pair.Trail != nil && pair.Trail.RecordedAt.After(newest) {
		newest = pair.Trail.RecordedAt
	}

	if grace > 0 && newest.After(now.Add(-grace)) {
		return mmodel.BalanceTrailDeferred
	}

	if pair.Trail == nil {
		if pair.Version == 0 {
			return mmodel.BalanceTrailOK
		}

		return mmodel.BalanceTrailBalanceAhead
	}

	if pair.Trail.Version == 0 && pair.Version > 0 {
		return mmodel.BalanceTrailUnverifiable
	}

	switch {
	case pair.Version > pair.Trail.Version:
		return mmodel.BalanceTrailBalanceAhead
	case pair.Version < pair.Trail.Version:
		return mmodel.BalanceTrailTrailAhead
	}

	if pair.Trail.ValidatedCancelCredit {
		return mmodel.BalanceTrailOK
	}

	if !pair.Available.Equal(pair.Trail.Available) || !pair.OnHold.Equal(pair.Trail.OnHold) {
		return mmodel.BalanceTrailValueMismatch
	}

	return mmodel.BalanceTrailOK
}
