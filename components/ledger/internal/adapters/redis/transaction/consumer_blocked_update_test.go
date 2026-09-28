// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseBlockedUpdateResult locks the {written, corrupt} reply contract of
// scripts/update_balance_blocked.lua as decoded from the go-redis array reply.
func TestParseBlockedUpdateResult(t *testing.T) {
	t.Parallel()

	t.Run("valid pair", func(t *testing.T) {
		t.Parallel()

		written, corrupt, err := parseBlockedUpdateResult([]any{int64(3), int64(1)})
		require.NoError(t, err)
		assert.EqualValues(t, 3, written)
		assert.EqualValues(t, 1, corrupt)
	})

	t.Run("wrong shape", func(t *testing.T) {
		t.Parallel()

		_, _, err := parseBlockedUpdateResult(int64(1))
		assert.Error(t, err)

		_, _, err = parseBlockedUpdateResult([]any{int64(1)})
		assert.Error(t, err)
	})

	t.Run("wrong element types", func(t *testing.T) {
		t.Parallel()

		_, _, err := parseBlockedUpdateResult([]any{"3", int64(1)})
		assert.Error(t, err)

		_, _, err = parseBlockedUpdateResult([]any{int64(3), "1"})
		assert.Error(t, err)
	})
}
