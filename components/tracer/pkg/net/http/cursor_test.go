// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustEncodeCursor encodes a Cursor to base64 string, panics on error.
// Used only in test setup where marshaling a simple struct should never fail.
func mustEncodeCursor(c Cursor) string {
	b, err := json.Marshal(c)
	if err != nil {
		panic("failed to marshal cursor in test setup: " + err.Error())
	}

	return base64.StdEncoding.EncodeToString(b)
}

func TestEncodeCursor(t *testing.T) {
	tests := []struct {
		name        string
		cursor      Cursor
		wantErr     bool
		expectedErr error
		validate    func(t *testing.T, encoded string)
	}{
		{
			name: "Success - encodes full cursor",
			cursor: Cursor{
				ID:         "rule-123",
				SortValue:  "2024-01-15T10:00:00Z",
				SortBy:     "created_at",
				SortOrder:  "DESC",
				PointsNext: true,
			},
			wantErr: false,
			validate: func(t *testing.T, encoded string) {
				decoded, err := DecodeCursor(encoded)
				require.NoError(t, err)
				assert.Equal(t, "rule-123", decoded.ID)
				assert.Equal(t, "2024-01-15T10:00:00Z", decoded.SortValue)
				assert.Equal(t, "created_at", decoded.SortBy)
				assert.Equal(t, "DESC", decoded.SortOrder)
				assert.True(t, decoded.PointsNext)
			},
		},
		{
			name: "Success - encodes cursor with name sort",
			cursor: Cursor{
				ID:         "rule-456",
				SortValue:  "my rule name",
				SortBy:     "name",
				SortOrder:  "ASC",
				PointsNext: true,
			},
			wantErr: false,
			validate: func(t *testing.T, encoded string) {
				decoded, err := DecodeCursor(encoded)
				require.NoError(t, err)
				assert.Equal(t, "rule-456", decoded.ID)
				assert.Equal(t, "my rule name", decoded.SortValue)
				assert.Equal(t, "name", decoded.SortBy)
				assert.Equal(t, "ASC", decoded.SortOrder)
			},
		},
		{
			name: "Error - empty ID",
			cursor: Cursor{
				ID:         "",
				SortValue:  "2024-01-15T10:00:00Z",
				SortBy:     "created_at",
				SortOrder:  "DESC",
				PointsNext: true,
			},
			wantErr:     true,
			expectedErr: ErrCursorEmptyID,
			validate:    nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := EncodeCursor(tc.cursor)

			if tc.wantErr {
				require.Error(t, err)
				if tc.expectedErr != nil {
					require.ErrorIs(t, err, tc.expectedErr)
				}
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, encoded)
				if tc.validate != nil {
					tc.validate(t, encoded)
				}
			}
		})
	}
}

func TestDecodeCursor(t *testing.T) {
	tests := []struct {
		name        string
		cursor      string
		expected    Cursor
		expectError bool
		expectedErr error
	}{
		{
			name: "Success - decodes full cursor",
			cursor: mustEncodeCursor(Cursor{
				ID:         "abc123",
				SortValue:  "2024-01-15T10:00:00Z",
				SortBy:     "created_at",
				SortOrder:  "DESC",
				PointsNext: true,
			}),
			expected: Cursor{
				ID:         "abc123",
				SortValue:  "2024-01-15T10:00:00Z",
				SortBy:     "created_at",
				SortOrder:  "DESC",
				PointsNext: true,
			},
			expectError: false,
		},
		{
			name: "Success - decodes cursor pointing previous",
			cursor: mustEncodeCursor(Cursor{
				ID:         "xyz789",
				SortValue:  "some value",
				SortBy:     "name",
				SortOrder:  "ASC",
				PointsNext: false,
			}),
			expected: Cursor{
				ID:         "xyz789",
				SortValue:  "some value",
				SortBy:     "name",
				SortOrder:  "ASC",
				PointsNext: false,
			},
			expectError: false,
		},
		{
			name:        "Error - invalid base64 encoding",
			cursor:      "not-valid-base64!@#$",
			expected:    Cursor{},
			expectError: true,
		},
		{
			name:        "Error - valid base64 but invalid JSON",
			cursor:      base64.StdEncoding.EncodeToString([]byte("not json")),
			expected:    Cursor{},
			expectError: true,
		},
		{
			name:        "Error - empty cursor string",
			cursor:      "",
			expected:    Cursor{},
			expectError: true,
		},
		{
			name: "Error - cursor with empty ID",
			cursor: mustEncodeCursor(Cursor{
				ID:         "",
				SortValue:  "2024-01-15T10:00:00Z",
				SortBy:     "created_at",
				SortOrder:  "DESC",
				PointsNext: true,
			}),
			expected:    Cursor{},
			expectError: true,
			expectedErr: ErrCursorEmptyID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DecodeCursor(tc.cursor)

			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErr != nil {
					require.ErrorIs(t, err, tc.expectedErr)
				}
				assert.Equal(t, tc.expected, result)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestCursor_Structure(t *testing.T) {
	t.Run("Success - cursor has expected JSON tags", func(t *testing.T) {
		c := Cursor{
			ID:         "test-id",
			SortValue:  "2024-01-15T10:00:00Z",
			SortBy:     "created_at",
			SortOrder:  "DESC",
			PointsNext: true,
		}
		b, err := json.Marshal(c)
		require.NoError(t, err)

		// Unmarshal into map to verify exact field names
		var result map[string]any
		err = json.Unmarshal(b, &result)
		require.NoError(t, err)

		// Verify expected JSON field names exist with correct values
		assert.Equal(t, "test-id", result["id"], "expected 'id' JSON tag")
		assert.Equal(t, "2024-01-15T10:00:00Z", result["sv"], "expected 'sv' JSON tag for SortValue")
		assert.Equal(t, "created_at", result["sb"], "expected 'sb' JSON tag for SortBy")
		assert.Equal(t, "DESC", result["so"], "expected 'so' JSON tag for SortOrder")
		assert.Equal(t, true, result["pn"], "expected 'pn' JSON tag for PointsNext")

		// Verify no unexpected fields
		assert.Len(t, result, 5, "cursor should have exactly 5 JSON fields")
	})
}

func TestCursor_RoundTrip(t *testing.T) {
	t.Run("Success - encode and decode preserves all fields", func(t *testing.T) {
		original := Cursor{
			ID:         "rule-abc-123",
			SortValue:  "2024-06-15T14:30:00Z",
			SortBy:     "updated_at",
			SortOrder:  "DESC",
			PointsNext: true,
		}

		encoded, err := EncodeCursor(original)
		require.NoError(t, err)

		decoded, err := DecodeCursor(encoded)
		require.NoError(t, err)

		assert.Equal(t, original, decoded)
	})
}
