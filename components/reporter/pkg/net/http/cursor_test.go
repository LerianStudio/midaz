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

func TestDecodeCursor_Success(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		cursor         Cursor
		expectedID     string
		expectedPoints bool
	}{
		{
			name: "Valid cursor pointing next",
			cursor: Cursor{
				ID:         "123456",
				PointsNext: true,
			},
			expectedID:     "123456",
			expectedPoints: true,
		},
		{
			name: "Valid cursor pointing previous",
			cursor: Cursor{
				ID:         "789",
				PointsNext: false,
			},
			expectedID:     "789",
			expectedPoints: false,
		},
		{
			name: "Cursor with UUID",
			cursor: Cursor{
				ID:         "00000000-0000-0000-0000-000000000001",
				PointsNext: true,
			},
			expectedID:     "00000000-0000-0000-0000-000000000001",
			expectedPoints: true,
		},
		{
			name: "Cursor with empty ID",
			cursor: Cursor{
				ID:         "",
				PointsNext: false,
			},
			expectedID:     "",
			expectedPoints: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Encode cursor to base64
			jsonData, err := json.Marshal(tt.cursor)
			require.NoError(t, err)
			encodedCursor := base64.StdEncoding.EncodeToString(jsonData)

			// Decode cursor
			result, err := DecodeCursor(encodedCursor)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedID, result.ID)
			assert.Equal(t, tt.expectedPoints, result.PointsNext)
		})
	}
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cursor string
	}{
		{
			name:   "Invalid base64 characters",
			cursor: "not-valid-base64!!!",
		},
		{
			name:   "Incomplete base64",
			cursor: "YWJj", // Valid but won't produce valid JSON
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := DecodeCursor(tt.cursor)
			require.Error(t, err)
		})
	}
}

func TestDecodeCursor_InvalidJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{
			name: "Invalid JSON structure",
			data: "not json at all",
		},
		{
			name: "Malformed JSON",
			data: `{"id": "123", "points_next":}`,
		},
		{
			name: "Wrong type for ID",
			data: `{"id": 123, "points_next": true}`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encoded := base64.StdEncoding.EncodeToString([]byte(tt.data))
			_, err := DecodeCursor(encoded)
			require.Error(t, err)
		})
	}
}

func TestDecodeCursor_EmptyString(t *testing.T) {
	t.Parallel()

	_, err := DecodeCursor("")
	require.Error(t, err)
}

func TestCursor_Struct(t *testing.T) {
	t.Parallel()

	cursor := Cursor{
		ID:         "test-id",
		PointsNext: true,
	}

	assert.Equal(t, "test-id", cursor.ID)
	assert.True(t, cursor.PointsNext)
}

func TestCursor_JSONTags(t *testing.T) {
	t.Parallel()

	cursor := Cursor{
		ID:         "123",
		PointsNext: true,
	}

	data, err := json.Marshal(cursor)
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	// Check JSON field names
	assert.Contains(t, result, "id")
	assert.Contains(t, result, "points_next")
	assert.Equal(t, "123", result["id"])
	assert.Equal(t, true, result["points_next"])
}
