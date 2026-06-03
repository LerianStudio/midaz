// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

// ErrCursorEmptyID is returned when cursor ID is empty.
var ErrCursorEmptyID = errors.New("cursor ID is required")

// Cursor represents a pagination cursor for consistent navigation.
// Contains all information needed to resume pagination from a specific point,
// regardless of data changes between requests.
type Cursor struct {
	ID         string `json:"id"` // ID of the last item returned
	SortValue  string `json:"sv"` // Value of the sort field for the last item
	SortBy     string `json:"sb"` // Field used for sorting (e.g., "created_at", "name")
	SortOrder  string `json:"so"` // Sort direction: "ASC" or "DESC"
	PointsNext bool   `json:"pn"` // Direction indicator (true = next page, false = previous)
}

// EncodeCursor encodes a Cursor to a base64 string.
func EncodeCursor(c Cursor) (string, error) {
	if c.ID == "" {
		return "", ErrCursorEmptyID
	}

	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// DecodeCursor decodes a cursor string.
func DecodeCursor(cursor string) (Cursor, error) {
	decodedCursor, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return Cursor{}, err
	}

	var cur Cursor

	if err := json.Unmarshal(decodedCursor, &cur); err != nil {
		return Cursor{}, err
	}

	if cur.ID == "" {
		return Cursor{}, ErrCursorEmptyID
	}

	return cur, nil
}
