// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fee

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestIsSegmentReference(t *testing.T) {
	t.Parallel()

	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	parsedUUID := uuid.MustParse(validUUID)

	tests := []struct {
		name     string
		entry    string
		wantIs   bool
		wantUUID uuid.UUID
		wantErr  bool
	}{
		{
			name:     "segment prefix with valid UUID returns true and parsed UUID",
			entry:    "segment:" + validUUID,
			wantIs:   true,
			wantUUID: parsedUUID,
			wantErr:  false,
		},
		{
			name:     "segment prefix with invalid UUID returns error",
			entry:    "segment:invalid",
			wantIs:   true,
			wantUUID: uuid.Nil,
			wantErr:  true,
		},
		{
			name:     "regular alias without prefix returns false and uuid.Nil",
			entry:    "regular-alias",
			wantIs:   false,
			wantUUID: uuid.Nil,
			wantErr:  false,
		},
		{
			name:     "empty string returns false and uuid.Nil",
			entry:    "",
			wantIs:   false,
			wantUUID: uuid.Nil,
			wantErr:  false,
		},
		{
			name:     "segment prefix only with no UUID returns error",
			entry:    "segment:",
			wantIs:   true,
			wantUUID: uuid.Nil,
			wantErr:  true,
		},
		{
			name:     "uppercase SEGMENT prefix is case sensitive and returns false",
			entry:    "SEGMENT:" + validUUID,
			wantIs:   false,
			wantUUID: uuid.Nil,
			wantErr:  false,
		},
		{
			name:     "segment prefix with UUID containing spaces returns error",
			entry:    "segment:uuid with spaces",
			wantIs:   true,
			wantUUID: uuid.Nil,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotIs, gotUUID, gotErr := isSegmentReference(tc.entry)

			assert.Equal(t, tc.wantIs, gotIs)
			assert.Equal(t, tc.wantUUID, gotUUID)

			if tc.wantErr {
				assert.Error(t, gotErr)
				assert.Contains(t, gotErr.Error(), "malformed segment waiver")
			} else {
				assert.NoError(t, gotErr)
			}
		})
	}
}
