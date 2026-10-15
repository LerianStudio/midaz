// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestClassifyBalanceTrailPair(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.October, 13, 12, 0, 0, 0, time.UTC)
	grace := 5 * time.Minute
	old := now.Add(-time.Hour)
	insideGrace := now.Add(-time.Minute)
	boundary := now.Add(-grace)

	dec := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }

	pair := func(version int64, available, onHold string, updatedAt time.Time, trail *mmodel.BalanceTrailHead) mmodel.BalanceTrailPair {
		return mmodel.BalanceTrailPair{
			Version:   version,
			Available: dec(available),
			OnHold:    dec(onHold),
			UpdatedAt: updatedAt,
			Trail:     trail,
		}
	}

	head := func(version int64, available, onHold string, recordedAt time.Time) *mmodel.BalanceTrailHead {
		return &mmodel.BalanceTrailHead{
			Version:    version,
			Available:  dec(available),
			OnHold:     dec(onHold),
			RecordedAt: recordedAt,
		}
	}

	cancelHead := func(version int64, available, onHold string, recordedAt time.Time) *mmodel.BalanceTrailHead {
		h := head(version, available, onHold, recordedAt)
		h.ValidatedCancelCredit = true

		return h
	}

	tests := []struct {
		name  string
		pair  mmodel.BalanceTrailPair
		grace time.Duration
		want  mmodel.BalanceTrailDirection
	}{
		{
			name:  "matching version and values is ok",
			pair:  pair(4, "100", "10", old, head(4, "100", "10", old)),
			grace: grace,
			want:  mmodel.BalanceTrailOK,
		},
		{
			name:  "balance updated inside grace is deferred",
			pair:  pair(5, "100", "10", insideGrace, head(4, "90", "10", old)),
			grace: grace,
			want:  mmodel.BalanceTrailDeferred,
		},
		{
			name:  "trail recorded inside grace is deferred",
			pair:  pair(4, "100", "10", old, head(5, "90", "10", insideGrace)),
			grace: grace,
			want:  mmodel.BalanceTrailDeferred,
		},
		{
			name:  "newest exactly at grace boundary is outside grace and classifies",
			pair:  pair(5, "100", "10", boundary, head(4, "90", "10", old)),
			grace: grace,
			want:  mmodel.BalanceTrailBalanceAhead,
		},
		{
			name:  "trail exactly at grace boundary is outside grace and classifies",
			pair:  pair(4, "100", "10", old, head(5, "90", "10", boundary)),
			grace: grace,
			want:  mmodel.BalanceTrailTrailAhead,
		},
		{
			name:  "zero grace never defers a pair stamped at now",
			pair:  pair(5, "100", "10", now, head(4, "90", "10", now)),
			grace: 0,
			want:  mmodel.BalanceTrailBalanceAhead,
		},
		{
			name:  "zero grace never defers a pair stamped after now",
			pair:  pair(4, "100", "10", now.Add(time.Second), head(4, "100", "10", now.Add(time.Second))),
			grace: 0,
			want:  mmodel.BalanceTrailOK,
		},
		{
			name:  "no trail with version zero is ok",
			pair:  pair(0, "0", "0", old, nil),
			grace: grace,
			want:  mmodel.BalanceTrailOK,
		},
		{
			name:  "no trail with version three is balance ahead",
			pair:  pair(3, "100", "0", old, nil),
			grace: grace,
			want:  mmodel.BalanceTrailBalanceAhead,
		},
		{
			name:  "no trail updated inside grace is deferred",
			pair:  pair(3, "100", "0", insideGrace, nil),
			grace: grace,
			want:  mmodel.BalanceTrailDeferred,
		},
		{
			name:  "balance version above trail is balance ahead",
			pair:  pair(7, "100", "10", old, head(6, "100", "10", old)),
			grace: grace,
			want:  mmodel.BalanceTrailBalanceAhead,
		},
		{
			name:  "trail version above balance is trail ahead",
			pair:  pair(6, "100", "10", old, head(7, "100", "10", old)),
			grace: grace,
			want:  mmodel.BalanceTrailTrailAhead,
		},
		{
			name:  "version order wins over value difference",
			pair:  pair(6, "1", "2", old, head(7, "100", "10", old)),
			grace: grace,
			want:  mmodel.BalanceTrailTrailAhead,
		},
		{
			name:  "available differs at equal version is value mismatch",
			pair:  pair(4, "100", "10", old, head(4, "99.99", "10", old)),
			grace: grace,
			want:  mmodel.BalanceTrailValueMismatch,
		},
		{
			name:  "on hold differs at equal version is value mismatch",
			pair:  pair(4, "100", "10", old, head(4, "100", "11", old)),
			grace: grace,
			want:  mmodel.BalanceTrailValueMismatch,
		},
		{
			name:  "trail recorded just outside grace classifies",
			pair:  pair(4, "100", "10", old, head(5, "90", "10", boundary.Add(-time.Second))),
			grace: grace,
			want:  mmodel.BalanceTrailTrailAhead,
		},
		{
			name:  "validated cancel head at equal version with different values is ok",
			pair:  pair(5, "100", "0", old, cancelHead(5, "60", "40", old)),
			grace: grace,
			want:  mmodel.BalanceTrailOK,
		},
		{
			name:  "validated cancel head behind the balance is balance ahead",
			pair:  pair(6, "100", "0", old, cancelHead(5, "100", "0", old)),
			grace: grace,
			want:  mmodel.BalanceTrailBalanceAhead,
		},
		{
			name:  "validated cancel head ahead of the balance is trail ahead",
			pair:  pair(5, "100", "0", old, cancelHead(6, "60", "40", old)),
			grace: grace,
			want:  mmodel.BalanceTrailTrailAhead,
		},
		{
			name:  "validated cancel head recorded inside grace is deferred",
			pair:  pair(5, "100", "0", old, cancelHead(5, "60", "40", insideGrace)),
			grace: grace,
			want:  mmodel.BalanceTrailDeferred,
		},
		{
			name:  "legacy trail at version zero behind a moved balance is unverifiable",
			pair:  pair(5, "100", "0", old, head(0, "70", "0", old)),
			grace: grace,
			want:  mmodel.BalanceTrailUnverifiable,
		},
		{
			name:  "legacy trail inside grace is deferred before unverifiable",
			pair:  pair(5, "100", "0", insideGrace, head(0, "70", "0", old)),
			grace: grace,
			want:  mmodel.BalanceTrailDeferred,
		},
		{
			name:  "legacy trail and balance both at version zero with equal values is ok",
			pair:  pair(0, "100", "0", old, head(0, "100", "0", old)),
			grace: grace,
			want:  mmodel.BalanceTrailOK,
		},
		{
			name:  "legacy trail and balance both at version zero with different values is value mismatch",
			pair:  pair(0, "100", "0", old, head(0, "90", "0", old)),
			grace: grace,
			want:  mmodel.BalanceTrailValueMismatch,
		},
		{
			name:  "scale only difference is ok",
			pair:  pair(4, "10.0", "5", old, head(4, "10.00", "5.000", old)),
			grace: grace,
			want:  mmodel.BalanceTrailOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ClassifyBalanceTrailPair(tc.pair, now, tc.grace)
			assert.Equal(t, tc.want, got)
		})
	}
}
