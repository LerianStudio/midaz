// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDecodeAccountClosingAttempt pins the marker encoding reconciliation depends
// on: the phase has to survive the round trip, because an attempt that never
// issued its closing write may have its protection given back and one that did may
// not.
func TestDecodeAccountClosingAttempt(t *testing.T) {
	attempt := uuid.MustParse("b2b3c8e0-0f0a-4a1e-9f8b-8b3e2f4d5a6c").String()

	tests := []struct {
		name  string
		value string
		want  AccountClosingAttempt
	}{
		{
			name:  "before the closing write was issued",
			value: attempt,
			want:  AccountClosingAttempt{Token: attempt},
		},
		{
			name:  "once the closing write was issued",
			value: attempt + accountClosingWriteSuffix,
			want:  AccountClosingAttempt{Token: attempt, WriteIssued: true},
		},
		{
			name:  "surrounding whitespace is not part of the token",
			value: "  " + attempt + "\n",
			want:  AccountClosingAttempt{Token: attempt},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoded, err := decodeAccountClosingAttempt(test.value)

			require.NoError(t, err)
			assert.Equal(t, test.want, decoded)
		})
	}
}

// TestDecodeAccountClosingAttemptRejectsAMarkerWithoutAToken pins that a marker
// which exists but names no attempt is unreadable rather than absent: reading it
// as absence would turn a protection failure into an authorization.
func TestDecodeAccountClosingAttemptRejectsAMarkerWithoutAToken(t *testing.T) {
	for _, value := range []string{" ", accountClosingWriteSuffix} {
		_, err := decodeAccountClosingAttempt(value)

		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrAccountProtectionMarkerUnreadable))
	}
}
