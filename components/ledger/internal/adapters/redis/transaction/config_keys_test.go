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
// configuration copies: a jittered share of a step that doubles from one
// millisecond and never exceeds the cap, so a PATCH on a hot balance spends a
// bounded time before it answers.
func TestBalanceSettingsCASBackoffStaysBounded(t *testing.T) {
	t.Parallel()

	var total time.Duration

	for attempt := 1; attempt < maxBalanceSettingsCASAttempts; attempt++ {
		step := min(time.Millisecond<<(attempt-1), balanceSettingsCASBackoffLimit)
		total += step

		for range 200 {
			pause := balanceSettingsCASBackoff(attempt)
			require.GreaterOrEqual(t, pause, time.Duration(0))
			require.Less(t, pause, step, "attempt %d", attempt)
		}
	}

	require.LessOrEqual(t, total, 64*time.Millisecond, "the pauses of one copy stay below this bound")
}
