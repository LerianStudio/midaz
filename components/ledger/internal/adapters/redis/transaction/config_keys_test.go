// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestBalanceSettingsCASBackoffStaysBounded pins the pause between lost
// configuration copies to its documented bounds: after the attempt-th lost
// compare-and-set, a jittered share in [0, step) of a step that doubles from one
// millisecond and is capped at 16 ms. A copy makes 8 attempts, so it pauses at
// most 7 times and for less than 63 ms in all, and a PATCH on a hot balance spends
// a bounded time before it answers.
func TestBalanceSettingsCASBackoffStaysBounded(t *testing.T) {
	t.Parallel()

	steps := []time.Duration{
		1 * time.Millisecond,
		2 * time.Millisecond,
		4 * time.Millisecond,
		8 * time.Millisecond,
		16 * time.Millisecond,
		16 * time.Millisecond,
		16 * time.Millisecond,
	}

	require.Equal(t, 8, maxBalanceSettingsCASAttempts, "a copy makes 8 attempts, so it pauses after the first 7")

	var total time.Duration

	for i, step := range steps {
		attempt := i + 1
		total += step

		var longest time.Duration

		for range 200 {
			pause := balanceSettingsCASBackoff(attempt)
			require.GreaterOrEqual(t, pause, time.Duration(0), "attempt %d", attempt)
			require.Less(t, pause, step, "attempt %d", attempt)

			longest = max(longest, pause)
		}

		// A jittered pause spreads over its step: 200 draws all in the lower half
		// of a uniform range would happen once in 2^200 runs.
		require.GreaterOrEqual(t, longest, step/2, "attempt %d pauses for a jittered share of its step, not a fixed or no pause", attempt)
	}

	require.Equal(t, 63*time.Millisecond, total, "the steps of one copy add up to 63 ms")
}
